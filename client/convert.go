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

// convert.go holds various maps that are needed to convert between the proto
// format and the native format used in the client.

import (
	cpb "github.com/johnsiilver/cog/proto/cog"
	pb "github.com/google/marmot/proto/marmot"
)

var nativeStateToProto = map[State]pb.States{
	UnknownState:    pb.States_UNKNOWN,
	NotStarted:      pb.States_NOT_STARTED,
	AdminNotStarted: pb.States_ADMIN_NOT_STARTED,
	Running:         pb.States_RUNNING,
	Pausing:         pb.States_PAUSING,
	Paused:          pb.States_PAUSED,
	AdminPaused:     pb.States_ADMIN_PAUSED,
	CrisisPaused:    pb.States_CRISIS_PAUSED,
	Completed:       pb.States_COMPLETED,
	Failed:          pb.States_FAILED,
	Stopping:        pb.States_STOPPING,
	Stopped:         pb.States_STOPPED,
}

var protoToNativeState map[pb.States]State

// Create the reverse of nativeStateToProto dynamically.
func init() {
	protoToNativeState = make(map[pb.States]State, len(nativeStateToProto))
	for k, v := range nativeStateToProto {
		protoToNativeState[v] = k
	}
}

var nativeReasonToProto = map[Reason]pb.Reasons{
	NoFailure:        pb.Reasons_NO_FAILURE,
	PreCheckFailure:  pb.Reasons_PRE_CHECK_FAILURE,
	ContCheckFailure: pb.Reasons_CONT_CHECK_FAILURE,
	MaxFailures:      pb.Reasons_MAX_FAILURES,
}

var protoToNativeReason map[pb.Reasons]Reason

// Create the reverse of nativeReasonToProto dynamically.
func init() {
	protoToNativeReason = make(map[pb.Reasons]Reason, len(nativeReasonToProto))
	for k, v := range nativeReasonToProto {
		protoToNativeReason[v] = k
	}
}

var nativeArgsTypeToProto = map[ArgsType]cpb.ArgsType{
	ATUnknown: cpb.ArgsType_AT_UNKNOWN,
	ATJSON:    cpb.ArgsType_JSON,
}

var protoToNativeArgsType map[cpb.ArgsType]ArgsType

// Create the reverse of nativeStateToProto dynamically.
func init() {
	protoToNativeArgsType = make(map[cpb.ArgsType]ArgsType, len(nativeArgsTypeToProto))
	for k, v := range nativeArgsTypeToProto {
		protoToNativeArgsType[v] = k
	}
}
