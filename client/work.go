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

package client

import (
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"

	pb "github.com/google/marmot/proto/marmot"
)

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

// Meta contains metadata information about different objects.  Not all fields
// are used in every object.
type Meta struct {
	// ID contains a unique UUIDv4 representing the object.
	ID string

	// Name contains the name to be displayed to users.
	Name string

	// Desc contains a description of what the object is doing.
	Desc string

	// State is the current state of the object.  It cannot be set by the client.
	State State

	// Started is when an object begins execution.
	Started time.Time

	// Ended is when an object stops execution.
	Ended time.Time

	// Submitted is when a Labor was submitted to the system.  Only valid for Labors.
	Submitted time.Time

	// Reason contains the failiure reason for an object, which is a type Reason.
	Reason Reason
}

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

// Labor represents a total unit of work to be done.
type Labor struct {
	// Tags are single word text strings that can be used to group Labors when
	// doing a search.
	Tags []string

	// Tasks are the tasks that are associated with this Labor.
	Tasks []*Task

	Meta
}

func (l *Labor) fromProto(p *pb.Labor) {
	l.ID = p.Id
	l.Name = p.Name
	l.Desc = p.Desc
	l.State = protoToNativeState[p.State.State]
	l.Started = time.Unix(p.Timing.Started, 0)
	l.Ended = time.Unix(p.Timing.Ended, 0)
	l.Submitted = time.Unix(p.Timing.Submitted, 0)
	l.Reason = protoToNativeReason[p.State.Reason]

	l.Tags = make([]string, len(p.Tags))
	copy(l.Tags, p.Tags)

	for _, t := range p.Tasks {
		nt := &Task{}
		nt.fromProto(t)
		l.Tasks = append(l.Tasks, nt)
	}
}

func (l *Labor) toProto() *pb.Labor {
	tags := make([]string, len(l.Tags))
	copy(tags, l.Tags)

	p := &pb.Labor{
		Id:   l.ID,
		Name: l.Name,
		Desc: l.Desc,
		Tags: tags,
		Timing: &pb.Timing{
			Started:   l.Started.Unix(),
			Ended:     l.Ended.Unix(),
			Submitted: l.Submitted.Unix(),
		},

		State: &pb.State{
			State:  nativeStateToProto[l.State],
			Reason: nativeReasonToProto[l.Reason],
		},
	}

	for _, t := range l.Tasks {
		p.Tasks = append(p.Tasks, t.toProto())
	}
	return p
}

// Task represents a selection of work to be executed. This may represents a
// canary, general rollout, or set of work to be executed concurrently.
type Task struct {
	// PreChecks are Jobs that are executed before executing the Jobs in the
	// Task. If any of these fail, the Task is not executed and fails.
	// This is used provide checks before initiating actions. These are run
	// concurrently.
	PreChecks []*Job

	// ContChecks are Jobs that are exeucted continuously until the task ends
	// with ContCheckInterval between runs.  If any check fails, the Task stops
	// execution and fails.  These are run concurrently.
	ContChecks []*Job

	// ToleratedFailures is how many sequence failures to tolerate before stopping.
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

	Meta
}

func (t *Task) fromProto(p *pb.Task) {
	t.ID = p.Id
	t.Name = p.Name
	t.Desc = p.Desc
	t.State = protoToNativeState[p.State.State]
	t.Started = time.Unix(p.Timing.Started, 0)
	t.Ended = time.Unix(p.Timing.Ended, 0)
	t.Reason = protoToNativeReason[p.State.Reason]
	t.ToleratedFailures = int(p.ToleratedFailures)
	t.Concurrency = int(p.Concurrency)
	t.ContCheckInterval = time.Duration(p.ContCheckInterval) * time.Second
	t.PassFailures = p.PassFailures

	for _, jp := range p.PreChecks {
		j := &Job{}
		j.fromProto(jp)
		t.PreChecks = append(t.PreChecks, j)
	}
	for _, jp := range p.ContChecks {
		j := &Job{}
		j.fromProto(jp)
		t.ContChecks = append(t.ContChecks, j)
	}
	for _, sp := range p.Sequences {
		s := &Sequence{}
		s.fromProto(sp)
		t.Sequences = append(t.Sequences, s)
	}
}

func (t *Task) toProto() *pb.Task {
	p := &pb.Task{
		Id:                t.ID,
		Name:              t.Name,
		Desc:              t.Desc,
		ToleratedFailures: int64(t.ToleratedFailures),
		Concurrency:       int64(t.Concurrency),
		ContCheckInterval: int64(t.ContCheckInterval / time.Second),
		PassFailures:      t.PassFailures,
		Timing: &pb.Timing{
			Started: t.Started.Unix(),
			Ended:   t.Ended.Unix(),
		},
		State: &pb.State{
			State:  nativeStateToProto[t.State],
			Reason: nativeReasonToProto[t.Reason],
		},
	}

	for _, j := range t.PreChecks {
		p.PreChecks = append(p.PreChecks, j.toProto())
	}

	for _, j := range t.ContChecks {
		p.ContChecks = append(p.ContChecks, j.toProto())
	}

	for _, s := range t.Sequences {
		p.Sequences = append(p.Sequences, s.toProto())
	}

	return p
}

// Sequence represents a sequence of Jobs to execute.
type Sequence struct {
	// Target is the name of what this sequence targets. This could be a device,
	// a service, a directory... whatever makes sense for whatever is being done.
	Target string

	// Jobs is a sequence of jobs to execute.
	Jobs []*Job

	Meta
}

func (s *Sequence) fromProto(p *pb.Sequence) {
	s.ID = p.Id
	s.Desc = p.Desc
	s.State = protoToNativeState[p.State.State]
	s.Started = time.Unix(p.Timing.Started, 0)
	s.Ended = time.Unix(p.Timing.Ended, 0)
	s.Reason = protoToNativeReason[p.State.Reason]
	s.Target = p.Target

	for _, jp := range p.Jobs {
		j := &Job{}
		j.fromProto(jp)
		s.Jobs = append(s.Jobs, j)
	}
}

func (s *Sequence) toProto() *pb.Sequence {
	p := &pb.Sequence{
		Id:     s.ID,
		Target: s.Target,
		Desc:   s.Desc,
		Timing: &pb.Timing{
			Started: s.Started.Unix(),
			Ended:   s.Ended.Unix(),
		},
		State: &pb.State{
			State:  nativeStateToProto[s.State],
			Reason: nativeReasonToProto[s.Reason],
		},
	}

	for _, j := range s.Jobs {
		p.Jobs = append(p.Jobs, j.toProto())
	}
	return p
}

// ArgsType indicates what type of encoding the arguments are in.
type ArgsType int

func (a ArgsType) isArgsType() {}

const (
	// ATUnknown indicates the arguements are encoded in an unknown format.
	ATUnknown ArgsType = 0
	// ATJSON indicates the arguments will be encoded in JSON format.
	ATJSON ArgsType = 1
)

// Job represents work to be done, via a Cog.
type Job struct {
	// CogPath is the name of the Cog the Job will use.
	CogPath string

	// Args are the arguments to the Cog in JSON protocol buffer format.
	// Use .SetArgs() to convert your proto.Message to JSON automatically.
	Args string

	// Output is the output from the Cog.
	Output string

	// Timeout is the amount of time a Job is allowed to run.
	Timeout time.Duration

	// Retries is the number of times to retry a Cog that fails.
	// TODO(johnsiilver): Add support for this on the server side.
	Retries int

	// RetryDelay is how long to wait between retrying a Cog.
	RetryDelay time.Duration

	Meta
}

// SetArgs transforms "p" into JSON format and stores it in .Args.
// Note that it uses the "github.com/gogo/protobuf/jsonpb".Marshaler and not
// the standard json library encoder.
func (j *Job) SetArgs(p proto.Message) error {
	var err error
	j.Args, err = (&jsonpb.Marshaler{Indent: "\t"}).MarshalToString(p)
	if err != nil {
		return err
	}
	return nil
}

// GetArgs converts the JSON stored in .Args to "p".
func (j *Job) GetArgs(p proto.Message) error {
	return jsonpb.UnmarshalString(j.Args, p)
}

func (j *Job) fromProto(p *pb.Job) {
	j.ID = p.Id
	j.Desc = p.Desc
	j.State = protoToNativeState[p.State.State]
	j.Started = time.Unix(p.Timing.Started, 0)
	j.Ended = time.Unix(p.Timing.Ended, 0)
	j.Reason = protoToNativeReason[p.State.Reason]

	// TODO(johnsiilver): Fix p.Plugin to be CogPath.
	j.CogPath = p.Plugin
	j.Args = p.Args
	j.Output = p.Output
	j.Timeout = time.Duration(p.Timeout) * time.Second
	j.Retries = int(p.Retries)
	j.RetryDelay = time.Duration(p.RetryDelay) * time.Second
}

func (j *Job) toProto() *pb.Job {
	p := &pb.Job{
		Id:         j.ID,
		Plugin:     j.CogPath,
		Desc:       j.Desc,
		Timeout:    int64(j.Timeout.Seconds()),
		Args:       j.Args,
		Output:     j.Output,
		Retries:    int64(j.Retries),
		RetryDelay: int64(j.RetryDelay.Seconds()),
		Timing: &pb.Timing{
			Started: j.Started.Unix(),
			Ended:   j.Ended.Unix(),
		},
		State: &pb.State{
			State:  nativeStateToProto[j.State],
			Reason: nativeReasonToProto[j.Reason],
		},
	}
	return p
}
