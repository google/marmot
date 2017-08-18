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
	"time"
)

// Registry holds a register of names to store implementations.
// Public to allow testing.
var Registry = map[string]func(args interface{}) Storage{}

// RegisterStore registers a storage method that can be utilized. Should only
// be called during init() and duplicate names panic.
func RegisterStore(name string, f func(interface{}) Storage) {
	if Registry[name] != nil {
		panic("storage registry name conflict: " + name)
	}
	Registry[name] = f
}

// Storage provides access to the underlying storage system.
type Storage interface {
	Read
	Write
	Search
}

// Read provides read access to data in Storage.  All methods must return
// copies of the containers, not the actual containers.
type Read interface {
	// Labor reads a Labor from storage.  If full==false, it only returns the
	// Labor data itself and none of the sub-containers.
	RLabor(ctx context.Context, id string, full bool) (*Labor, error)

	// Task reads a Task from storage. This only returns the Task data.
	RTask(ctx context.Context, id string) (*Task, error)

	// Sequence reads a Sequence from storage.  It only returns the Sequence
	// data and non of the sub-containers.
	RSequence(ctx context.Context, id string) (*Sequence, error)

	// Job reads a Job from storage.  If full==false, it will not return the
	// Job.Args or Job.Output.
	RJob(ctx context.Context, id string, full bool) (*Job, error)
}

// Write provides write access to data in Storage.
type Write interface {
	// Labor writes the Labor and sub-containers to storage.
	// This should only be used either on ingest of a Labor or during a
	// recovery of a Labor.
	WLabor(ctx context.Context, l *Labor) error

	// WLaborState allows updating of the Labor's State and Reason.
	WLaborState(ctx context.Context, l *Labor) error

	// Task writes out a Task to storage.  This only writes the Task data, none
	// of the sub-containers.
	WTask(ctx context.Context, t *Task) error

	// Sequence writes out a Sequence to storage.  This only writes the Sequence
	// data, none of the sub-containers.
	WSequence(ctx context.Context, s *Sequence) error

	// Job writes out a Job to storage.  This only writes the Job data.  It does
	// not write the Job.Args, as they should only be recorded once by a call to
	// Write.Labor().
	WJob(ctx context.Context, j *Job) error
}

// BaseFilter is a basic filter that is included in all search related filters.
type BaseFilter struct {
	// States returns objects that are currently at one the "states".
	// If empty, all states are included.
	States []State
}

// LaborFilter is a search filter used for searching for Labors.
type LaborFilter struct {
	BaseFilter

	// NamePrefix locates Labors starting with the string.
	// NameSuffix locates Labors ending with the string.
	NamePrefix, NameSuffix string

	// Tags matches any Labor that has any tag listed.
	Tags []string

	// SubmitBegin includes only Labors that were submitted at or after
	// SubmitBegin.
	// SubmitEnd will only include an object which was submitted before SubmitEnd.
	SubmitBegin, SubmitEnd time.Time
}

// SearchResult is the result of a Search.Labors().
type SearchResult struct {
	// Labor is the found Labor.
	Labor *Labor

	// Error indicates the stream had an error.
	Error error
}

// Search provides search access to data in Storage.
type Search interface {
	// Labors streams a time ordered stream of Labors matching the filter.
	// They are ordered from earliest submitted.  They do not contain data.
	Labors(ctx context.Context, filter LaborFilter) (chan SearchResult, error)
}
