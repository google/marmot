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

// Task represents a selection of work to be executed. This may represents a
// canary, general rollout, or set of work to be executed concurrently.
type Task struct {
	*Hub
	Meta

	// PreChecks are Jobs that are executed before executing the Jobs in the
	// Task. If any of these fail, the Task is not executed and fails.
	// This is used provide checks before initiating actions. These are run
	// concurrently.
	PreChecks []*Job

	// ContChecks are Jobs that are exeucted continuously until the task ends
	// with ContCheckInterval between runs.  If any check fails, the Task stops
	// execution and fails.  These are run concurrently.
	ContChecks []*Job

	// TODO(johnsiilver): Add support for these.
	// PauseOnEntry bool
	// SleepOnEntry time.Duration

	// ToleratedFailures is how many failures to tolerate before stopping.
	ToleratedFailures int

	// Concurrency is how many Jobs to run simultaneously.
	Concurrency int

	// ContCheckInterval is how long between runs of ContCheck Jobs.
	ContCheckInterval time.Duration

	// PassFailures causes the failures in this Task to be passed to the next
	// Task, acting againsts its ToleratedFailures.
	PassFailures bool

	// Sequence are a set of Jobs.
	Sequences []*Sequence

	// cogInfo contains information about Cogs.
	cogInfo *CogInfo

	// store holds the storage layer for data.
	store Storage

	// contDone is set to a channel that is closed when all continuous checks
	// are completed.
	contDone chan struct{}

	signalCont chan Signal
	signalAck  chan struct{}

	// signalDrain is used to siphon off signals for sub-components until the
	// workflow completes running.
	signalDrain *signalDrain

	// wg is used to allow us to syncronize on when all Sequences have completed
	// execution.
	wg sync.WaitGroup

	// changeMu prevents multiple signals being processed at the same time.
	changeMu sync.Mutex
}

// NewTask is the constructor for Task.
func NewTask(name, desc string, done chan bool) *Task {
	t := &Task{
		Meta:        Meta{Name: name, Desc: desc},
		Hub:         NewHub(),
		Concurrency: 1,
		signalCont:  make(chan Signal, 1),
		signalAck:   make(chan struct{}, 1),
		signalDrain: newSignalDrain(done),
	}

	t.SetState(NotStarted)
	return t
}

// Validate validates the Tasks attributes.
func (t *Task) Validate() error {
	if t.State() != NotStarted {
		return fmt.Errorf("Task state must be NotStarted, was: %v", t.State())
	}

	if err := t.Meta.Validate(); err != nil {
		return err
	}

	if len(t.Sequences) == 0 {
		return fmt.Errorf("Task must have at least one Sequeunce")
	}

	if t.Concurrency < 1 {
		return fmt.Errorf("Task must have Concurrenct > 0: %d", t.Concurrency)
	}

	if t.signalCont == nil {
		return fmt.Errorf("Task.signalCont cannot be nil")
	}

	if t.signalAck == nil {
		return fmt.Errorf("Task.signalAck cannot be nil")
	}

	for x, seq := range t.Sequences {
		if err := seq.Validate(); err != nil {
			return fmt.Errorf("Sequence[%d]: %s", x, err)
		}
	}
	return nil
}

// Signal allows signalling of this Task.
func (t *Task) Signal(signal Signal) {
	t.changeMu.Lock()
	defer t.changeMu.Unlock()

	state := t.State()

	switch signal {
	case Pause, CrisisPause, AdminPause:
		if state != Running {
			return
		}
		t.SetState(Pausing)
	case Stop:
		switch state {
		case Paused, CrisisPaused, AdminPaused, Running:
			t.SetState(Stopping)
		default:
			return
		}
	case Resume:
		switch state {
		case Paused:
			t.SetState(Running)
		default:
			return
		}
	case CrisisResume:
		switch state {
		case CrisisPaused:
			t.SetState(Running)
		default:
			return
		}
	case AdminResume:
		switch state {
		case AdminPaused:
			t.SetState(Running)
		default:
			return
		}
	default:
		panic(fmt.Sprintf("got a signal %q I don't know how to deal with", signal))
	}

	t.signalCont <- signal

	// Signal all of our subs simultaneously and wait for them to respond.
	if len(t.Sequences) > 0 {
		sigWG := sync.WaitGroup{}
		sigWG.Add(len(t.Sequences))
		for _, s := range t.Sequences {
			go func(s *Sequence) {
				defer sigWG.Done()
				s.Signal(signal)
			}(s)
		}
		glog.Infof("before signal wait")
		sigWG.Wait()
		glog.Infof("after signal wait")
		select { // wait until either the continuous checks stop or signal acknowledgement.
		case <-t.contDone:
		case <-t.signalAck:
		}
		glog.Infof("after waiting for continuous actions")
	}

	switch signal {
	case Pause:
		t.SetState(Paused)
	case CrisisPause:
		t.SetState(CrisisPaused)
	case AdminPause:
		t.SetState(AdminPaused)
	case Stop:
		t.SetState(Stopped)
	case Resume, AdminResume, CrisisResume:
		t.SetState(Running)
	default:
		panic(fmt.Sprintf("got a signal %q I don't know how to deal with", signal))
	}
}

// execute exeuctes this Task.
func (t *Task) execute() {
	tasksProc.Add(1)
	tasksExec.Add(1)
	defer tasksExec.Add(-1)
	t.Started = time.Now()
	defer func() { t.Ended = time.Now() }()

	t.SetState(Running)

	if t.Concurrency < 1 {
		panic("(did): concurrency cannot be less than 1, task is broken and server didn't validate")
	}

	rateLimit := make(chan struct{}, t.Concurrency)

	// This happens before PreChecks, which allows the PreChecks and ContChecks
	// to run simultaneously.
	t.contChecks()

	if err := t.runChecks(t.PreChecks); err != nil {
		t.SetState(Failed)
		t.SetReason(PreCheckFailure)
		<-t.contDone
		return
	}

	wg := &sync.WaitGroup{}
	for _, s := range t.Sequences {
		if t.handleStateChange() {
			break
		}

		rateLimit <- struct{}{}
		glog.Infof("executing sequence")
		wg.Add(1)
		go s.execute(wg, &t.failures)
		<-rateLimit
	}
	wg.Wait()
	glog.Infof("All Sequences done")

	// We do this here to get the state before the continuous actions finish.
	t.handleFinalState()

	glog.Infof("Before <-contDone")
	<-t.contDone
	glog.Infof("After <-contDone")

	// We do this one more time to get the actual final state.
	t.handleFinalState()
	t.signalDrain.addCh(t.signalCont, t.signalAck)
}

func (t *Task) handleStateChange() (exit bool) {
	select {
	case <-t.contDone: // This indicates a continuous check failed.
		return true
	default:
		// Do nothing
	}

	if t.failures.get() > t.ToleratedFailures {
		return true
	}

	for {
		switch t.State() {
		case Pausing, Paused, CrisisPaused, AdminPaused:
			waitStateChange(t.Hub, []State{Pausing, Paused, CrisisPaused, AdminPaused})
			time.Sleep(100 * time.Millisecond)
			continue
		case Stopping, Stopped:
			return true
		default:
			// Do nothing
		}
		break
	}
	return false
}

func (t *Task) handleFinalState() {
	for {
		glog.Infof("handling final state")
		state := t.State()
		switch state {
		case Running, Completed:
			if t.failures.get() > t.ToleratedFailures {
				t.SetReason(MaxFailures) // Must be before SetState() in order to write value to storage.
				t.SetState(Failed)
			} else {
				t.SetState(Completed)
			}
		case Stopped:
			t.SetState(t.reallyStopped())
		case Failed:
			// Do nothing, already at its final state.
		case Stopping:
			waitStateChange(t.Hub, []State{Stopping})
			continue
		case Paused, Pausing, CrisisPaused, AdminPaused:
			waitStateChange(t.Hub, []State{Pausing, Paused, CrisisPaused, AdminPaused})
			continue
		default:
			panic(fmt.Sprintf("(did): %q is not a state that a task can be in here", state))
		}
		break
	}
}

// runChecks concurrently runs all "checks" passed to it.
func (t *Task) runChecks(checks []*Job) error {
	wg := &sync.WaitGroup{}
	for _, check := range checks {
		wg.Add(1)
		go check.execute(wg)
	}
	wg.Wait()

	for _, check := range checks {
		if check.Hub.State() != Completed {
			return fmt.Errorf("a check failed")
		}
	}
	return nil
}

// contChecks runs the continuous checks until we reach a stopped state.
// The returned channel is closed when contChecks has stopped.
func (t *Task) contChecks() {
	t.contDone = make(chan struct{})

	// There are no checks, so we just wait for the state of the task to reach
	// a completion state, then we close the done channel.  This allows the
	// execute to check done() for closure (which means an error) while it
	// spins off sequence executions.
	if len(t.ContChecks) == 0 {
		go func() {
			defer close(t.contDone)
			for {
				switch t.State() {
				case Completed, Stopped, Failed:
					return
				}
				select {
				case sig := <-t.signalCont:
					t.signalAck <- struct{}{}
					switch sig {
					case Pause, CrisisPause, AdminPause:
						waitStateChange(t.Hub, []State{Pausing, Paused, CrisisPaused, AdminPaused})
					case Stop:
						return
					}
				default:
					// Do nothing.
				}
				time.Sleep(1 * time.Second)
			}
		}()
		return
	}

	go func() {
		defer close(t.contDone)
		for {
			// If we received a signal, process it. Otherwise, keep executing.
			select {
			case sig := <-t.signalCont:
				t.signalAck <- struct{}{}
				switch sig {
				case Pause, CrisisPause, AdminPause:
					waitStateChange(t.Hub, []State{Pausing, Paused, CrisisPaused, AdminPaused})
				case Stop:
					return
				}
			default:
				// Do nothing.
			}

			switch t.State() {
			case Completed, Stopped, Failed:
				return
			}

			if err := t.runChecks(t.ContChecks); err != nil {
				t.Signal(Stop)
				t.SetReason(ContCheckFailure) // Must be before SetState() in order to write value to storage.
				t.SetState(Failed)
				return
			}
			runAfter := time.Now().Add(t.ContCheckInterval)
			for {
				switch t.State() {
				case Completed, Stopped, Failed:
					return
				case Pausing:
					// This allows us to process a signal to pause.
					break
				}
				if time.Now().After(runAfter) {
					break
				}
				time.Sleep(1 * time.Second)
			}
		}
	}()
}

// SetState overrides the internal Hub.SetState() adding writing to storage.
func (t *Task) SetState(st State) {
	t.Hub.SetState(st)
	switch st {
	case Completed, Failed, Stopped:
		t.Ended = time.Now()
	}
	if t.store != nil && t.ID != "" {
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := t.store.WTask(ctx, t)
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

// reallyStopped looks to see if stop was occuring while the last sequences
// were running.  If they completed successfully, we didn't get stopped.
func (t *Task) reallyStopped() State {
	for _, seq := range t.Sequences {
		switch seq.State() {
		case Stopped, NotStarted:
			return Stopped
		}
	}
	if t.failures.get() > t.ToleratedFailures {
		t.SetReason(MaxFailures) // Must be before SetState() in order to write value to storage.
		return Failed
	}
	return Completed
}
