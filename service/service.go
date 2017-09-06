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

// Package service implements the GRPC service for Marmot.
package service

import (
	"fmt"
	"time"

	log "github.com/golang/glog"
	cogStorage "github.com/google/marmot/service/cogs/storage"
	"github.com/google/marmot/service/convert"
	"github.com/google/marmot/service/coordinator"
	"github.com/google/marmot/work"
	"golang.org/x/net/context"

	pb "github.com/google/marmot/proto/marmot"
)

// Marmot implements pb.MarmotServiceServer.
type Marmot struct {
	coord   *coordinator.Coordinator
	store   work.Storage
	cogKV   cogStorage.Reader
	cogInfo *work.CogInfo
}

// New is the constructor for Marmot.
func New(store work.Storage, cogKV cogStorage.Reader, maxCrashes int) (*Marmot, error) {
	cogInfo, err := work.NewCogInfo(maxCrashes)
	if err != nil {
		return nil, err
	}

	coord, err := coordinator.New(store, cogInfo, maxCrashes)
	if err != nil {
		return nil, err
	}

	return &Marmot{
		coord:   coord,
		store:   store,
		cogKV:   cogKV,
		cogInfo: cogInfo,
	}, nil
}

// Submit implements pb.MarmotServiceServer.Submit.
func (m *Marmot) Submit(ctx context.Context, req *pb.SubmitReq) (*pb.SubmitResp, error) {
	if req.Labor == nil {
		return nil, fmt.Errorf("must submit a Labor")
	}
	l, err := convert.ToNative(req.Labor, m.store)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	if req.StartOnSubmit {
		l.SetState(work.NotStarted)
	} else {
		l.SetState(work.AdminNotStarted)
	}

	if err := m.coord.Add(l); err != nil {
		log.Error(err)
		return nil, err
	}
	return &pb.SubmitResp{Id: l.ID}, nil
}

// Monitor implements pb.MarmotServiceServer.Monitor.
func (m *Marmot) Monitor(req *pb.MonitorReq, stream pb.MarmotService_MonitorServer) error {
	var lastState work.State
	for {
		l, err := m.store.RLabor(stream.Context(), req.Id, false)
		if err != nil {
			log.Error(err)
			return err
		}

		log.Infof("state is: %v", l.State())
		switch l.State() {
		case work.Completed, work.Failed, work.Stopped:
			stream.Send(&pb.MonitorResp{Labor: convert.FromNative(l, false)})
			return nil
		}

		if l.State() != lastState {
			stream.Send(&pb.MonitorResp{Labor: convert.FromNative(l, false)})
			lastState = l.State()
		}
		time.Sleep(2 * time.Second)
	}
}

// Start implements pb.MarmotServiceServer.Start.
func (m *Marmot) Start(ctx context.Context, req *pb.StartReq) (*pb.StartResp, error) {
	if err := m.coord.Start(req.Id); err != nil {
		return nil, err
	}
	return &pb.StartResp{}, nil
}

// Pause implements pb.MarmotServiceServer.Pause.
func (m *Marmot) Pause(ctx context.Context, req *pb.PauseReq) (*pb.PauseResp, error) {
	if err := m.coord.Signal(req.Id, work.Pause); err != nil {
		return nil, err
	}
	return &pb.PauseResp{}, nil
}

// Resume implements pb.MarmotServiceServer.Resume.
func (m *Marmot) Resume(ctx context.Context, req *pb.ResumeReq) (*pb.ResumeResp, error) {
	if err := m.coord.Signal(req.Id, work.Resume); err != nil {
		return nil, err
	}
	return &pb.ResumeResp{}, nil
}

// Stop implements pb.MarmotServiceServer.Stop.
func (m *Marmot) Stop(ctx context.Context, req *pb.StopReq) (*pb.StopResp, error) {
	if err := m.coord.Signal(req.Id, work.Stop); err != nil {
		return nil, err
	}
	return &pb.StopResp{}, nil
}

// FetchLabor implements pb.MarmotServiceServer.FetchLabor.
func (m *Marmot) FetchLabor(ctx context.Context, req *pb.FetchLaborReq) (*pb.Labor, error) {
	n, err := m.store.RLabor(ctx, req.Id, req.Full)
	if err != nil {
		return nil, err
	}

	return convert.FromNative(n, req.Full), nil
}

// SearchLabor implements pb.MarmotServiceServer.SearchLabor.
func (m *Marmot) SearchLabor(req *pb.LaborSearchReq, stream pb.MarmotService_SearchLaborServer) error {
	states := []work.State{}
	for _, s := range req.Filter.States {
		states = append(states, work.State(s))
	}

	filter := work.LaborFilter{
		BaseFilter: work.BaseFilter{
			States: states,
		},
		NamePrefix:  req.Filter.NamePrefix,
		NameSuffix:  req.Filter.NameSuffix,
		Tags:        req.Filter.Tags,
		SubmitBegin: time.Unix(req.Filter.SubmitBegin, 0),
		SubmitEnd:   time.Unix(req.Filter.SubmitEnd, 0),
	}

	ch, err := m.store.Labors(stream.Context(), filter)
	if err != nil {
		log.Error(err)
		return err
	}

	for result := range ch {
		if result.Error != nil {
			log.Error(err)
			return err
		}
		log.Infof("labor before conversion:\n%s", result.Labor)
		log.Infof("labor after conversion:\n%s", convert.FromNative(result.Labor, false))
		stream.Send(convert.FromNative(result.Labor, false))
	}
	return nil
}
