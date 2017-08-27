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
	pb "github.com/johnsiilver/cog/proto/cog"
)

// Job represents work to be done, via a Cog.
type Job struct {
	*Hub

	Meta

	// CogPath is the name of the Cog the Job will use.
	CogPath string

	// Args are the arguments to the Cog.
	Args string

	// ArgsType is the encoding method for the Args.
	ArgsType pb.ArgsType

	// Output is the output from the Cog.
	Output string

	// Timeout is the amount of time a Job is allowed to run.
	Timeout time.Duration

	// Retries is the number of times to retry a Cog that fails.
	Retries int

	// RetryDelay is how long to wait between retrying a Cog.
	RetryDelay time.Duration

	// RealUser is the username of user Submitting the Labor to Marmot.
	RealUser string

	// cogInfo contains information about Cogs.
	cogInfo *CogInfo

	// server contains information about this server instance.
	server *pb.Server

	// store holds the storage layer for data.
	store Storage
}

// NewJob is the constructor for Job.
func NewJob(cogPath, desc string) *Job {
	j := &Job{
		CogPath: cogPath,
		Meta:    Meta{Desc: desc},
		Hub:     NewHub(),
	}
	j.SetState(NotStarted)
	return j
}

// Validate validates Job's attributes.
func (j *Job) Validate() error {
	if j.State() != NotStarted {
		return fmt.Errorf("Job state must be NotStarted, was: %v", j.State())
	}

	if err := j.Meta.Validate(); err != nil {
		return err
	}

	if j.CogPath == "" {
		return fmt.Errorf("Job.Cog cannot be an empty string")
	}

	if j.Args == "" {
		return fmt.Errorf("Jobs.Args cannot be empty string")
	}

	if j.ArgsType == pb.ArgsType_AT_UNKNOWN {
		return fmt.Errorf("Jobs.ArgsType cannot be unknown")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if !j.cogInfo.client.Loaded(j.CogPath) {
		if err := j.cogInfo.client.Load(ctx, j.CogPath); err != nil {
			return fmt.Errorf("could not load Cog %q: %s", j.CogPath, err)
		}
	}

	err := j.cogInfo.client.Validate(ctx, j.CogPath, []byte(j.Args), j.ArgsType)
	if err != nil {
		return fmt.Errorf("Jobs.Args did not validate: %s", err)
	}
	return nil
}

// execute executes this Task.
func (j *Job) execute(wg *sync.WaitGroup) {
	defer wg.Done()

	jobsProc.Add(1)
	jobsExec.Add(1)
	defer jobsExec.Add(-1)
	j.Started = time.Now()

	j.SetState(Running)

	var (
		out   []byte
		state State
		retry bool
		err   error
	)

	for i := -1; i < j.Retries; i++ { // i := -1, because we always attempt once, even if retries are 0.
		ctx, cancel := context.WithTimeout(context.Background(), j.Timeout)
		out, state, retry, err = executeCog(
			ctx,
			executeArgs{
				j.CogPath,
				j.RealUser,
				[]byte(j.Args),
				j.ArgsType,
				j.cogInfo,
				j.server,
			},
		)
		cancel()
		glog.Infof("right after executeCog: %v", state)

		if state == Completed || !retry {
			break
		}

		retryAfter := time.Now().Add(j.RetryDelay)
		for {
			if time.Now().After(retryAfter) {
				break
			}
			time.Sleep(1 * time.Second)
		}
	}

	if err != nil {
		j.Output = err.Error()
	} else {
		j.Output = string(out)
	}
	glog.Infof("the state before SetState: %v", state)
	j.SetState(state)
}

// SetState overrides the internal Hub.SetState() adding writing to storage.
func (j *Job) SetState(st State) {
	glog.Infof("Job setting state: %v", st)
	j.Hub.SetState(st)
	switch st {
	case Completed, Failed, Stopped:
		j.Ended = time.Now()
	}
	//glog.Infof("wrote time: %v", j.Ended)
	if j.store != nil && j.ID != "" {
		glog.Infof("Job wrote storage")
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := j.store.WJob(ctx, j)
			cancel()
			if err != nil {
				time.Sleep(time.Duration(random.Intn(5)) * time.Second)
				continue
			}
			break
		}
	}
}
