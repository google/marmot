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

// Package coordinator coordinates the running of Labors within the system.
package coordinator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/marmot/work"
	"github.com/kylelemons/godebug/pretty"
)

type process struct {
	added time.Time
	labor *work.Labor
	sync.Mutex
}

// Coordinator handles coordination of executing Labors.
type Coordinator struct {
	current    map[string]*process
	store      work.Storage
	mu         sync.RWMutex
	maxReloads int
	cogInfo    *work.CogInfo
}

// New is the constructor for Coordinator.
func New(store work.Storage, cogInfo *work.CogInfo, maxReloads int) (*Coordinator, error) {
	return &Coordinator{
		current:    make(map[string]*process, 10),
		store:      store,
		cogInfo:    cogInfo,
		maxReloads: maxReloads,
	}, nil
}

// Labor returns a running Labor.
func (c *Coordinator) Labor(id string) (*work.Labor, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p := c.current[id]
	if p == nil {
		return nil, fmt.Errorf("could not locate Labor with ID %q", id)
	}
	return p.labor, nil
}

// Add adds a Labor for processsing. Processing begins immediately if the state
// is set to NotStarted.  If set to AdminNotStarted, Start() must be called to
// begin execution.
func (c *Coordinator) Add(l *work.Labor) error {
	const writeTimeout = 30 * time.Second

	l.SetCogInfo(c.cogInfo)
	l.SetStorage(c.store)
	l.SetSignalDrain()

	if err := l.Validate(); err != nil {
		return err
	}
	l.SetIDs()

	glog.Infof("before add:\n%s", pretty.Sprint(l))
	ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()
	l.Submitted = time.Now()
	if err := c.store.WLabor(ctx, l); err != nil {
		return fmt.Errorf("problem writing Labor to storage: %s", err)
	}

	c.mu.Lock()
	c.current[l.Meta.ID] = &process{added: time.Now(), labor: l}
	c.mu.Unlock()

	if l.State() == work.NotStarted {
		if err := l.Start(); err != nil {
			return err
		}
		go func() {
			<-l.Completed
			c.mu.Lock()
			defer c.mu.Unlock()
			delete(c.current, l.ID)
		}()
	}

	return nil
}

// Start begins processing of a Labor in the AdminNotStarted state.
func (c *Coordinator) Start(id string) error {
	c.mu.Lock()
	p := c.current[id]
	c.mu.Unlock()

	if p == nil {
		return fmt.Errorf("Labor(%s): could not be found among running Labors", id)
	}

	p.Lock()
	defer p.Unlock()

	if p.labor.State() != work.AdminNotStarted {
		return fmt.Errorf("Labor(%s): can only start Labors in state 'AdminNotStarted'", id)
	}

	go func() {
		<-p.labor.Completed
		c.mu.Lock()
		defer c.mu.Unlock()
		delete(c.current, id)
	}()

	return p.labor.Start()
}

// Signal signals a Labor (Pause/Stop/Unpause/...).
func (c *Coordinator) Signal(id string, signal work.Signal) error {
	c.mu.Lock()
	p := c.current[id]
	c.mu.Unlock()

	if p == nil {
		return fmt.Errorf("could not locate Labor with ID %q", id)
	}

	p.Lock()
	defer p.Unlock()

	if err := p.labor.Signal(signal); err != nil {
		return err
	}
	return nil
}
