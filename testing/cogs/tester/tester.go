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

package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/johnsiilver/cog"
	pb "github.com/johnsiilver/cog/proto/cog"
	tpb "github.com/google/marmot/testing/cogs/proto/tester"
	"golang.org/x/net/context"
)

type testCog struct {
}

func (t *testCog) Execute(ctx context.Context, args proto.Message, realUser, endpoint, id string) (cog.Out, error) {
	a := args.(*tpb.Args)

	switch true {
	case a.Success:
		return cog.Out{Status: cog.Success, Output: &tpb.Response{Message: a.Message}}, nil
	case a.Failure:
		return cog.Out{Status: cog.Failure, Output: &tpb.Response{Message: a.Message}}, nil
	case a.FailureNoRetries:
		return cog.Out{Status: cog.FailureNoRetry, Output: &tpb.Response{Message: a.Message}}, nil
	case a.Wait > 0:
		after := time.Now().Add(time.Duration(a.Wait) * time.Second)
		for {
			select {
			case <-ctx.Done():
				return cog.Out{}, fmt.Errorf("plugin reached context deadline")
			default:
				// Do nothing
			}
			if time.Now().After(after) {
				return cog.Out{Status: cog.Success, Output: &tpb.Response{Message: a.Message}}, nil
			}
			time.Sleep(1 * time.Second)
		}
	}
	return cog.Out{Status: cog.FailureNoRetry, Output: &tpb.Response{Message: "invalid args"}}, nil
}

func (t *testCog) Describe() *pb.Description {
	return &pb.Description{
		Owner:       "johnsiilver",
		Description: "Plugin desc",
		Tags:        []string{"yo", "mama"},
	}
}

func (t *testCog) Validate(p proto.Message) error {
	a, ok := p.(*tpb.Args)
	if !ok {
		return fmt.Errorf("wrong type of args")
	}

	if !(a.Success || a.Failure || a.FailureNoRetries || a.Wait > 0) {
		return fmt.Errorf("must set one of Succes/Failure/FailureNoRetries/Wait")
	}
	return nil
}

func (t *testCog) ArgsProto() proto.Message {
	return &tpb.Args{}
}

func main() {
	flag.Parse()
	if err := cog.Start(&testCog{}); err != nil {
		panic(err)
	}
	select {}
}
