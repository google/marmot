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
	"testing"
	"time"

	"github.com/kylelemons/godebug/pretty"

	pb "github.com/google/marmot/proto/marmot"
)

func TestLaborFromToProto(t *testing.T) {
	p := &pb.Labor{
		Id:   "1",
		Name: "yo",
		Desc: "world",
		Tags: []string{"tag"},
		Timing: &pb.Timing{
			Started:   time.Now().Unix(),
			Ended:     time.Now().Unix(),
			Submitted: time.Now().Unix(),
		},
		State: &pb.State{
			State:  pb.States_COMPLETED,
			Reason: pb.Reasons_MAX_FAILURES,
		},
	}
	l := &Labor{
		Tags: []string{"tag"},
		Meta: Meta{
			ID:        "1",
			Name:      "yo",
			Desc:      "world",
			Started:   time.Unix(p.Timing.Started, 0),
			Ended:     time.Unix(p.Timing.Ended, 0),
			Submitted: time.Unix(p.Timing.Submitted, 0),
			State:     Completed,
			Reason:    MaxFailures,
		},
	}

	got := &Labor{}
	got.fromProto(p)

	if diff := pretty.Compare(l, got); diff != "" {
		t.Errorf("TestLaborFromToProto(fromProto): -want/+got:\n%s", diff)
	}

	gotP := got.toProto()

	if diff := pretty.Compare(p, gotP); diff != "" {
		t.Errorf("TestLaborFromToProto(toProto): -want/+got:\n%s", diff)
	}
}

func TestTaskFromToProto(t *testing.T) {
	p := &pb.Task{
		Id:   "1",
		Name: "yo",
		Desc: "world",
		Timing: &pb.Timing{
			Started: time.Now().Unix(),
			Ended:   time.Now().Unix(),
		},
		State: &pb.State{
			State:  pb.States_COMPLETED,
			Reason: pb.Reasons_MAX_FAILURES,
		},
		ToleratedFailures: 2,
		Concurrency:       5,
		ContCheckInterval: 60,
		PassFailures:      true,
	}

	task := &Task{
		ToleratedFailures: 2,
		Concurrency:       5,
		ContCheckInterval: 60 * time.Second,
		PassFailures:      true,
		Meta: Meta{
			ID:      "1",
			Name:    "yo",
			Desc:    "world",
			Started: time.Unix(p.Timing.Started, 0),
			Ended:   time.Unix(p.Timing.Ended, 0),
			State:   Completed,
			Reason:  MaxFailures,
		},
	}

	got := &Task{}
	got.fromProto(p)

	if diff := pretty.Compare(task, got); diff != "" {
		t.Errorf("TestTaskFromToProto(fromProto): -want/+got:\n%s", diff)
	}

	gotP := got.toProto()

	if diff := pretty.Compare(p, gotP); diff != "" {
		t.Errorf("TestTaskFromToProto(toProto): -want/+got:\n%s", diff)
	}
}

func TestSequenceFromToProto(t *testing.T) {
	p := &pb.Sequence{
		Id:     "1",
		Target: "target",
		Desc:   "world",
		Timing: &pb.Timing{
			Started: time.Now().Unix(),
			Ended:   time.Now().Unix(),
		},
		State: &pb.State{
			State:  pb.States_COMPLETED,
			Reason: pb.Reasons_MAX_FAILURES,
		},
	}
	seq := &Sequence{
		Target: "target",
		Meta: Meta{
			ID:      "1",
			Desc:    "world",
			Started: time.Unix(p.Timing.Started, 0),
			Ended:   time.Unix(p.Timing.Ended, 0),
			State:   Completed,
			Reason:  MaxFailures,
		},
	}
	got := &Sequence{}
	got.fromProto(p)

	if diff := pretty.Compare(seq, got); diff != "" {
		t.Errorf("TestSequenceFromToProto(fromProto): -want/+got:\n%s", diff)
	}

	gotP := got.toProto()

	if diff := pretty.Compare(p, gotP); diff != "" {
		t.Errorf("TestSequenceFromToProto(toProto): -want/+got:\n%s", diff)
	}
}

func TestJobFromToProto(t *testing.T) {
	p := &pb.Job{
		Plugin:     "path",
		Timeout:    10,
		Args:       "json",
		Output:     "output",
		Retries:    3,
		RetryDelay: 30,
		Id:         "1",
		Desc:       "world",
		Timing: &pb.Timing{
			Started: time.Now().Unix(),
			Ended:   time.Now().Unix(),
		},
		State: &pb.State{
			State:  pb.States_COMPLETED,
			Reason: pb.Reasons_MAX_FAILURES,
		},
	}
	j := Job{
		CogPath:    "path",
		Args:       "json",
		Output:     "output",
		Timeout:    10 * time.Second,
		Retries:    3,
		RetryDelay: 30 * time.Second,
		Meta: Meta{
			ID:      "1",
			Desc:    "world",
			Started: time.Unix(p.Timing.Started, 0),
			Ended:   time.Unix(p.Timing.Ended, 0),
			State:   Completed,
			Reason:  MaxFailures,
		},
	}

	got := &Job{}
	got.fromProto(p)

	if diff := pretty.Compare(j, got); diff != "" {
		t.Errorf("TestJobFromToProto(fromProto): -want/+got:\n%s", diff)
	}

	gotP := got.toProto()

	if diff := pretty.Compare(p, gotP); diff != "" {
		t.Errorf("TestJobFromToProto(toProto): -want/+got:\n%s", diff)
	}
}
