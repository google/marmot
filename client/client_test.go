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
	"context"
	"os"
	"path"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/google/marmot/instance"
	pb "github.com/google/marmot/testing/cogs/proto/tester"
	"github.com/kylelemons/godebug/pretty"
)

var (
	testCog     = "localFile://" + path.Join(os.Getenv("GOPATH"), "src/github.com/google/marmot/testing/cogs/tester/tester")
	successArgs = &pb.Args{Success: true, Message: "Success"}
	failureArgs = &pb.Args{Failure: true, Message: "Failure"}
)

func TestInsecure(t *testing.T) {
	tests := []struct {
		desc    string
		cogArgs *pb.Args
		result  State
	}{
		{
			desc:    "Plugin returns success",
			cogArgs: successArgs,
			result:  Completed,
		},
		{
			desc:    "Plugin returns failure",
			cogArgs: failureArgs,
			result:  Failed,
		},
	}

	for _, test := range tests {
		args := instance.Args{
			SIP:        "localhost",
			SPort:      0,
			MaxCrashes: 3,
			Insecure:   true,
			CertFile:   "",
			KeyFile:    "",
			Storage:    "inmemory",
		}
		inst, err := instance.New(args)
		if err != nil {
			t.Fatal(err)
		}
		defer inst.Stop()

		go inst.Run()
		time.Sleep(3 * time.Second)

		build, err := NewBuilder(
			"test",
			"labor desc",
		)
		if err != nil {
			t.Fatal(err)
		}

		ctrl, err := NewControl(inst.Addr(), Insecure())
		if err != nil {
			t.Fatal(err)
		}

		build.AddTask("task", "task desc")
		build.AddSequence("target", "seq desc")
		build.AddJob(testCog, "job desc", test.cogArgs)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		id, err := ctrl.Submit(ctx, build.Labor(), true)
		if err != nil {
			t.Fatal(err)
		}

		if err = ctrl.Wait(ctx, id); err != nil {
			t.Fatal(err)
		}

		l, err := ctrl.FetchLabor(ctx, id, true)
		if err != nil {
			t.Fatal(err)
		}
		glog.Infof(pretty.Sprint(l))
		if l.Tasks[0].Sequences[0].Jobs[0].State != test.result {
			t.Fatalf("Test %s: job status: got %v, want %v", test.desc, l.Tasks[0].Sequences[0].Jobs[0], test.result)
		}
		if l.Tasks[0].Sequences[0].State != test.result {
			t.Fatalf("Test %s: sequence status: got %v, want %v", test.desc, l.Tasks[0].Sequences[0].State, test.result)
		}
		if l.Tasks[0].State != test.result {
			t.Fatalf("Test %s: task status: got %v, want %v", test.desc, l.Tasks[0].State, test.result)
		}
		if l.State != test.result {
			t.Fatalf("Test %s: labor status: got state %v, want state %v", test.desc, l.State, test.result)
		}
	}
}
