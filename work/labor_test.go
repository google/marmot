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
	"testing"

	"github.com/golang/glog"
)

func TestLaborSignal(t *testing.T) {
	glog.Infof("TestLaborSignal")
	tests := []struct {
		desc   string
		state  State
		signal Signal
		err    bool
		want   State
	}{
		{
			desc:   "Error: Pause, but state wasn't running",
			state:  Completed,
			signal: Pause,
			err:    true,
		},
		{
			desc:   "Error: CrisisPause, but state wasn't running",
			state:  Completed,
			signal: CrisisPause,
			err:    true,
		},
		{
			desc:   "Error: AdminPause, but state wasn't running",
			state:  Completed,
			signal: AdminPause,
			err:    true,
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
			desc:   "Error: Stop, but state wasn't an acceptable state",
			state:  Completed,
			signal: Stop,
			err:    true,
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
			err:    true,
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
			err:    true,
		},
		{
			desc:   "CrisisResume from CrisisPaused",
			state:  CrisisPaused,
			signal: CrisisResume,
			want:   Running,
		},
	}

	for _, test := range tests {
		l := NewLabor("", "")
		l.Hub.SetState(test.state)
		err := l.Signal(test.signal)
		switch {
		case err == nil && test.err:
			t.Errorf("Test %q: got err == nil, want err != nil", test.desc)
			continue
		case err != nil && !test.err:
			t.Errorf("Test %q: got err == %q, want err == nil", test.desc, err)
			continue
		case err != nil:
			continue
		}

		if l.Hub.State() != test.want {
			t.Errorf("Test %q: got state %v, want state %v", test.desc, l.Hub.State(), test.want)
		}
	}
}

func TestStart(t *testing.T) {
	glog.Infof("TestStart")
	tests := []struct {
		desc  string
		state State
		err   bool
	}{
		{
			desc:  "Error: Not in supported state",
			state: Running,
			err:   true,
		},
		{
			desc:  "Success: NotStarted state",
			state: NotStarted,
		},
		{
			desc:  "Success: AdminNotStarted state",
			state: AdminNotStarted,
		},
	}

	for _, test := range tests {
		l := NewLabor("", "")
		l.Hub.SetState(test.state)
		err := l.Start()
		switch {
		case err == nil && test.err:
			t.Errorf("Test %q: got err == nil, want err != nil", test.desc)
			continue
		case err != nil && !test.err:
			t.Errorf("Test %q: got err == %q, want err == nil", test.desc, err)
			continue
		case err != nil:
			continue
		}
	}
}
