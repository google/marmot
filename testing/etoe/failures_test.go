package etoe

// This file tests that various failure scenarios work.

import (
	"context"
	"testing"
	"time"

	"github.com/google/marmot/client"
)

func TestSingleTaskTestFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		desc      string
		successes int
		failures  int
		tolerated int
		fail      bool
	}{
		{
			desc:      "Tolerate 3 failures, get 4 failures",
			successes: 3,
			failures:  4,
			tolerated: 3,
			fail:      true,
		},
		{
			desc:      "Tolerate 0 failures, get 1 failure",
			successes: 2,
			failures:  1,
			tolerated: 0,
			fail:      true,
		},
		{
			desc:      "Tolerate 3 failures, get 2 failures",
			successes: 0,
			failures:  2,
			tolerated: 3,
			fail:      false,
		},
	}

	for _, test := range tests {
		build, err := client.NewBuilder(
			"test",
			"labor desc",
		)
		if err != nil {
			t.Fatal(err)
		}
		ctrl, err := client.NewControl(server.Addr(), client.Insecure())
		if err != nil {
			t.Fatal(err)
		}

		build.AddTask("task", "task desc", client.ToleratedFailures(test.tolerated), client.Concurrency(test.successes+test.failures))
		for i := 0; i < test.successes; i++ {
			build.AddSequence("target", "seq desc")
			build.AddJob(testCog, "job desc", successArgs)
		}
		for i := 0; i < test.failures; i++ {
			build.AddSequence("target", "seq desc")
			build.AddJob(testCog, "job desc", successArgs)
			build.AddJob(testCog, "job desc", failureArgs)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		id, err := ctrl.Submit(ctx, build.Labor(), true)
		if err != nil {
			t.Fatal(err)
		}

		if err := ctrl.Wait(ctx, id); err != nil {
			t.Fatal(err)
		}

		l, err := ctrl.FetchLabor(ctx, id, true)
		if err != nil {
			t.Fatal(err)
		}

		for i := 0; i < test.successes; i++ {
			if l.Tasks[0].Sequences[i].Jobs[0].State != client.Completed {
				t.Fatalf("Test %s: seq[%d]job[%d] status: got %s, want %s", test.desc, i, 0, l.Tasks[0].Sequences[i].Jobs[0].State, client.Completed)
			}
			if l.Tasks[0].Sequences[i].State != client.Completed {
				t.Fatalf("Test %s: seq[%d] state: got %s, want %s", test.desc, i, l.Tasks[0].Sequences[i].State, client.Completed)
			}
		}
		for i := test.successes; i < test.successes+test.failures; i++ {
			if l.Tasks[0].Sequences[i].Jobs[0].State != client.Completed {
				t.Fatalf("Test %s: seq[%d]job[%d] status: got %s, want %s", test.desc, i, 0, l.Tasks[0].Sequences[i].Jobs[0].State, client.Completed)
			}
			if l.Tasks[0].Sequences[i].Jobs[1].State != client.Failed {
				t.Fatalf("Test %s: seq[%d]job[%d] status: got %s, want %s", test.desc, i, 1, l.Tasks[0].Sequences[i].Jobs[0].State, client.Completed)
			}
			if l.Tasks[0].Sequences[i].State != client.Failed {
				t.Fatalf("Test %s: seq[%d] state: got %s, want %s", test.desc, i, l.Tasks[0].Sequences[i].State, client.Failed)
			}
		}
		if test.fail {
			if l.State != client.Failed {
				t.Fatalf("Test %s: labor status: got state %q, want state %q", test.desc, l.State, client.Failed)
			}
			if l.Reason != client.MaxFailures {
				t.Fatalf("Test %s: labor status: got state %q, want state %q", test.desc, l.Reason, client.MaxFailures)
			}
			if l.Tasks[0].State != client.Failed {
				t.Fatalf("Test %s: task status: got state %q, want state %q", test.desc, l.Tasks[0].State, client.Failed)
			}
		} else {
			if l.State != client.Completed {
				t.Fatalf("Test %s: labor status: got state %q, want state %q", test.desc, l.State, client.Completed)
			}
			if l.Reason != client.NoFailure {
				t.Fatalf("Test %s: labor status: got state %q, want state %q", test.desc, l.Reason, client.NoFailure)
			}
			if l.Tasks[0].State != client.Completed {
				t.Fatalf("Test %s: task status: got state %q, want state %q", test.desc, l.Tasks[0].State, client.Completed)
			}
		}
	}
}

type taskPlan struct {
	success      int
	failures     int
	tolerated    int
	passFailures bool
	state        client.State
}

func TestMultipleTaskTestFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		desc  string
		tasks []taskPlan
		fail  bool
	}{
		{
			desc: "First task fails",
			tasks: []taskPlan{
				{
					success:   3,
					failures:  1,
					tolerated: 0,
					state:     client.Failed,
				},
				{
					success: 1,
					state:   client.NotStarted,
				},
			},
			fail: true,
		},
		{
			desc: "Second task fails",
			tasks: []taskPlan{
				{
					success:   3,
					failures:  1,
					tolerated: 1,
					state:     client.Completed,
				},
				{
					success:   1,
					failures:  1,
					tolerated: 0,
					state:     client.Failed,
				},
			},
			fail: true,
		},
		{
			desc: "Second task starts and immediately fails because of passFailures==true",
			tasks: []taskPlan{
				{
					success:      3,
					failures:     1,
					tolerated:    1,
					passFailures: true,
					state:        client.Completed,
				},
				{
					success:   1,
					failures:  0,
					tolerated: 0,
					state:     client.Failed,
				},
			},
			fail: true,
		},
		{
			desc: "Second task starts and fails because of passFailures==true",
			tasks: []taskPlan{
				{
					success:      3,
					failures:     1,
					tolerated:    1,
					passFailures: true,
					state:        client.Completed,
				},
				{
					success:   1,
					failures:  1,
					tolerated: 1,
					state:     client.Failed,
				},
			},
			fail: true,
		},
		{
			desc: "Second task starts and succeeds with passFailures==true",
			tasks: []taskPlan{
				{
					success:      3,
					failures:     1,
					tolerated:    1,
					passFailures: true,
					state:        client.Completed,
				},
				{
					success:   1,
					failures:  1,
					tolerated: 2,
					state:     client.Completed,
				},
			},
			fail: false,
		},
	}

	for _, test := range tests {
		build, err := client.NewBuilder(
			"test",
			"labor desc",
		)
		if err != nil {
			t.Fatal(err)
		}

		ctrl, err := client.NewControl(server.Addr(), client.Insecure())
		if err != nil {
			t.Fatal(err)
		}

		for _, plan := range test.tasks {
			opts := []client.TaskOption{
				client.ToleratedFailures(plan.tolerated),
				client.Concurrency(99),
			}
			if plan.passFailures {
				opts = append(opts, client.PassFailures(true))
			}

			build.AddTask("task", "task desc", opts...)
			for i := 0; i < plan.success; i++ {
				build.AddSequence("target", "seq desc")
				build.AddJob(testCog, "job desc", successArgs)
			}
			for i := 0; i < plan.failures; i++ {
				build.AddSequence("target", "seq desc")
				build.AddJob(testCog, "job desc", failureArgs)
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		id, err := ctrl.Submit(ctx, build.Labor(), true)
		if err != nil {
			t.Fatal(err)
		}

		if err := ctrl.Wait(ctx, id); err != nil {
			t.Fatal(err)
		}

		l, err := ctrl.FetchLabor(ctx, id, true)
		if err != nil {
			t.Fatal(err)
		}

		switch test.fail {
		case false:
			if l.State != client.Completed {
				t.Errorf("Test %q: labor.State: got %v, want %v", test.desc, l.State, client.Completed)
			}
		case true:
			if l.State != client.Failed {
				t.Errorf("Test %q: labor.State: got %v, want %v", test.desc, l.State, client.Failed)
			}
		}
		for x, task := range l.Tasks {
			if task.State != test.tasks[x].state {
				t.Errorf("Test %q: task[%d].State: got %v, want %v", test.desc, x, task.State, test.tasks[x].state)
			}
		}
	}
}
