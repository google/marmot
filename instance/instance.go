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

// Package instance holds functions for creating a Marmot instance and
// methods for running the instance.
package instance

import (
	"fmt"
	"net"

	log "github.com/golang/glog"
	pb "github.com/google/marmot/proto/marmot"
	"github.com/google/marmot/service"
	"github.com/google/marmot/work"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	// Loads the inmemory storage registration.  As instance supplants much of
	// main, the import goes here.
	_ "github.com/google/marmot/service/storage/inmemory"
)

// Args are arguments to New().
type Args struct {
	// SIP is the IP address to use for Stubby.  By default, it listens on all
	// IP addresses.  Use "localhost" if you just want just 127.0.0.1.
	SIP string
	// SPort is the port to use for the Stubby service.
	SPort int32
	// MaxCrashses is the maximum number of plugin crashes to allow.
	MaxCrashes int
	// Insecure indicates that you are not going to be using transport encryption.
	// THIS IS A BAD IDEA EXCEPT FOR TESTS!!!!!!
	Insecure bool
	// CertFile and KeyFile are paths to a certificate or key file.
	CertFile, KeyFile string
	// Storage is the type of backend storage to use.
	Storage string
}

// Instance holds an instance of a Marmot server.
type Instance struct {
	args   Args
	server *grpc.Server
	addr   string
	store  work.Storage
}

// New is the constructor for Instance.
func New(args Args) (*Instance, error) {
	store, err := retrieveStorage(args.Storage)
	if err != nil {
		return nil, err
	}

	return &Instance{
		args:  args,
		store: store,
	}, nil
}

// Addr returns the address the server is running on.
func (i *Instance) Addr() string {
	return i.addr
}

// Run attempts to run a Marmot instance.
func (i *Instance) Run() {
	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%d", i.args.SIP, i.args.SPort))
	if err != nil {
		panic(fmt.Sprintf("failed to listen: %v", err))
	}

	log.Infof("Listening on %s", lis.Addr().String())
	i.addr = lis.Addr().String()

	srv, err := service.New(i.store, i.args.MaxCrashes)
	if err != nil {
		panic(err)
	}

	opts := []grpc.ServerOption{
		grpc.RPCCompressor(grpc.NewGZIPCompressor()),
		grpc.RPCDecompressor(grpc.NewGZIPDecompressor()),
		grpc.MaxMsgSize(1024 * 1024 * 1024 * 2),
	}

	if !i.args.Insecure {
		tc, err := credentials.NewServerTLSFromFile(i.args.CertFile, i.args.KeyFile)
		if err != nil {
			panic(err)
		}
		opts = append(opts, grpc.Creds(tc))
	}

	i.server = grpc.NewServer(opts...)
	pb.RegisterMarmotServiceServer(i.server, srv)

	i.server.Serve(lis)
}

// Stop stops the GRPC service.  This should only be used in tests as there
// is no guarentees that all child goroutines will stop.
func (i *Instance) Stop() {
	i.server.GracefulStop()
}

// Storage returns the storage object being used by the server.
func (i *Instance) Storage() work.Storage {
	return i.store
}

func retrieveStorage(store string) (work.Storage, error) {
	var args interface{}
	switch store {
	case "inmemory":
		args = nil
	default:
		return nil, fmt.Errorf("-storage=%q is invalid", store)
	}

	storeFunc, ok := work.Registry[store]
	if !ok {
		return nil, fmt.Errorf("cannot locate storage type %q", store)
	}
	return storeFunc(args), nil
}
