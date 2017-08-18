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
	"runtime"
	"testing"

	"github.com/golang/glog"
)

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

func TestTaskSignal(t *testing.T) {
	glog.Infof("TestTaskSignal")
	tests := []struct {
		desc   string
		state  State
		signal Signal
		want   State
	}{
		{
			desc:   "Error: Pause, but state wasn't running",
			state:  Completed,
			signal: Pause,
			want:   Completed,
		},
		{
			desc:   "Error: CrisisPause, but state wasn't running",
			state:  Completed,
			signal: CrisisPause,
			want:   Completed,
		},
		{
			desc:   "Error: AdminPause, but state wasn't running",
			state:  Completed,
			signal: AdminPause,
			want:   Completed,
		},
		{
			desc:   "Pause",
			state:  Running,
			signal: Pause,
			want:   Paused,
		},
		{
			desc:   "CrisisPause",
			state:  Running,
			signal: CrisisPause,
			want:   CrisisPaused,
		},
		{
			desc:   "AdminPause",
			state:  Running,
			signal: AdminPause,
			want:   AdminPaused,
		},

		{
			desc:   "Stop, but state wasn't an acceptable state",
			state:  Completed,
			signal: Stop,
			want:   Completed,
		},
		{
			desc:   "Stop from Paused",
			state:  Paused,
			signal: Stop,
			want:   Stopped,
		},
		{
			desc:   "Stop from AdminPaused",
			state:  AdminPaused,
			signal: Stop,
			want:   Stopped,
		},
		{
			desc:   "Stop from CrisisPaused",
			state:  CrisisPaused,
			signal: Stop,
			want:   Stopped,
		},
		{
			desc:   "Stop from Running",
			state:  CrisisPaused,
			signal: Stop,
			want:   Stopped,
		},
		{
			desc:   "Error: Resume from unsupported state",
			state:  Completed,
			signal: Resume,
			want:   Completed,
		},
		{
			desc:   "Resume from paused",
			state:  Paused,
			signal: Resume,
			want:   Running,
		},
		{
			desc:   "Error: CrisisResume from unsupported state",
			state:  Paused,
			signal: CrisisResume,
			want:   Paused,
		},
		{
			desc:   "CrisisResume from CrisisPaused",
			state:  CrisisPaused,
			signal: CrisisResume,
			want:   Running,
		},
		{
			desc:   "Error: AdminResume from unsupported state",
			state:  Paused,
			signal: AdminResume,
			want:   Paused,
		},
		{
			desc:   "CrisisResume from CrisisPaused",
			state:  CrisisPaused,
			signal: CrisisResume,
			want:   Running,
		},
	}

	for _, test := range tests {
		task := NewTask("", "", make(chan bool))
		task.Hub.SetState(test.state)

		task.Signal(test.signal)
		if task.Hub.State() != test.want {
			t.Errorf("Test %q: got state %v, want state %v", test.desc, task.Hub.State(), test.want)
		}
	}
}

// TODO(johnsiilver): Add a check for ToleratedFailures.
// TODO(johnsiilver): Add a check for PreChecks.
// TODO(johnsiilver): Add a check for ContinuousChecks.
