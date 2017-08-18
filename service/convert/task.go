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

func nativeTask(l *work.Labor, pTasks []*pb.Task) error {
	tasks := make([]*work.Task, 0, len(pTasks))
	for _, t := range pTasks {
		task := work.NewTask(t.Name, t.Desc, l.Completed)
		task.ID = t.Id
		if t.Timing != nil {
			if t.Timing.Started != 0 {
				task.Started = time.Unix(t.Timing.Started, 0)
			}
			if t.Timing.Ended != 0 {
				task.Ended = time.Unix(t.Timing.Ended, 0)
			}
		}
		task.ToleratedFailures = int(t.ToleratedFailures)
		if t.Concurrency < 1 {
			task.Concurrency = 1
		} else {
			task.Concurrency = int(t.Concurrency)
		}
		task.ContCheckInterval = time.Duration(t.ContCheckInterval) * time.Second
		task.PassFailures = t.PassFailures

		if t.State != nil {
			if t.State.State != pb.States_UNKNOWN {
				task.SetState(protoToNativeState[t.State.State])
			} else {
				task.SetState(work.NotStarted)
			}
			task.SetReason(protoToNativeReason[t.State.Reason])
		} else {
			task.SetState(work.NotStarted)
			task.SetReason(work.NoFailure)
		}

		task.PreChecks = []*work.Job{}
		task.ContChecks = []*work.Job{}
		if err := handleNativeJob(&task.PreChecks, t.PreChecks); err != nil {
			return err
		}
		if err := handleNativeJob(&task.ContChecks, t.ContChecks); err != nil {
			return err
		}

		if err := nativeSequence(task, t.Sequences, l.Completed); err != nil {
			return err
		}
		tasks = append(tasks, task)
	}
	l.Tasks = tasks
	return nil
}

func protoTask(p *pb.Labor, nTasks []*work.Task, data bool) {
	tasks := make([]*pb.Task, 0, len(nTasks))
	for _, t := range nTasks {
		task := &pb.Task{
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
				State:  nativeStateToProto[t.State()],
				Reason: nativeReasonToProto[t.Reason()],
			},
			PreChecks:  []*pb.Job{},
			ContChecks: []*pb.Job{},
		}

		handleProtoJob(&task.PreChecks, t.PreChecks, data)
		handleProtoJob(&task.ContChecks, t.ContChecks, data)
		protoSequence(task, t.Sequences, data)

		tasks = append(tasks, task)
	}
	p.Tasks = tasks
}
