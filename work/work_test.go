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
	"bytes"
	"os"
	"path"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/glog"
	"github.com/golang/protobuf/jsonpb"
)

const tester = "tester"

func cogPath(cog string) string {
	return "localFile://" + path.Join(os.Getenv("GOPATH"), "src/github.com/google/marmot/testing/cogs/", cog, cog)
}

func TestRun(t *testing.T) {
	glog.Infof("Test run")
	defer glog.Infof("Test run completed")

	done := make(chan bool)

	labor := NewLabor("hello", "world")
	labor.Hub.SetState(NotStarted)

	task1 := NewTask("task1", "", done)
	task1.Sequences = append(task1.Sequences, NewSequence("dev1", done))
	task1.Sequences = append(task1.Sequences, NewSequence("dev2", done))
	task1.Sequences[0].Jobs = append(task1.Sequences[0].Jobs, NewJob(cogPath(tester), "job1"))
	task1.Sequences[0].Jobs = append(task1.Sequences[0].Jobs, NewJob(cogPath(tester), "job2"))
	task1.Sequences[1].Jobs = append(task1.Sequences[1].Jobs, NewJob(cogPath(tester), "job3"))
	task1.Sequences[1].Jobs = append(task1.Sequences[1].Jobs, NewJob(cogPath(tester), "job4"))

	labor.Tasks = append(labor.Tasks, task1)

	ci, err := NewCogInfo(0)
	if err != nil {
		panic(err)
	}
	labor.SetCogInfo(ci)
	labor.SetIDs()
	labor.Start()
	<-labor.Completed

	glog.Infof("Final state was: %v", labor.Hub.State())
	for _, task := range labor.Tasks {
		glog.Infof("\tTask %q: %d", task.Meta.Name, task.Hub.State())
		for _, seq := range task.Sequences {
			glog.Infof("\t\tSequence %q: %d", seq.Meta.Name, seq.Hub.State())
			for _, job := range seq.Jobs {
				glog.Infof("\t\t\tJob %q: %d", job.Meta.Name, job.Hub.State())
			}
		}
	}
}

var jsonMarshaller = jsonpb.Marshaler{Indent: "\t"}

func mustProtoToJSON(p proto.Message) []byte {
	buff := new(bytes.Buffer)
	if err := jsonMarshaller.Marshal(buff, p); err != nil {
		panic(err)
	}
	return buff.Bytes()
}

func TestSignalDrain(t *testing.T) {
	glog.Infof("TestSignalDrain")

	ex0 := make(chan Signal, 1)
	ex1 := make(chan Signal, 1)
	ack0 := make(chan struct{}, 1)
	ack1 := make(chan struct{}, 1)

	wd := make(chan bool)

	sd := newSignalDrain(wd)

	sd.drain()

	ex0 <- Stop
	sd.addCh(ex0, ack0)
	<-ack0

	sd.addCh(ex1, ack1)
	ex0 <- Stop
	ex1 <- Stop
	<-ack0
	<-ack1

	close(wd)
	<-sd.done
}
