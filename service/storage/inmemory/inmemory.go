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

// Package inmemory provides an inmemory storage mechanism for Labors as they
// get processed.  This is useful for testing and plugin development when you
// don't want to turn up a database for storage.
// Note: Search functionality is impractical, most likely because I choose
// bad data layout schemes for searching.  This should be fine, because of the
// use case that is being solved here.
package inmemory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/marmot/work"
	"github.com/petar/GoLLRB/llrb"
)

func init() {
	f := func(args interface{}) work.Storage {
		return &storage{
			labors:    llrb.New(),
			tasks:     llrb.New(),
			sequences: llrb.New(),
			jobs:      llrb.New(),

			bySubmit: llrb.New(),
			byTag:    map[string]byStarts{},
			byState:  map[work.State]byStarts{},
		}
	}
	work.RegisterStore("inmemory", f)
}

type started struct {
	started time.Time
	id      string
}

type byStarts []started

func (a byStarts) Len() int            { return len(a) }
func (a byStarts) Swap(i, j int)       { a[i], a[j] = a[j], a[i] }
func (a byStarts) Less(i, j int) bool  { return a[j].started.After(a[i].started) }
func (a *byStarts) delete(id string) { // n runtime, but this should be fine.
	index := -1
	for i, entry := range *a {
		if entry.id == id {
			index = i
			break
		}
	}
	if index != -1 {
		b := append((*a)[:index], (*a)[index+1:]...)
		*a = b
	}
}
func (a byStarts) intersection(b byStarts) byStarts { // Slowwwwwwwwwwwww
	ret := byStarts{}
	f := make(map[string]bool, len(a))
	for _, entry := range a {
		f[entry.id] = true
	}

	for _, entry := range b {
		if f[entry.id] {
			ret = append(ret, entry)
		}
	}

	return ret
}

type row struct {
	id        string
	name      string
	desc      string
	started   time.Time
	ended     time.Time
	submitted time.Time
	state     work.State
	reason    work.Reason
}

type labor struct {
	row
	clientID string
	tags     []string
	tasks    []string
}

func (l labor) to() *work.Labor {
	labor := work.NewLabor(l.name, l.desc)
	labor.ID = l.id
	labor.ClientID = l.clientID
	labor.Tags = l.tags
	labor.Started = l.started
	labor.Ended = l.ended
	labor.Submitted = l.submitted
	labor.SetState(l.state)
	labor.SetReason(l.reason)
	labor.Completed = make(chan bool)
	return labor
}

func (l *labor) from(o *work.Labor) {
	l.tags = make([]string, len(o.Tags))
	l.tasks = make([]string, 0, len(o.Tasks))

	l.id = o.ID
	l.clientID = o.ClientID
	l.tags = o.Tags
	l.name = o.Name
	l.desc = o.Desc
	l.started = o.Started
	l.ended = o.Ended
	l.submitted = o.Submitted
	l.state = o.State()
	l.reason = o.Reason()
	copy(l.tags, o.Tags)
	for _, t := range o.Tasks {
		l.tasks = append(l.tasks, t.ID)
	}
}

type task struct {
	row
	preChecks         []string
	contChecks        []string
	sequences         []string
	toleratedFailures int
	concurrency       int
	contCheckInterval time.Duration
	passFailures      bool
}

func (t task) to(done chan bool) *work.Task {
	task := work.NewTask(t.name, t.desc, done)
	task.ID = t.id
	task.Started = t.started
	task.Ended = t.ended
	task.SetState(t.state)
	task.SetReason(t.reason)
	task.ToleratedFailures = t.toleratedFailures
	task.Concurrency = t.concurrency
	task.ContCheckInterval = t.contCheckInterval
	task.PassFailures = t.passFailures
	return task
}

func (t *task) from(o *work.Task) {
	if o.ID == "" {
		panic("a Task must have an ID. You are probably in test code, which should alwasy set different IDs for Tasks")
	}
	t.preChecks = make([]string, 0, len(o.PreChecks))
	t.contChecks = make([]string, 0, len(o.ContChecks))
	t.sequences = make([]string, 0, len(o.Sequences))

	t.id = o.ID
	t.name = o.Name
	t.desc = o.Desc
	t.started = o.Started
	t.ended = o.Ended
	t.state = o.State()
	t.reason = o.Reason()
	t.toleratedFailures = o.ToleratedFailures
	t.concurrency = o.Concurrency
	t.contCheckInterval = o.ContCheckInterval
	t.passFailures = o.PassFailures

	for _, j := range o.PreChecks {
		t.preChecks = append(t.preChecks, j.ID)
	}
	for _, j := range o.ContChecks {
		t.contChecks = append(t.contChecks, j.ID)
	}
	for _, s := range o.Sequences {
		t.sequences = append(t.sequences, s.ID)
	}
}

type sequence struct {
	row
	target string
	jobs   []string
}

func (s sequence) to(done chan bool) *work.Sequence {
	seq := work.NewSequence(s.target, done)
	seq.ID = s.id
	seq.Desc = s.desc
	seq.Started = s.started
	seq.Ended = s.ended
	seq.SetState(s.state)
	seq.SetReason(s.reason)
	return seq
}

func (s *sequence) from(o *work.Sequence) {
	if o.ID == "" {
		panic("a Sequence must have an ID. You are probably in test code, which should alwasy set different IDs for Sequences")
	}
	s.jobs = make([]string, 0, len(o.Jobs))
	s.id = o.ID
	s.name = o.Name
	s.desc = o.Desc
	s.target = o.Target
	s.started = o.Started
	s.ended = o.Ended
	s.state = o.State()
	s.reason = o.Reason()

	for _, j := range o.Jobs {
		s.jobs = append(s.jobs, j.ID)
	}
}

type job struct {
	row
	cogPath string
	args    []byte
	output  []byte
}

func (j job) to() *work.Job {
	job := work.NewJob(j.cogPath, j.desc)
	job.ID = j.id
	job.Desc = j.desc
	job.Args = string(j.args)
	job.Output = string(j.output)
	job.Started = j.started
	job.Ended = j.ended
	job.SetState(j.state)
	job.SetReason(j.reason)
	return job
}

func (j *job) from(o *work.Job) {
	if o.ID == "" {
		panic("a Job must have an ID. You are probably in test code, which should alwasy set different IDs for Jobs")
	}
	j.args = make([]byte, len(o.Args))
	j.output = make([]byte, len(o.Output))

	j.id = o.ID
	j.name = o.Name
	j.desc = o.Desc
	j.cogPath = o.CogPath
	j.started = o.Started
	j.ended = o.Ended
	glog.Infof("Write inmemory state: %v", o.State())
	j.state = o.State()
	j.reason = o.Reason()
	copy(j.args, o.Args)
	copy(j.output, o.Output)
}

func (j *job) updateOutput(out []byte) {
	j.output = out
}

type item struct {
	key   string
	value interface{}
}

func (i item) Less(than llrb.Item) bool {
	if i.key < than.(item).key {
		return true
	}
	return false
}

type itemDate struct {
	key   time.Time
	value string
}

func (i itemDate) Less(than llrb.Item) bool {
	if i.key.After(than.(itemDate).key) {
		return false
	}
	return true
}

type storage struct {
	labors    *llrb.LLRB
	tasks     *llrb.LLRB
	sequences *llrb.LLRB
	jobs      *llrb.LLRB

	bySubmit *llrb.LLRB
	byTag    map[string]byStarts
	byState  map[work.State]byStarts

	laborsMu    sync.RWMutex
	tasksMu     sync.RWMutex
	sequencesMu sync.RWMutex
	jobsMu      sync.RWMutex
}

// RLabor implements work.Read.RLabor.
func (s *storage) RLabor(ctx context.Context, id string, full bool) (*work.Labor, error) {
	s.laborsMu.RLock()
	defer s.laborsMu.RUnlock()

	i := s.labors.Get(item{key: id})
	if i == nil {
		return nil, fmt.Errorf("Labor %q does not exist", id)
	}

	lrow := i.(item).value.(labor)

	if !full {
		return lrow.to(), nil
	}

	lnative := lrow.to()
	tasks, err := s.addTasks(lrow.tasks, lnative.Completed, full)
	if err != nil {
		return nil, err
	}
	lnative.Tasks = tasks
	return lnative, nil
}

func (s *storage) addTasks(ids []string, done chan bool, full bool) ([]*work.Task, error) {
	s.tasksMu.RLock()
	defer s.tasksMu.RUnlock()

	wt := make([]*work.Task, 0, len(ids))
	for _, id := range ids {
		ti := s.tasks.Get(item{key: id})
		if ti == nil {
			return nil, fmt.Errorf("Task %q could not be found", id)
		}

		trow := ti.(item).value.(task)
		tnative := trow.to(done)

		preChecks, err := s.addJobs(trow.preChecks, full)
		if err != nil {
			return nil, err
		}
		tnative.PreChecks = preChecks

		contChecks, err := s.addJobs(trow.contChecks, full)
		if err != nil {
			return nil, err
		}
		tnative.ContChecks = contChecks

		sequences, err := s.addSequences(trow.sequences, done, full)
		if err != nil {
			return nil, err
		}
		tnative.Sequences = sequences

		wt = append(wt, tnative)
	}
	return wt, nil
}

func (s *storage) addSequences(ids []string, done chan bool, full bool) ([]*work.Sequence, error) {
	s.sequencesMu.RLock()
	defer s.sequencesMu.RUnlock()

	ws := make([]*work.Sequence, 0, len(ids))
	for _, id := range ids {
		si := s.sequences.Get(item{key: id})
		if si == nil {
			return nil, fmt.Errorf("Sequence %q could not be found", id)
		}

		srow := si.(item).value.(sequence)
		snative := srow.to(done)

		jobs, err := s.addJobs(srow.jobs, full)
		if err != nil {
			return nil, err
		}
		snative.Jobs = jobs
		ws = append(ws, snative)
	}
	return ws, nil
}

func (s *storage) addJobs(ids []string, full bool) ([]*work.Job, error) {
	s.jobsMu.RLock()
	defer s.jobsMu.RUnlock()

	wj := make([]*work.Job, 0, len(ids))
	for _, id := range ids {
		ji := s.jobs.Get(item{key: id})
		if ji == nil {
			return nil, fmt.Errorf("Job %q could not be found", id)
		}

		jrow := ji.(item).value.(job)
		if !full {
			jrow.output = nil
			jrow.args = nil
		}
		wj = append(wj, jrow.to())
	}
	return wj, nil
}

// RTask implements work.Read.RTask.
func (s *storage) RTask(ctx context.Context, id string) (*work.Task, error) {
	s.tasksMu.RLock()
	defer s.tasksMu.RUnlock()

	ti := s.tasks.Get(item{key: id})
	if ti == nil {
		return nil, fmt.Errorf("Task %q could not be found", id)
	}

	return ti.(item).value.(task).to(nil), nil
}

// RSequence implements work.Read.RSequence.
func (s *storage) RSequence(ctx context.Context, id string) (*work.Sequence, error) {
	s.sequencesMu.RLock()
	defer s.sequencesMu.RUnlock()

	si := s.sequences.Get(item{key: id})
	if si == nil {
		return nil, fmt.Errorf("Sequence %q could not be found", id)
	}

	return si.(item).value.(sequence).to(nil), nil
}

// RJob implements work.Read.RJob.
func (s *storage) RJob(ctx context.Context, id string, full bool) (*work.Job, error) {
	s.jobsMu.RLock()
	defer s.jobsMu.RUnlock()

	ji := s.jobs.Get(item{key: id})
	if ji == nil {
		return nil, fmt.Errorf("Job %q could not be found", id)
	}

	return ji.(item).value.(job).to(), nil
}

// WLabor implements work.Write.WLabor.
func (s *storage) WLabor(ctx context.Context, l *work.Labor) error {
	s.laborsMu.Lock()
	defer s.laborsMu.Unlock()

	var curlrow labor
	tmp := s.labors.Get(item{key: l.ID})
	if tmp != nil {
		curlrow = tmp.(item).value.(labor)
	}

	lrow := labor{}
	lrow.from(l)
	s.labors.ReplaceOrInsert(item{key: l.ID, value: lrow})

	if !l.Submitted.IsZero() {
		s.bySubmit.ReplaceOrInsert(itemDate{key: l.Submitted, value: l.ID})
	}

	for _, tag := range l.Tags {
		if len(s.byTag[tag]) == 0 {
			s.byTag[tag] = make(byStarts, 0, 1)
		}
		s.byTag[tag] = append(s.byTag[tag], started{started: l.Started, id: l.ID})
		sort.Sort(s.byTag[tag])
	}

	if tmp != nil {
		if v := s.byState[curlrow.state]; v != nil {
			v.delete(l.ID)
			s.byState[curlrow.state] = v
		}
	}

	glog.Infof("I see state(%s): %s", l.ID, l.State().String())
	if v := s.byState[l.State()]; v == nil {
		s.byState[l.State()] = byStarts{
			started{started: l.Started, id: l.ID},
		}
		glog.Infof("here")
	} else {
		v = append(v, started{started: l.Started, id: l.ID})
		s.byState[l.State()] = v
		glog.Infof("nope here")
	}
	glog.Infof("byState: %#v", s.byState)

	for x, task := range l.Tasks {
		glog.Infof("task[%d]: %+v", x, task)
		err := s.WTask(ctx, task)
		if err != nil { // Because we failed, we need to remove all previous inserts.
			v := s.byState[l.State()]
			v.delete(l.ID)
			s.byState[l.State()] = v

			for _, tag := range l.Tags {
				v := s.byTag[tag]
				v.delete(l.ID)
				s.byTag[tag] = v
			}

			s.labors.Delete(item{key: l.ID})
		}
	}
	return nil
}

// WLaborState allows updating of the Labor's State and Reason.
func (s *storage) WLaborState(ctx context.Context, l *work.Labor) error {
	s.laborsMu.Lock()
	defer s.laborsMu.Unlock()

	v := s.labors.Get(item{key: l.ID})
	if v == nil {
		return fmt.Errorf("could not locate Labor: %q", l.ID)
	}

	lrow := v.(item).value.(labor)
	lrow.state = l.State()
	lrow.reason = l.Reason()

	s.labors.ReplaceOrInsert(item{key: l.ID, value: lrow})
	return nil
}

// WTask implements work.Write.WTask.
func (s *storage) WTask(ctx context.Context, t *work.Task) error {
	s.tasksMu.Lock()
	defer s.tasksMu.Unlock()

	trow := task{}
	trow.from(t)
	s.tasks.ReplaceOrInsert(item{key: t.ID, value: trow})

	for _, job := range t.PreChecks {
		if err := s.WJob(ctx, job); err != nil {
			s.tasks.Delete(item{key: t.ID})
		}
	}

	for _, job := range t.ContChecks {
		if err := s.WJob(ctx, job); err != nil {
			s.tasks.Delete(item{key: t.ID})
		}
	}

	for _, seq := range t.Sequences {
		if err := s.WSequence(ctx, seq); err != nil {
			s.tasks.Delete(item{key: t.ID})
		}
	}
	return nil
}

// WSequence implements work.Write.WSequence.
func (s *storage) WSequence(ctx context.Context, seq *work.Sequence) error {
	s.sequencesMu.Lock()
	defer s.sequencesMu.Unlock()

	srow := sequence{}
	srow.from(seq)

	s.sequences.ReplaceOrInsert(item{key: seq.ID, value: srow})

	for _, job := range seq.Jobs {
		if err := s.WJob(ctx, job); err != nil {
			s.sequences.Delete(item{key: seq.ID})
		}
	}
	return nil
}

// WJob implements work.Write.WJob.
func (s *storage) WJob(ctx context.Context, j *work.Job) error {
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()

	jrow := job{}

	resp := s.jobs.Get(item{key: j.ID})
	if resp != nil {
		jrow = resp.(item).value.(job)
		jrow.state = j.State()
		jrow.started = j.Started
		jrow.ended = j.Ended
		jrow.updateOutput([]byte(j.Output))
	} else {
		jrow.from(j)
	}

	s.jobs.ReplaceOrInsert(item{key: j.ID, value: jrow})
	return nil
}

// Labors implements work.Search.Labors.
func (s *storage) Labors(ctx context.Context, filter work.LaborFilter) (chan work.SearchResult, error) {
	ch := make(chan work.SearchResult, 10)

	go func() {
		defer close(ch)

		start := filter.SubmitBegin
		end := filter.SubmitEnd
		if end.IsZero() {
			end = time.Unix(1<<63-62135596801, 999999999)
		}

		filtered := false
		foundList := byStarts{}
		if len(filter.Tags) > 0 {
			filtered = true
			for _, tag := range filter.Tags {
				if len(s.byTag[tag]) > 0 {
					s.laborsMu.RLock()
					foundList = append(foundList, s.byTag[tag]...)
					s.laborsMu.RUnlock()
				}
			}
		}

		if filtered && len(foundList) == 0 {
			return
		}

		if len(filter.States) > 0 {
			tmp := byStarts{}
			s.laborsMu.RLock()
			for _, state := range filter.States {
				tmp = append(tmp, s.byState[state]...)
			}
			s.laborsMu.RUnlock()

			if filtered {
				foundList = tmp.intersection(foundList)
			} else {
				foundList = tmp
			}
			filtered = true
		}

		if filtered && len(foundList) == 0 {
			return
		}

		if filtered {
			sort.Sort(foundList)
			for _, entry := range foundList {
				if entry.started.Equal(start) || entry.started.After(start) {
					if entry.started.Before(end) {
						if err := s.filterByName(ctx, entry.id, filter, ch); err != nil {
							return
						}
					}
				}
			}
		} else {
			f := func(i llrb.Item) bool {
				id := i.(itemDate).value
				r := s.labors.Get(item{key: id})
				if r == nil {
					glog.Errorf("labor found in other trees not in .labor tree: %s", id)
					return false
				}

				lrow := r.(item).value.(labor)
				if err := s.filterByName(ctx, lrow.id, filter, ch); err != nil {
					return false
				}
				return true
			}
			s.laborsMu.RLock()
			s.bySubmit.AscendRange(itemDate{key: start}, itemDate{key: end}, f)
			s.laborsMu.RUnlock()
		}

	}()
	return ch, nil
}

func (s *storage) filterByName(ctx context.Context, id string, filter work.LaborFilter, ch chan work.SearchResult) error {
	s.laborsMu.RLock()
	lrow := s.labors.Get(item{key: id}).(item).value.(labor)
	s.laborsMu.RUnlock()

	if filter.NamePrefix != "" {
		if !strings.HasPrefix(lrow.name, filter.NamePrefix) {
			return nil
		}
	}
	if filter.NameSuffix != "" {
		if !strings.HasSuffix(lrow.name, filter.NameSuffix) {
			return nil
		}
	}

	var sr work.SearchResult
	native, err := s.reassemble(lrow)
	if err != nil {
		sr = work.SearchResult{Error: err}
	} else {
		sr = work.SearchResult{Labor: native}
	}

	select {
	case ch <- sr:
		if sr.Error != nil {
			return sr.Error
		}
	case <-ctx.Done():
		return fmt.Errorf("context expired")
	}
	return nil
}

func (s *storage) reassemble(l labor) (*work.Labor, error) {
	native := l.to()
	var err error
	native.Tasks, err = s.addTasks(l.tasks, native.Completed, false)
	if err != nil {
		return nil, err
	}
	return native, nil
}
