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
	"time"

	pb "github.com/google/marmot/proto/marmot"
	"github.com/google/marmot/work"
)

func nativeSequence(t *work.Task, pSequence []*pb.Sequence, done chan bool) error {
	seqs := make([]*work.Sequence, 0, len(pSequence))
	for _, s := range pSequence {
		seq := work.NewSequence(s.Target, done)
		seq.ID = s.Id
		seq.Desc = s.Desc
		if s.Timing != nil {
			if s.Timing.Started != 0 {
				seq.Started = time.Unix(s.Timing.Started, 0)
			}
			if s.Timing.Ended != 0 {
				seq.Ended = time.Unix(s.Timing.Ended, 0)
			}
		}

		if s.State != nil {
			if s.State.State != pb.States_UNKNOWN {
				seq.SetState(protoToNativeState[s.State.State])
			} else {
				seq.SetState(work.NotStarted)
			}
			seq.SetReason(protoToNativeReason[s.State.Reason])
		} else {
			seq.SetState(work.NotStarted)
			seq.SetReason(work.NoFailure)
		}

		if seq.Jobs == nil {
			seq.Jobs = []*work.Job{}
		}
		if err := handleNativeJob(&seq.Jobs, s.Jobs); err != nil {
			return err
		}
		seqs = append(seqs, seq)
	}
	t.Sequences = seqs
	return nil
}

func protoSequence(p *pb.Task, nSequence []*work.Sequence, data bool) {
	seqs := make([]*pb.Sequence, 0, len(nSequence))
	for _, s := range nSequence {
		seq := &pb.Sequence{
			Target: s.Target,
			Desc:   s.Desc,
			Id:     s.ID,
			Timing: &pb.Timing{
				Started: s.Started.Unix(),
				Ended:   s.Ended.Unix(),
			},
			State: &pb.State{
				State:  nativeStateToProto[s.State()],
				Reason: nativeReasonToProto[s.Reason()],
			},
			Jobs: []*pb.Job{},
		}
		handleProtoJob(&seq.Jobs, s.Jobs, data)

		seqs = append(seqs, seq)
	}
	p.Sequences = seqs
}
