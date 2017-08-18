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

// Package client provides a client for interacting with the Marmot Workflow Service.
// Note: interfaces here such as Builder will change.  If using to fake tests,
// you should embed the interface in your testing interface or face possible
// breakage in the future.
package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/glog"
	"github.com/golang/protobuf/jsonpb"
	pb "github.com/google/marmot/proto/marmot"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	oldContext "golang.org/x/net/context"
)

var jsonMarshaller = jsonpb.Marshaler{Indent: "\t"}

// Builder provides methods for building a Labor in a way that programtically
// friendly as an alternative to building one directly using the protocol buffer.
type Builder interface {
	// AddTask adds a Task to the Labor.
	AddTask(name, desc string, opts ...TaskOption) error

	// AddPreCheck adds a Precheck to a Task. Will be added to the last
	// Task created by AddTask().
	AddPreCheck(cogPath, desc string, args proto.Message) error

	// AddContCheck adds a ContCheck to a Task. Will be added to the last
	// Task created by AddTask().
	AddContCheck(cogPath, desc string, args proto.Message) error

	// AddSequence adds a Sequence to the last Task created with AddTask().
	AddSequence(target, desc string) error

	// AddJob adds a Job to the last Sequence created with AddSequence()/
	AddJob(cogPath, desc string, args proto.Message) error

	// Labor returns a COPY of the Labor object.
	Labor() *Labor
}

// TaskOption sets an option for a task.
type TaskOption func(t *Task)

// ToleratedFailures sets the number of failures allowed before a Task
// stops execution.  Defaults to 0.
func ToleratedFailures(n int) TaskOption {
	return func(t *Task) {
		if n < 0 {
			n = 0
		}
		t.ToleratedFailures = n
	}
}

// Concurrency sets the number of Jobs that can execute at one time within
// the Task.  Defaults to 1.
func Concurrency(n int) TaskOption {
	return func(t *Task) {
		if n < 1 {
			n = 1
		}
		t.Concurrency = n
	}
}

// ContCheckInterval sets how long to wait before doing another continuous
// check.  This defaults to 0 seconds.
func ContCheckInterval(d time.Duration) TaskOption {
	return func(t *Task) {
		if d < 0 {
			d = 0
		}
		t.ContCheckInterval = d
	}
}

// PassFailures sets the next Tasks failures equal to the ending failures for
// this Task instead of 0.  This defaults to false.
func PassFailures(b bool) TaskOption {
	return func(t *Task) {
		t.PassFailures = b
	}
}

// BuilderOption is options to the NewBuilder() constructor.
type BuilderOption func(b *builder) error

// Tags adds tags t to the underlying Labor.
func Tags(t []string) BuilderOption {
	return func(b *builder) error {
		b.labor.Tags = t
		return nil
	}
}

// builder implements Builder.
type builder struct {
	labor *Labor
	sync.Mutex
}

// NewBuilder is the constructor for Builder.
func NewBuilder(name, desc string, opts ...BuilderOption) (Builder, error) {
	b := &builder{
		labor: &Labor{
			Meta: Meta{
				Name: name,
				Desc: desc,
			},
			// TODO(johnsiilver): Move to Submit
			// ClientId: uuid.New(),
		},
	}

	for _, opt := range opts {
		opt(b)
	}

	return b, nil
}

// AddTask implements Builder.AddTask().
func (b *builder) AddTask(name, desc string, opts ...TaskOption) error {
	b.Lock()
	defer b.Unlock()

	if name == "" {
		return fmt.Errorf("name must not be an empty string")
	}

	t := &Task{
		Meta: Meta{
			Name: name,
			Desc: desc,
		},
		Concurrency:       0,
		ContCheckInterval: 0,
	}

	for _, opt := range opts {
		opt(t)
	}
	b.labor.Tasks = append(b.labor.Tasks, t)
	return nil
}

func (b *builder) addJob(cogPath, desc string, args proto.Message, sl *[]*Job) error {
	b.Lock()
	defer b.Unlock()

	if cogPath == "" {
		return fmt.Errorf("cogPath must not be an empty string")
	}

	if args == nil {
		return fmt.Errorf("args must not be nil")
	}

	buff := new(bytes.Buffer)
	if err := jsonMarshaller.Marshal(buff, args); err != nil {
		return fmt.Errorf("args could not be marshalled into JSON")
	}

	j := &Job{
		Meta: Meta{
			Desc: desc,
		},
		CogPath: cogPath,
		Args:    buff.String(),
	}
	*sl = append(*sl, j)
	return nil
}

// AddPreCheck implements Builder.AddPreCheck().
func (b *builder) AddPreCheck(cogPath, desc string, args proto.Message) error {
	if len(b.labor.Tasks) == 0 {
		return fmt.Errorf("no Tasks have been added")
	}

	t := b.labor.Tasks[len(b.labor.Tasks)-1]
	return b.addJob(cogPath, desc, args, &t.PreChecks)
}

// AddContCheck implements Builder.AddContCheck().
func (b *builder) AddContCheck(cogPath, desc string, args proto.Message) error {
	if len(b.labor.Tasks) == 0 {
		return fmt.Errorf("no Tasks have been added")
	}

	t := b.labor.Tasks[len(b.labor.Tasks)-1]
	return b.addJob(cogPath, desc, args, &t.ContChecks)
}

// AddSequence implements Builder.AddSequence().
func (b *builder) AddSequence(target, desc string) error {
	b.Lock()
	defer b.Unlock()

	if len(b.labor.Tasks) == 0 {
		return fmt.Errorf("no Tasks have been added")
	}
	t := b.labor.Tasks[len(b.labor.Tasks)-1]

	s := &Sequence{
		Meta: Meta{
			Desc: desc,
		},
		Target: target,
	}
	t.Sequences = append(t.Sequences, s)
	return nil
}

// AddJob implements Builder.AddJob().
func (b *builder) AddJob(cogPath, desc string, args proto.Message) error {
	if len(b.labor.Tasks) == 0 {
		return fmt.Errorf("no Tasks have been added")
	}

	t := b.labor.Tasks[len(b.labor.Tasks)-1]

	if len(t.Sequences) == 0 {
		return fmt.Errorf("no Sequence has been added to the latest Task")
	}

	s := t.Sequences[len(t.Sequences)-1]
	return b.addJob(cogPath, desc, args, &s.Jobs)
}

// Labor implements Builder.Labor().
func (b *builder) Labor() *Labor {
	if b.labor == nil {
		return nil
	}
	p := b.labor.toProto()
	l := &Labor{}
	l.fromProto(proto.Clone(p).(*pb.Labor))
	return l
}

// LaborStream is used to return the results of a stream of Labor objects.
type LaborStream struct {
	Labor *Labor
	Error error
}

// SearchFilter is used to restrict the Labors being found during a search.
type SearchFilter struct {
	// States returns objects that are currently at one the "states".
	// If empty, all states are included.
	States []pb.States

	// NamePrefix/ NameSuffix locates Labors starting with the string.
	// An empty string has no filter.
	NamePrefix, NameSuffix string

	// Tags matches any Labor that has any tag listed.  An empty list has no filter.
	Tags []string

	// SubmitBegin locates any Labor that was submitted at or after this time.
	SubmitBegin time.Time

	// SubmitEnd locates any Labor that was submitted before this time.
	SubmitEnd time.Time
}

func (s SearchFilter) toProto() *pb.LaborFilter {
	return &pb.LaborFilter{
		States:      s.States,
		NamePrefix:  s.NamePrefix,
		NameSuffix:  s.NameSuffix,
		Tags:        s.Tags,
		SubmitBegin: s.SubmitBegin.Unix(),
		SubmitEnd:   s.SubmitEnd.Unix(),
	}
}

// Control provides methods for interacting with the Marmot service.
type Control interface {
	// Submit submits a Labor to the Marmot service. If start is set, will start
	// execution immediately.  Otherwise Start() must be called. Returns the
	// ID of the Labor.
	Submit(ctx context.Context, l *Labor, start bool) (string, error)

	// Start starts execution of Labor with ID "id".
	Start(ctx context.Context, id string) error

	// Pause pauses execution of Labor with ID "id".
	Pause(ctx context.Context, id string) error

	// Resume unpauses a Labor the is in the pause state at ID "id".
	Resume(ctx context.Context, id string) error

	// Stop stops an executing Labor with the ID "id".
	Stop(ctx context.Context, id string) error

	// Wait waits for a Labor with id to reach a completion state.
	Wait(ctx context.Context, id string) error

	// Monitor returns Labor objects representing a single Labor as it is executed
	// by the service, stopping when the Labor reaches its final execution.
	// These Labors will only have state data and no args/output data.
	Monitor(ctx context.Context, id string) (chan LaborStream, error)

	// SearchLabors returns a stream of Labors matching the filter.
	SearchLabors(ctx context.Context, filter SearchFilter) (chan LaborStream, error)

	// FetchLabor returns with ID 'id'.  If 'full' is set, the input/output data
	// will be returned.
	FetchLabor(ctx context.Context, id string, full bool) (*Labor, error)

	// Close kills the connection to the service.
	Close()
}

type loginCreds struct {
	Username, Password string
}

func (c *loginCreds) GetRequestMetadata(oldContext.Context, ...string) (map[string]string, error) {
	return map[string]string{
		"username": c.Username,
		"password": c.Password,
	}, nil
}

func (c *loginCreds) RequireTransportSecurity() bool {
	return true
}

// ControlOption is an optional argument to NewControl().
type ControlOption func(c *control) error

// PasswordAuth provides a username/password for authenticating to Marmot.
func PasswordAuth(user, password string) ControlOption {
	return func(c *control) error {
		cred := grpc.WithPerRPCCredentials(&loginCreds{
			Username: user,
			Password: password,
		})
		c.dialOptions = append(c.dialOptions, cred)
		return nil
	}
}

// CertAuth uses the machine's installed certs to attempt authentication.
// Unless running a test, serverHostOverride should be set to empty string.
func CertAuth(serverHostOverride string) ControlOption {
	return func(c *control) error {
		c.dialOptions = append(c.dialOptions, grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, serverHostOverride)))
		return nil
	}
}

// CertFileAuth uses paths to a cert.pem and key.pem to authenticate to the service.
func CertFileAuth(cert, key string) ControlOption {
	return func(c *control) error {
		tc, err := credentials.NewServerTLSFromFile(cert, key)
		if err != nil {
			return err
		}
		c.dialOptions = append(c.dialOptions, grpc.WithTransportCredentials(tc))
		return nil
	}
}

// Insecure indicates to use no transport encryption.  This should only be used in testing.
func Insecure() ControlOption {
	return func(c *control) error {
		c.dialOptions = append(c.dialOptions, grpc.WithInsecure())
		return nil
	}
}

type control struct {
	conn        *grpc.ClientConn
	client      pb.MarmotServiceClient
	dialOptions []grpc.DialOption
}

// NewControl is the constructor for Control.
func NewControl(endpoint string, opts ...ControlOption) (Control, error) {
	c := &control{
		dialOptions: []grpc.DialOption{
			grpc.WithCompressor(grpc.NewGZIPCompressor()),
			grpc.WithDecompressor(grpc.NewGZIPDecompressor()),
			grpc.WithBlock(),
			grpc.WithTimeout(10 * time.Second),
		},
	}
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}

	var err error
	c.conn, err = grpc.Dial(endpoint, c.dialOptions...)
	if err != nil {
		return nil, err
	}

	c.client = pb.NewMarmotServiceClient(c.conn)
	return c, nil
}

// Submit implements Control.Submit().
func (c *control) Submit(ctx context.Context, l *Labor, start bool) (string, error) {
	resp, err := c.client.Submit(
		ctx,
		&pb.SubmitReq{
			Labor:         l.toProto(),
			StartOnSubmit: start,
		},
	)

	if err != nil {
		return "", fmt.Errorf("problem submitting labor: %s", err)
	}

	return resp.Id, nil
}

// Wait waits for a Labor to reach a completion state.
func (c *control) Wait(ctx context.Context, id string) error {
	ch, err := c.Monitor(ctx, id)
	if err != nil {
		return err
	}

	var resp LaborStream
	for resp = range ch {
		if resp.Error != nil {
			return resp.Error
		}
	}

	return nil
}

// Submit implements Control.Submit().
func (c *control) Start(ctx context.Context, id string) error {
	_, err := c.client.Start(ctx, &pb.StartReq{Id: id})
	return err
}

// Pause implements Control.Pause().
func (c *control) Pause(ctx context.Context, id string) error {
	_, err := c.client.Pause(ctx, &pb.PauseReq{Id: id})
	return err
}

// Resume implements Control.Resume().
func (c *control) Resume(ctx context.Context, id string) error {
	_, err := c.client.Resume(ctx, &pb.ResumeReq{Id: id})
	return err
}

// Stop implements Control.Stop().
func (c *control) Stop(ctx context.Context, id string) error {
	_, err := c.client.Stop(ctx, &pb.StopReq{Id: id})
	return err
}

// Monitor implements Control.Monitor().
func (c *control) Monitor(ctx context.Context, id string) (chan LaborStream, error) {
	sc, err := c.client.Monitor(ctx, &pb.MonitorReq{Id: id})
	if err != nil {
		return nil, err
	}
	ch := make(chan LaborStream, 10)
	go func() {
		for {
			resp, err := sc.Recv()
			if err == io.EOF {
				defer close(ch)
				return
			}
			if err != nil {
				ch <- LaborStream{Error: err}
				return
			}
			l := &Labor{}
			l.fromProto(resp.Labor)
			ch <- LaborStream{Labor: l}
		}
	}()
	return ch, nil
}

// SearchLabors implements Control.SearchLabors().
func (c *control) SearchLabors(ctx context.Context, filter SearchFilter) (chan LaborStream, error) {
	sc, err := c.client.SearchLabor(ctx, &pb.LaborSearchReq{Filter: filter.toProto()})
	if err != nil {
		return nil, err
	}
	ch := make(chan LaborStream, 10)
	go func() {
		for {
			labor, err := sc.Recv()
			if err == io.EOF {
				close(ch)
				return
			}
			if err != nil {
				ch <- LaborStream{Error: err}
				return
			}
			l := &Labor{}
			l.fromProto(labor)
			ch <- LaborStream{Labor: l}
		}
	}()
	return ch, nil
}

// FetchLabor implements Control.FetchLabor().
func (c *control) FetchLabor(ctx context.Context, id string, full bool) (*Labor, error) {
	lp, err := c.client.FetchLabor(ctx, &pb.FetchLaborReq{Id: id, Full: full})
	if err != nil {
		return nil, err
	}
	l := &Labor{}
	glog.Infof(proto.MarshalTextString(lp))
	l.fromProto(lp)
	return l, nil
}

// Close implements Control.Close().
func (c *control) Close() {
	c.conn.Close()
}
