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

package work

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/golang/glog"
)

// Sequence represents a sequence of Jobs to execute.
type Sequence struct {
	*Hub
	Meta

	// Target is the name of what this sequence targets. This could be a device,
	// a service, a directory... whatever makes sense for whatever is being done.
	Target string

	// Jobs is a sequence of jobs to execute.
	Jobs []*Job

	// store holds the storage layer for data.
	store Storage

	// signalExec is used to signal the running execute().
	signalExec chan Signal

	// signalAck is used to listen for execute() to acknowledge a signal.
	signalAck chan struct{}

	// signalDrain is used to siphon off signals for sub-components until the
	// workflow completes running.
	signalDrain *signalDrain

	// changeMu prevents multiple signals being processed at the same time.
	changeMu sync.Mutex
}

// NewSequence is the constructor for Sequence.
func NewSequence(target string, done chan bool) *Sequence {
	s := &Sequence{
		Target:      target,
		Meta:        Meta{},
		Hub:         NewHub(),
		signalExec:  make(chan Signal, 1),
		signalAck:   make(chan struct{}, 1),
		signalDrain: newSignalDrain(done),
	}
	s.SetState(NotStarted)
	return s
}

// Validate validates the Sequences attributes.
func (s *Sequence) Validate() error {
	if s.State() != NotStarted {
		return fmt.Errorf("Sequence state must be NotStarted, was: %v", s.State())
	}

	if s.Target == "" {
		return fmt.Errorf("Sequence must have Target set")
	}

	if err := s.Meta.Validate(); err != nil {
		return err
	}

	if s.signalExec == nil {
		return fmt.Errorf("singalExec cannot be nil")
	}

	if s.signalAck == nil {
		return fmt.Errorf("singalAck cannot be nil")
	}

	for x, job := range s.Jobs {
		if err := job.Validate(); err != nil {
			return fmt.Errorf("Job[%d]: %s", x, err)
		}
	}
	return nil
}

// Signal allows signalling of this Task.
func (s *Sequence) Signal(signal Signal) {
	s.changeMu.Lock()
	defer s.changeMu.Unlock()

	state := s.State()

	switch signal {
	case Pause, CrisisPause, AdminPause:
		if state != Running {
			return
		}
		s.SetState(Pausing)
	case Stop:
		switch state {
		case Paused, CrisisPaused, AdminPaused, Running:
			s.SetState(Stopping)
		default:
			return
		}
	case Resume:
		switch state {
		case Paused:
			s.SetState(Running)
		default:
			return
		}
	case CrisisResume:
		switch state {
		case CrisisPaused:
			s.SetState(Running)
		default:
			return
		}
	case AdminResume:
		switch state {
		case AdminPaused:
			s.SetState(Running)
		default:
			return
		}
	default:
		panic(fmt.Sprintf("(did): got a signal %q I don't know how to deal with", signal))
	}

	s.signalExec <- signal
	<-s.signalAck

	switch signal {
	case Pause:
		s.SetState(Paused)
	case CrisisPause:
		s.SetState(CrisisPaused)
	case AdminPause:
		s.SetState(AdminPaused)
	case Stop:
		s.SetState(s.reallyStopped())
	case Resume:
		s.SetState(Running)
	default:
		panic(fmt.Sprintf("(did): got a signal %q I don't know how to deal with", signal))
	}
}

// execute executes this Sequence.
func (s *Sequence) execute(wg *sync.WaitGroup, failures *counter) {
	defer wg.Done()
	defer s.signalDrain.addCh(s.signalExec, s.signalAck)
	defer glog.Infof("signalDrain is %v", s.signalDrain)

	seqsProc.Add(1)
	seqsExec.Add(1)
	defer seqsExec.Add(-1)

	s.Started = time.Now()

	s.SetState(Running)

	for _, j := range s.Jobs {
		for {
			select {
			case sig := <-s.signalExec:
				s.signalAck <- struct{}{}
				switch sig {
				case Pause:
					waitStateChange(s.Hub, []State{Paused, Pausing})
					continue
				case CrisisPause:
					waitStateChange(s.Hub, []State{CrisisPaused, Pausing})
					continue
				case AdminPause:
					waitStateChange(s.Hub, []State{AdminPaused, Pausing})
					continue
				case Stop:
					(*failures).incr()
					return
				case Resume:
					break
				default:
					panic(fmt.Sprintf("(did): got a signal %q I don't know how to deal with", sig))
				}
			default:
				// No signal, select ends, we break the loop.
			}
			break
		}

		glog.Infof("executing job")
		jobWG := &sync.WaitGroup{}
		jobWG.Add(1)
		j.execute(jobWG)
		jobWG.Wait()
		glog.Infof("job done")

		state := j.Hub.State()
		if state != Completed {
			switch state {
			case Stopped:
				s.Hub.SetState(s.reallyStopped())
			default:
				(*failures).incr()
				s.Hub.SetState(Failed)
			}
			glog.Infof("what was the state: %v", state)
			return
		}
	}
	s.SetState(Completed)
}

// SetState overrides the internal Hub.SetState() adding writing to storage.
func (s *Sequence) SetState(st State) {
	s.Hub.SetState(st)
	switch st {
	case Completed, Failed, Stopped:
		s.Ended = time.Now()
	}

	if s.store != nil && s.ID != "" {
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := s.store.WSequence(ctx, s)
			cancel()
			if err != nil {
				glog.Errorf("error writing Task state to storage: %s", err)
				time.Sleep(time.Duration(random.Intn(5)) * time.Second)
				continue
			}
			break
		}
	}
}

// reallyStopped looks to see if stop was occuring while the last job of a
// sequence was running.  If it completed successfully, we didn't get stopped.
func (s *Sequence) reallyStopped() State {
	switch s.Jobs[len(s.Jobs)-1].State() {
	case Stopped, NotStarted:
		return Stopped
	}
	return s.Jobs[len(s.Jobs)-1].State()
}
