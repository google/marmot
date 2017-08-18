package etoe

import (
	"os"
	"path"
	"time"

	"github.com/google/marmot/instance"

	pb "github.com/google/marmot/testing/cogs/proto/tester"
)

var (
	testCog     = "localFile://" + path.Join(os.Getenv("GOPATH"), "src/github.com/google/marmot/testing/cogs/tester/tester")
	successArgs = &pb.Args{Success: true, Message: "Success"}
	failureArgs = &pb.Args{Failure: true, Message: "Failure"}
	waitArgs    = &pb.Args{Wait: 10, Message: "success wait"}
)

var server *instance.Instance

func init() {
	args := instance.Args{
		SIP:        "localhost",
		SPort:      0,
		MaxCrashes: 3,
		Insecure:   true,
		CertFile:   "",
		KeyFile:    "",
		Storage:    "inmemory",
	}
	var err error
	server, err = instance.New(args)
	if err != nil {
		panic(err)
	}
	go server.Run()
	time.Sleep(3 * time.Second)
}
