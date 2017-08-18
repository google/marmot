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
	"math/rand"
	"sync"
	"time"
	"unicode"

	"github.com/golang/glog"
	"github.com/kylelemons/godebug/pretty"
	"github.com/pborman/uuid"
)

// Labor represents a total unit of work to be done.
type Labor struct {
	*Hub

	Meta

	// ClientID is a unique ID sent by the client.
	ClientID string

	// Tags are single word text strings that can be used to group Labors when
	// doing a search.
	Tags []string

	// Tasks are the tasks that are associated with this Labor.
	Tasks []*Task

	// Completed is closed when the Labor has finished execution.
	Completed chan bool

	// cogInfo contains information about Cogs.
	cogInfo *CogInfo

	// store holds the storage layer for data.
	store Storage

	// signalDrain is used to siphon off signals for sub-components until the
	// workflow completes running.
	signalDrain *signalDrain

	// changeMu prevents the Labor from receiving multiple signals at the same time.
	changeMu sync.Mutex
}

// NewLabor is the constructor for Labor.
func NewLabor(name, desc string) *Labor {
	return &Labor{
		Hub:       NewHub(),
		Meta:      Meta{Name: name, Desc: desc},
		Completed: make(chan bool),
	}
}

// Validate validates that the Labor is correctly formed.
func (l *Labor) Validate() error {
	switch l.State() {
	case NotStarted:
	default:
		return fmt.Errorf("Labor must be in UnknownState state, was: %v", l.State())
	}

	switch l.Reason() {
	case NoFailure:
	default:
		return fmt.Errorf("Labor must have Reason: NoFailure, was: %v", l.Reason())
	}

	if err := l.Meta.Validate(); err != nil {
		return fmt.Errorf("Labor's Meta field did not validate: %s", err)
	}

	if len(l.Tasks) == 0 {
		return fmt.Errorf("Labor had 0 Tasks")
	}

	if l.Completed == nil {
		return fmt.Errorf("Labor.Completed must not be nil")
	}

	for i, t := range l.Tags {
		for _, r := range t {
			if unicode.IsSpace(r) {
				return fmt.Errorf("Labor.Tag[%d]: %q is not a single word", i, t)
			}
		}
	}

	for x, task := range l.Tasks {
		if err := task.Validate(); err != nil {
			return fmt.Errorf("Task[%d] validate error: %s", x, err)
		}
	}
	return nil
}

// SetIDs creates new UUIDv4 IDs for all containers.
func (l *Labor) SetIDs() {
	l.ID = uuid.New()
	for _, t := range l.Tasks {
		t.ID = uuid.New()
		for _, p := range t.PreChecks {
			p.ID = uuid.New()
		}
		for _, c := range t.ContChecks {
			c.ID = uuid.New()
		}
		for _, s := range t.Sequences {
			s.ID = uuid.New()
			for _, j := range s.Jobs {
				j.ID = uuid.New()
			}
		}
	}
}

// SetStorage sets the internal .store for all containers to "store".
func (l *Labor) SetStorage(store Storage) {
	l.store = store
	for _, t := range l.Tasks {
		t.store = store
		for _, p := range t.PreChecks {
			p.store = store
		}
		for _, c := range t.ContChecks {
			c.store = store
		}
		for _, s := range t.Sequences {
			s.store = store
			for _, j := range s.Jobs {
				j.store = store
			}
		}
	}
}

// SetCogInfo sets the internal .cogInfof for all containers to "cogInfo".
func (l *Labor) SetCogInfo(cogInfo *CogInfo) {
	l.cogInfo = cogInfo
	for _, t := range l.Tasks {
		t.cogInfo = cogInfo
		for _, p := range t.PreChecks {
			p.cogInfo = cogInfo
		}
		for _, c := range t.ContChecks {
			c.cogInfo = cogInfo
		}
		for _, s := range t.Sequences {
			for _, j := range s.Jobs {
				j.cogInfo = cogInfo
			}
		}
	}
}

// SetSignalDrain sets the internal .signalDrain for all containers.
func (l *Labor) SetSignalDrain() {
	sd := newSignalDrain(l.Completed)
	sd.drain()
	l.signalDrain = sd

	for _, t := range l.Tasks {
		t.signalDrain = sd
		for _, s := range t.Sequences {
			s.signalDrain = sd
		}
	}
}

// Signal adjusts the running state to a preliminary state to be in line with
// the signal (PAUSE signal would induce a PAUSING state), sends the signal
// to all Tasks, and then adjusts the Labor's state to its final state.
func (l *Labor) Signal(signal Signal) error {
	l.changeMu.Lock()
	defer l.changeMu.Unlock()

	state := l.State()

	switch signal {
	case Pause, CrisisPause, AdminPause:
		if l.State() != Running {
			return fmt.Errorf("cannot %q a Labor in the %q state", signal, state)
		}
		l.SetState(Pausing)
	case Stop:
		switch state {
		case Paused, CrisisPaused, AdminPaused, Running:
			// Do Nothing.
		default:
			return fmt.Errorf("cannot %q a Labor in the %q state", signal, state)
		}
		l.SetState(Stopping)
	case Resume:
		switch state {
		case Paused:
			// Do Nothing.
		default:
			return fmt.Errorf("cannot %q a Labor in the %q state", signal, state)
		}
		l.SetState(Running)
	case CrisisResume:
		switch state {
		case CrisisPaused:
			// Do Nothing.
		default:
			return fmt.Errorf("cannot %q a Labor in the %q state", signal, state)
		}
		l.SetState(Running)
	case AdminResume:
		switch state {
		case AdminPaused:
			// Do Nothing.
		default:
			return fmt.Errorf("cannot %q a Labor in the %q state", signal, state)
		}
		l.SetState(Running)
	default:
		panic(fmt.Sprintf("got a signal %q I don't know how to deal with", signal))
	}

	for _, t := range l.Tasks { // Only one at at time, because only one can be running.
		t.Signal(signal)
	}

	switch signal {
	case Pause:
		l.SetState(Paused)
	case CrisisPause:
		l.SetState(CrisisPaused)
	case AdminPause:
		l.SetState(AdminPaused)
	case Stop:
		l.SetState(Stopped)
	case Resume, CrisisResume, AdminResume:
		l.SetState(Running)
	default:
		panic(fmt.Sprintf("(did): got a signal %q I don't know how to deal with", signal))
	}

	return nil
}

// Start starts processing the Labor.
func (l *Labor) Start() error {
	l.changeMu.Lock()
	defer l.changeMu.Unlock()

	state := l.State()
	switch state {
	case NotStarted, AdminNotStarted:
		l.SetState(Running)
		glog.Infof("Staring execution")
		go l.executeTasks()
		return nil
	}
	return fmt.Errorf("cannot start Labor that is in state %q", state)
}

func (l *Labor) executeTasks() {
	defer close(l.Completed)
	laborsProc.Add(1)
	laborsExec.Add(1)
	defer laborsExec.Add(-1)
	l.Started = time.Now()

	passFailures := 0
	for _, t := range l.Tasks {
		glog.Infof("executing task")
		if passFailures > 0 {
			t.failures.incrBy(passFailures)
		}
		t.execute()
		tState := t.Hub.State()
		if tState != Completed {
			if tState == Stopped {
				l.SetState(l.reallyStopped())
				return
			}
			l.SetReason(t.Reason()) // Must be before SetState() in order to write value to storage.
			l.SetState(Failed)
			glog.Infof("labor at the end:\n%s", pretty.Sprint(l))
			glog.Infof("labor reason: %s", l.Reason())
			return
		}
		if t.PassFailures {
			passFailures = t.failures.get()
		} else {
			passFailures = 0
		}
	}

	l.SetState(Completed)
	glog.Infof("labor at the end:\n%s", pretty.Sprint(l))
}

var random = rand.New(rand.NewSource(time.Now().UnixNano()))

// SetState overrides the internal Hub.SetState() adding writing to storage.
func (l *Labor) SetState(st State) {
	l.Hub.SetState(st)
	switch st {
	case Completed, Failed, Stopped:
		l.Ended = time.Now()
	}
	if l.store != nil && l.ID != "" {
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := l.store.WLaborState(ctx, l)
			cancel()
			if err != nil {
				glog.Errorf("error writing labor state to storage: %s", err)
				time.Sleep(time.Duration(random.Intn(5)) * time.Second)
				continue
			}
			glog.Infof("wrote to storage")
			break
		}
	}
}

// reallyStopped looks to see if stop was occuring while the last action of
// the sequence in the last task was being done.  This results in a stopped
// state that isn't stopped.  This should only be run if we think we are in
// a stopped state, otherwise it can give a false state.
func (l *Labor) reallyStopped() State {
	switch l.Tasks[len(l.Tasks)-1].State() {
	case Stopped, NotStarted:
		return Stopped
	default:
		return l.Tasks[len(l.Tasks)-1].State()
	}
}
