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

package testing

import (
	"context"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/google/marmot/work"
	"github.com/kylelemons/godebug/pretty"
	"github.com/pborman/uuid"

	_ "github.com/google/marmot/service/storage/inmemory"
)

var pcompare = pretty.Config{
	Diffable:          true,
	IncludeUnexported: false,
}

var base = &work.Labor{
	Meta: work.Meta{
		Name: "base",
		Desc: "base labor",
		ID:   uuid.New(),
	},
	Tasks: []*work.Task{
		{
			Meta: work.Meta{
				Name: "task0",
				Desc: "task 0",
				ID:   uuid.New(),
			},
			Sequences: []*work.Sequence{
				{
					Meta: work.Meta{ // Sequences don't use Name or Desc
						ID: uuid.New(),
					},
					Target: "target0",
					Jobs: []*work.Job{
						{
							Meta: work.Meta{ // Jobs don't use Name
								Desc: "job 0",
								ID:   uuid.New(),
							},
							CogPath: "plugin0",
						},
					},
				},
			},
		},
	},
}

// storeArgs is where to hold arguments needed to activate a storage
// implementation.  If not found here, it is assumed to be nil.
var storeArgs map[string]interface{}

// TestReadWrite tests most of the writer functions and all the reader functions.
func TestReadWrite(t *testing.T) {
	for name, f := range work.Registry {
		store := f(storeArgs[name])
		readWrite(t, name, store)
	}
}

func readWrite(t *testing.T, name string, store work.Storage) {
	ctx := context.Background()
	cp := copyLabor(*base)
	if err := store.WLabor(ctx, cp); err != nil {
		t.Fatalf("Test %q: .WLabor(): %s", name, err)
	}

	gotLabor, err := store.RLabor(ctx, cp.ID, true)
	if err != nil {
		t.Fatalf("Test %q: .RLabor(): %s", name, err)
	}

	if diff := pcompare.Compare(prepLabor(cp, true), prepLabor(gotLabor, true)); diff != "" {
		t.Fatalf("Test %q: .RLabor(): -want/+got:\n%s", name, diff)
	}
}

func TestSearch(t *testing.T) {
	for name, f := range work.Registry {
		glog.Infof("\n\n\nTesting TestSearch(%s)", name)
		store := f(storeArgs[name])
		search(t, name, store)
	}
}

func search(t *testing.T, name string, store work.Storage) {
	l0 := copyLabor(*base)
	l0.ID = uuid.New()
	l0.Submitted = time.Now().Add(-1 * time.Hour)
	l0.SetState(work.Running)
	l0.Tags = []string{"hello"}
	l0.Name = "prefix suffix"
	l0.SetStorage(store)
	ci, err := work.NewCogInfo(0)
	if err != nil {
		panic(err)
	}
	l0.SetCogInfo(ci)
	glog.Infof("%+v", l0.Tasks[0])
	mustStore(l0, store)

	l1 := copyLabor(*base)
	l1.ID = uuid.New()
	l1.Submitted = time.Now().Add(-10 * time.Minute)
	l1.SetState(work.Completed)
	l1.Tags = []string{"hello", "world"}
	l1.Name = "crap boo"
	l1.SetStorage(store)
	ci, err = work.NewCogInfo(0)
	if err != nil {
		panic(err)
	}
	l1.SetCogInfo(ci)
	mustStore(l1, store)

	tests := []struct {
		desc    string
		filter  work.LaborFilter
		results []*work.Labor
	}{
		{
			desc: "Find Running state",
			filter: work.LaborFilter{
				BaseFilter: work.BaseFilter{
					States: []work.State{work.Running},
				},
			},
			results: []*work.Labor{
				prepLabor(l0, true),
			},
		},
		{
			desc: "Find with Tags 'world'",
			filter: work.LaborFilter{
				Tags: []string{"world"},
			},
			results: []*work.Labor{
				prepLabor(l1, true),
			},
		},
		{
			desc: "Find with Tags 'hello'",
			filter: work.LaborFilter{
				Tags: []string{"hello"},
			},
			results: []*work.Labor{
				prepLabor(l0, true),
				prepLabor(l1, true),
			},
		},
		{
			desc: "Find submitted between 2 hours ago and 30 minutes",
			filter: work.LaborFilter{
				SubmitBegin: time.Now().Add(-2 * time.Hour),
				SubmitEnd:   time.Now().Add(-30 * time.Minute),
			},
			results: []*work.Labor{
				prepLabor(l0, true),
			},
		},
		{
			desc: "Find with name prefix 'prefix'",
			filter: work.LaborFilter{
				NamePrefix: "prefix",
			},
			results: []*work.Labor{
				prepLabor(l0, true),
			},
		},
		{
			desc: "Find with name suffix 'boo'",
			filter: work.LaborFilter{
				NameSuffix: "boo",
			},
			results: []*work.Labor{
				prepLabor(l1, true),
			},
		},
		{
			desc: "Find with tag 'hello' and in state Completed",
			filter: work.LaborFilter{
				BaseFilter: work.BaseFilter{
					States: []work.State{work.Completed},
				},
				NameSuffix: "boo",
			},
			results: []*work.Labor{
				prepLabor(l1, true),
			},
		},
	}

	for x, test := range tests {
		start := time.Now()
		ch, err := store.Labors(context.Background(), test.filter)
		if err != nil {
			t.Errorf("Testing store %q: Test %q: %s", name, test.desc, err)
			continue
		}

		results := []*work.Labor{}
		for entry := range ch {
			if entry.Error != nil {
				t.Errorf("Testing store %q: Test %q: %s", name, test.desc, err)
				continue
			}
			results = append(results, prepLabor(entry.Labor, true))
		}
		glog.Infof("Test %d: %v", x, time.Now().Sub(start))

		if diff := pcompare.Compare(test.results, results); diff != "" {
			t.Errorf("Testing store %q: Test %q: -want/+got\n%s", name, test.desc, diff)
		}
	}
}

func prepLabor(l *work.Labor, full bool) *work.Labor {
	cp := copyLabor(*l)
	cp.Completed = nil
	cp.Hub = nil
	if full {
		for _, t := range cp.Tasks {
			t.Hub = nil
			for _, j := range t.PreChecks {
				j.Hub = nil
			}
			for _, j := range t.ContChecks {
				j.Hub = nil
			}
			for _, s := range t.Sequences {
				s.Hub = nil
				for _, j := range s.Jobs {
					j.Hub = nil
				}
			}
		}
	} else {
		cp.Tasks = nil
	}
	return cp
}

// copyLabor makes a shallow copy of the labor itself, not of any of the
// sub-objects.
func copyLabor(l work.Labor) *work.Labor {
	l.Hub = work.NewHub()
	for _, task := range l.Tasks {
		task.Hub = work.NewHub()
		for _, job := range task.PreChecks {
			job.Hub = work.NewHub()
		}
		for _, job := range task.ContChecks {
			job.Hub = work.NewHub()
		}
		for _, seq := range task.Sequences {
			seq.Hub = work.NewHub()
			for _, job := range seq.Jobs {
				job.Hub = work.NewHub()
			}
		}
	}
	return &l
}

func mustStore(l *work.Labor, store work.Storage) {
	glog.Infof("mustStore: %+v", l.Tasks[0])
	if err := store.WLabor(context.Background(), l); err != nil {
		panic(err)
	}
}
