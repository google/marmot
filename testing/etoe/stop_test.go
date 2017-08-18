package etoe

import (
	"context"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/google/marmot/client"
	pb "github.com/google/marmot/testing/cogs/proto/tester"
	"github.com/kylelemons/godebug/pretty"
)

type stopSeq struct {
	beforeActions int
	afterActions  int
	waitTime      int32
}

type stopPlan struct {
	concurrency int
	sequences   []stopSeq
	hitStopAt   time.Duration
}

func (s stopPlan) toBuild() client.Builder {
	if s.concurrency < 1 {
		s.concurrency = 1
	}

	build, err := client.NewBuilder(
		"test",
		"labor desc",
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

func TestStop(t *testing.T) {
	t.Parallel()

	tests := []struct {
		desc   string
		plan   stopPlan
		result client.State
	}{
		{
			desc: "Hit stop after Labor is finished processing",
			plan: stopPlan{
				sequences: []stopSeq{
					{},
				},
				hitStopAt: 20 * time.Second,
			},
			result: client.Completed,
		},
		{
			desc: "Stop while single sequence is processing",
			plan: stopPlan{
				sequences: []stopSeq{
					{
						beforeActions: 2,
						afterActions:  2,
						waitTime:      10,
					},
				},
				hitStopAt: 5 * time.Second,
			},
			result: client.Stopped,
		},
		{
			desc: "Stop while multiple sequence are processing",
			plan: stopPlan{
				sequences: []stopSeq{
					{
						beforeActions: 100,
						afterActions:  100,
						waitTime:      10,
					},
				},
				concurrency: 10,
				hitStopAt:   5 * time.Second,
			},
			result: client.Stopped,
		},
		{
			desc: "Stop while the last sequence/action in a Task runs",
			plan: stopPlan{
				sequences: []stopSeq{
					{
						beforeActions: 0,
						afterActions:  0,
						waitTime:      20,
					},
				},
				hitStopAt: 5 * time.Second,
			},
			result: client.Completed,
		},
	}

	ctrl, err := client.NewControl(server.Addr(), client.Insecure())
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range tests {
		build := test.plan.toBuild()

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		id, err := ctrl.Submit(ctx, build.Labor(), true)
		if err != nil {
			t.Fatal(err)
		}

		go func() {
			time.Sleep(test.plan.hitStopAt)
			glog.Infof("hitting stop")
			ctrl.Stop(ctx, id)
		}()

		if err := ctrl.Wait(ctx, id); err != nil{
			t.Fatal(err)
		}

		l, err := ctrl.FetchLabor(ctx, id, true)
		if err != nil {
			t.Fatal(err)
		}

		if l.State != test.result {
			t.Errorf("Test %s: Labor State: got %q, want %q", test.desc, l.State, test.result)
			glog.Infof("%s", pretty.Sprint(l))
		}

		for _, task := range l.Tasks {
			switch task.State {
			case client.NotStarted, client.Running, client.Completed, client.Stopped:
				// Do nothing
			default:
				t.Errorf("task should not be in state: %v", task.State)
			}
			for _, seq := range task.Sequences {
				switch seq.State {
				case client.NotStarted, client.Running, client.Completed, client.Stopped:
					// Do nothing
				default:
					t.Errorf("sequence should not be in state: %v", seq.State)
				}
				for _, job := range seq.Jobs {
					switch job.State {
					case client.NotStarted, client.Running, client.Completed, client.Stopped:
						// Do nothing
					default:
						t.Errorf("job should not be in state: %v", job.State)
					}
				}
			}
		}
	}
}
