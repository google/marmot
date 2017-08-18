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

	"github.com/golang/glog"
	cpb "github.com/johnsiilver/cog/proto/cog"
	pb "github.com/google/marmot/proto/marmot"
	"github.com/google/marmot/work"
)

func handleNativeJob(jobs *[]*work.Job, pJobs []*pb.Job) error {
	*jobs = make([]*work.Job, 0, len(pJobs))
	for _, j := range pJobs {
		job := work.NewJob(j.Plugin, j.Desc)
		job.ID = j.Id
		job.Args = j.Args
		job.ArgsType = cpb.ArgsType_JSON
		job.Output = j.Output
		job.CogPath = j.Plugin

		if j.Timing != nil {
			job.Started = time.Unix(j.Timing.Started, 0)
			job.Ended = time.Unix(j.Timing.Ended, 0)
		}

		if j.State != nil {
			if j.State.State != pb.States_UNKNOWN {
				job.SetState(protoToNativeState[j.State.State])
			} else {
				job.SetState(work.NotStarted)
			}
			job.SetReason(protoToNativeReason[j.State.Reason])
		} else {
			job.SetState(work.NotStarted)
			job.SetReason(work.NoFailure)
		}

		if j.Timeout != 0 {
			job.Timeout = time.Duration(j.Timeout) * time.Second
		} else {
			job.Timeout = 5 * time.Minute
		}

		*jobs = append(*jobs, job)
	}
	return nil
}

func handleProtoJob(jobs *[]*pb.Job, nJobs []*work.Job, data bool) {
	*jobs = make([]*pb.Job, 0, len(nJobs))
	for _, j := range nJobs {
		glog.Infof("native state: %v, proto state: %v", j.State(), nativeStateToProto[j.State()])
		job := &pb.Job{
			Id:     j.ID,
			Desc:   j.Desc,
			Plugin: j.CogPath,
			Timing: &pb.Timing{
				Started: j.Started.Unix(),
				Ended:   j.Ended.Unix(),
			},
			State: &pb.State{
				State:  nativeStateToProto[j.State()],
				Reason: nativeReasonToProto[j.Reason()],
			},
		}
		if data {
			job.Args = j.Args
			job.Output = j.Output
		}
		*jobs = append(*jobs, job)
	}
}
