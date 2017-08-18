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

package convert

import (
	"fmt"
	"time"

	pb "github.com/google/marmot/proto/marmot"
	"github.com/google/marmot/work"
)

var nativeStateToProto = map[work.State]pb.States{
	work.UnknownState:    pb.States_UNKNOWN,
	work.NotStarted:      pb.States_NOT_STARTED,
	work.AdminNotStarted: pb.States_ADMIN_NOT_STARTED,
	work.Running:         pb.States_RUNNING,
	work.Pausing:         pb.States_PAUSING,
	work.Paused:          pb.States_PAUSED,
	work.AdminPaused:     pb.States_ADMIN_PAUSED,
	work.CrisisPaused:    pb.States_CRISIS_PAUSED,
	work.Completed:       pb.States_COMPLETED,
	work.Failed:          pb.States_FAILED,
	work.Stopping:        pb.States_STOPPING,
	work.Stopped:         pb.States_STOPPED,
}

var protoToNativeState map[pb.States]work.State

// Create the reverse of nativeStateToProto dynamically.
func init() {
	protoToNativeState = make(map[pb.States]work.State, len(nativeStateToProto))
	for k, v := range nativeStateToProto {
		protoToNativeState[v] = k
	}
}

var nativeReasonToProto = map[work.Reason]pb.Reasons{
	work.NoFailure:        pb.Reasons_NO_FAILURE,
	work.PreCheckFailure:  pb.Reasons_PRE_CHECK_FAILURE,
	work.ContCheckFailure: pb.Reasons_CONT_CHECK_FAILURE,
	work.MaxFailures:      pb.Reasons_MAX_FAILURES,
}

var protoToNativeReason map[pb.Reasons]work.Reason

// Create the reverse of nativeReasonToProto dynamically.
func init() {
	protoToNativeReason = make(map[pb.Reasons]work.Reason, len(nativeReasonToProto))
	for k, v := range nativeReasonToProto {
		protoToNativeReason[v] = k
	}
}

// ToNative converts the protocol buffer input from a GRPC to the native
// implementation used in the service.
func ToNative(p *pb.Labor, store work.Storage) (*work.Labor, error) {
	switch {
	case p == nil:
		return nil, fmt.Errorf("ToNative(nil) is not valid")
	}

	l := work.NewLabor(p.Name, p.Desc)
	l.ID = p.Id
	l.ClientID = p.ClientId
	l.Tags = p.Tags
	if p.State != nil {
		if p.State.State != pb.States_UNKNOWN {
			l.SetState(protoToNativeState[p.State.State])
		} else {
			l.SetState(work.NotStarted)
		}
		l.SetReason(protoToNativeReason[p.State.Reason])
	} else {
		l.SetState(work.NotStarted)
		l.SetReason(work.NoFailure)
	}

	if p.Timing != nil {
		if p.Timing.Started != 0 {
			l.Started = time.Unix(p.Timing.Started, 0)
		}
		if p.Timing.Ended != 0 {
			l.Ended = time.Unix(p.Timing.Ended, 0)
		}
		if p.Timing.Submitted != 0 {
			l.Submitted = time.Unix(p.Timing.Submitted, 0)
		}
	}

	l.SetStorage(store)

	if err := nativeTask(l, p.Tasks); err != nil {
		return nil, err
	}

	return l, nil
}

// FromNative converts the native represenation into a protocol buffer for
// sending back via GRPC.  If data == false, Job.Args and Job.Output will not
// be set.
func FromNative(l *work.Labor, data bool) *pb.Labor {
	p := &pb.Labor{
		Name:     l.Name,
		Desc:     l.Desc,
		Id:       l.ID,
		ClientId: l.ClientID,
		Tags:     l.Tags,
		Timing: &pb.Timing{
			Started:   l.Started.Unix(),
			Ended:     l.Ended.Unix(),
			Submitted: l.Submitted.Unix(),
		},
		State: &pb.State{
			State:  nativeStateToProto[l.State()],
			Reason: nativeReasonToProto[l.Reason()],
		},
	}

	protoTask(p, l.Tasks, data)
	return p
}
