// Copyright 2017 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/*
Package work provides mechanisms for executing a workflow, which is
represented by the Labor object.  The Labor object has the following structure:

Labor
  |__Task
	|  |__Sequence
	|	 | |__Job
	|	 | |__Job
	|  |__Sequence
	|    |__Job
	|	   |__Job
	|__Task
	   |__Sequence
		 ...
	...

Labor

Represents the body of work to be done.

Tasks

Execute one at a time.  Failure of a task prevents other tasks from running.

Sequences

Different sequences *May* run concurrently.  They represent a change to a single entity.

Job

A Job is what does the work.  Each job in a Sequence is run sequentially.
Failure of one Job fails the Sequence.
*/
package work

import (
	"context"
	"expvar"
	"fmt"
	"regexp"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/johnsiilver/cog/client"
	pb "github.com/johnsiilver/cog/proto/cog"
)

var (
	laborsExec = expvar.NewInt("LaborsExecuting")
	tasksExec  = expvar.NewInt("TasksExecuting")
	seqsExec   = expvar.NewInt("SequencesExecuting")
	jobsExec   = expvar.NewInt("JobsExecuting")

	laborsProc = expvar.NewInt("LaborsProcessed")
	tasksProc  = expvar.NewInt("TasksProcessed")
	seqsProc   = expvar.NewInt("SequencesProcessed")
	jobsProc   = expvar.NewInt("JobsProcessed")
)

var uuid4hex = regexp.MustCompile(`(?i)[0-9a-f]{12}4[0-9a-f]{3}[89ab][0-9a-f]{15}$`)

// Reason details reasons that a Container fails.
type Reason int

// Note: Changes to this list require running stringer!
const (
	// NoFailure indicates there was no failures in the execution.
	NoFailure Reason = 0

	// PreCheckFailure indicates that a pre check caused a failure.
	PreCheckFailure Reason = 1

	// ContChecks indicates a continuous check caused a failure.
	ContCheckFailure Reason = 2

	// MaxFailures indicates that the maximum number of Jobs failed.
	MaxFailures Reason = 3
)

// Meta contains metadata information about different objects.  Not all fields
// are used in every object.
type Meta struct {
	// ID contains a unique UUIDv4 representing the object.
	ID string

	// Name contains the name to be displayed to users.
	Name string

	// Desc contains a description of what the object is doing.
	Desc string

	// Started is when an object begins execution.
	Started time.Time

	// Ended is when an object stops execution.
	Ended time.Time

	// Submitted is when a Labor was submitted to the system.  Only valid for Labors.
	Submitted time.Time

	// mu protects reason.
	mu sync.Mutex

	// reason contains the failiure reason for an object, which is a type Reason.
	reason atomic.Value
}

// Reason retrieves the failure reason for the object.
func (m *Meta) Reason() Reason {
	r := m.reason.Load()
	if r == nil {
		return Reason(0)
	}
	return r.(Reason)
}

// SetReason sets the failure reason in a thread safe manner.
func (m *Meta) SetReason(r Reason) {
	m.reason.Store(r)
}

// Validate validates Meta's attributes.
func (m *Meta) Validate() error {
	if m.ID != "" {
		return fmt.Errorf("ID cannot be set, was %q", m.ID)
	}

	return nil
}

// State represents the exeuction state of an object.  Not all states can
// be used in all objects.
type State int

// Note: Changes to this list require running stringer!
const (
	// UnknownState indicates the object's state hasn't been set.  This should
	// be resolved before object execution.
	UnknownState State = 0

	// NotStarted indicates that the object hasn't started execution.
	NotStarted State = 1

	// AdminNotStarted indicates the Labor object was sent to the server, but
	// was not intended to run until started by an RPC or human.
	AdminNotStarted State = 2

	// Running indicates the object is currently executing.
	Running State = 3

	// Pausing indicates the object is intending to pause execution.
	Pausing State = 4

	// Paused indicates the object has paused execution.
	Paused State = 5

	// AdminPaused indicates that a human has paused execution.
	AdminPaused State = 6

	// CrisisPaused indicates that the crisis service for emergency control has
	// paused execution.
	CrisisPaused State = 7

	// Completed indicates the object has completed execution succcessfully.
	Completed State = 8

	// Failed indicates the object failed its execution.
	Failed State = 9

	// Stopping indicates the object is attempting to stop.
	Stopping State = 10

	// Stopped indicates the object's execution was stopped.
	Stopped State = 11
)

// Signal is used to tell objects to switch execution states.
type Signal int

const (
	// Stop indicates to stop execution if running.
	Stop Signal = iota

	// Pause indicates to pause execution if running.
	Pause

	// CrisisPause indicates to pause execution if running.
	CrisisPause

	// AdminPause indicates to pause execution if running.
	AdminPause

	// Resume indicates to resume execution if in Paused state.
	Resume

	// CrisisResume indicates to resume execution if in CrisisPaused state.
	CrisisResume

	// AdminResume indicates to resume execution if in the AdminPaused state.
	AdminResume
)

// counter provides a simple counter that can be incremented and decremented
// in a thread safe way.
type counter int64

func (c *counter) incr() {
	atomic.AddInt64((*int64)(c), 1)
	runtime.Gosched()
}

func (c *counter) incrBy(i int) {
	atomic.AddInt64((*int64)(c), int64(i))
}

func (c *counter) decr() {
	atomic.AddInt64((*int64)(c), -1)
	runtime.Gosched()
}
func (c *counter) get() int {
	return int(atomic.LoadInt64((*int64)(c)))
}

// Hub provides access to an object's state data, various statistics, and
// signally methods.
type Hub struct {
	state     atomic.Value
	executing counter
	failures  counter

	// Signal provides access to signalling methods.
	Signal chan SignalMsg
}

// NewHub is a constructor for Hub.
func NewHub() *Hub {
	s := &Hub{
		Signal: make(chan SignalMsg, 1),
	}
	s.state.Store(UnknownState)
	return s
}

// SetState allows setting of the state data in a thread safe manner.
func (s *Hub) SetState(st State) {
	s.state.Store(st)
}

// State retrieves the state in a thread safe manner.
func (s *Hub) State() State {
	return s.state.Load().(State)
}

// Executing gets the number of sub objects that are currently executing.
func (s *Hub) Executing() int {
	return s.executing.get()
}

// SignalMsg is used to send a signal to an object and receive an
// ackknowledgement back.
type SignalMsg struct {
	// Signal is the signal type you are sending.
	Signal Signal

	// Ack is closed when the sub-object has finished its signal processing.
	Ack chan bool
}

// waitStateChange indicates to wait until the State is not longer one of the
// "from" states.
func waitStateChange(current *Hub, from []State) {
	for {
		state := current.State()
		keepWaiting := false
		for _, fromState := range from {
			if fromState == state {
				keepWaiting = true
				break
			}
		}
		if keepWaiting {
			time.Sleep(1 * time.Second)
			continue
		}
		break
	}
}

const defaultTimeout = 5 * time.Minute

// CogInfo contains information about Cogs running on the system.
type CogInfo struct {
	client     *client.Client
	mu         *sync.Mutex
	cogCrashes map[string]*int64
	maxCrashes int
}

func (c *CogInfo) numCrash(cogPath string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	v, ok := c.cogCrashes[cogPath]
	if !ok {
		return 0
	}
	return int(atomic.LoadInt64(v))
}

// crashIncr increments the crash counter for cogPath by 1.
func (c *CogInfo) crashIncr(cogPath string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	v, ok := c.cogCrashes[cogPath]
	if !ok {
		v = new(int64)
		c.cogCrashes[cogPath] = v
	}
	atomic.AddInt64(v, 1)
}

// crashReset removes the cogPath counter from the cogCrashes map.
func (c *CogInfo) crashReset(cogPath string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.cogCrashes, cogPath)
}

// NewCogInfo creates a CogInfo.
func NewCogInfo(maxCrashes int) (*CogInfo, error) {
	cli, err := client.New()
	if err != nil {
		return nil, err
	}
	return &CogInfo{
		client:     cli,
		mu:         &sync.Mutex{},
		cogCrashes: make(map[string]*int64, 10),
		maxCrashes: maxCrashes,
	}, nil
}

type executeArgs struct {
	cogPath  string
	realUser string
	args     []byte
	argsType pb.ArgsType
	cogInfo  *CogInfo
	server   *pb.Server
}

func executeCog(ctx context.Context, args executeArgs) (output []byte, state State, retry bool, err error) {
	glog.Infof("starting executeCog")
	defer glog.Infof("ending executeCog")
	if args.cogInfo.numCrash(args.cogPath) > args.cogInfo.maxCrashes {
		return nil, Failed, false, fmt.Errorf("plugin has exceeded maximum crashes, new verison must be loaded to try again")
	}

	if !args.cogInfo.client.Loaded(args.cogPath) {
		if err = args.cogInfo.client.Load(args.cogPath); err != nil {
			return nil, Failed, false, err
		}
	}

	status, out, err := args.cogInfo.client.Execute(ctx, args.cogPath, args.realUser, args.args, args.argsType, args.server)
	if err != nil {
		if _, ok := err.(client.CogCrashError); ok {
			args.cogInfo.crashIncr(args.cogPath)
		}
		return nil, Failed, true, err
	}
	switch status {
	case pb.Status_SUCCESS:
		return out, Completed, true, nil
	case pb.Status_FAILURE:
		return out, Failed, true, nil
	case pb.Status_FAILURE_NO_RETRIES:
		return out, Failed, false, nil
	case pb.Status_UNKNOWN:
		return out, Failed, false, fmt.Errorf("returned unknown status, the plugin is broken")
	}
	return out, Failed, false, fmt.Errorf("returned a state that isn't valid: %v", status)
}

// signalPair represents an input channel (exec) and ackknowledgement
// channel (ack).
type signalPair struct {
	exec chan Signal
	ack  chan struct{}
}

// signalDrainage is used to drain signals to components that are no longer
// processing until workDone is closed.
type signalDrain struct {
	pairs    []signalPair
	workDone chan bool
	sync.RWMutex
	done chan struct{}
	once sync.Once
}

// newSignalDrain is the constructor for signalDrain.
func newSignalDrain(workDone chan bool) *signalDrain {
	return &signalDrain{
		workDone: workDone,
		done:     make(chan struct{}),
	}
}

// addCh adds an exec/ack pair to be signal drained.
func (s *signalDrain) addCh(exec chan Signal, ack chan struct{}) {
	s.Lock()
	defer s.Unlock()
	s.pairs = append(
		s.pairs,
		signalPair{
			exec: exec,
			ack:  ack,
		},
	)
}

// drainSignals is run by all components after they reach a completed state.
// This allows signalling to continue working after a component finishes
// processing.  Multiple calls to this will only call this once.
func (s *signalDrain) drain() {
	s.once.Do(
		func() {
			go func() {
				for {
					if s.processPairs() {
						return
					}
					time.Sleep(500 * time.Millisecond)
				}
			}()
		},
	)
}

func (s *signalDrain) processPairs() bool {
	s.RLock()
	for _, pair := range s.pairs {
		pair := pair

		select {
		case _ = <-pair.exec:
			go func() {
				select {
				case pair.ack <- struct{}{}:
				case <-time.After(30 * time.Second):
					glog.Infof("Something is seriously wrong, signal acknowledgement is over 30 seconds")
					pair.ack <- struct{}{}
				}
			}()
		case <-s.workDone:
			s.RUnlock()
			close(s.done)
			return true
		default:
			// Do nothing.
		}
	}
	s.RUnlock()
	return false
}
