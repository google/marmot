package etoe

/*
import (
	"context"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/google/marmot/client"
	cpb "github.com/google/marmot/proto/marmot"
	pb "github.com/google/marmot/testing/cogs/proto/tester"
	"github.com/kylelemons/godebug/pretty"
)

type contPlan struct {
  concurrency int32
}

func (s stopPlan) toBuild() client.Builder {
	if s.concurrency < 1 {
		s.concurrency = 1
	}

	build, err := client.NewBuilder(
		"test",
		"labor desc",
		server.Addr(),
		client.BuilderControlOption(client.Insecure()),
	)
	if err != nil {
		panic(err)
	}

	build.AddTask("task", "task desc", client.Concurrency(s.concurrency))
	for _, seq := range s.sequences {
		build.AddSequence("target", "seq desc")
		for i := 0; i < seq.beforeActions; i++ {
			build.AddJob(testCog, "job desc", successArgs)
		}
		if seq.waitTime == 0 {
			seq.waitTime = 1
		}
		build.AddJob(testCog, "job desc", &pb.Args{Wait: seq.waitTime, Message: "success wait"})
		for i := 0; i < seq.afterActions; i++ {
			build.AddJob(testCog, "job desc", successArgs)
		}
	}

	return build
}
*/
