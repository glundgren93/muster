package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/glundgren93/muster/internal/config"
	"github.com/glundgren93/muster/internal/state"
)

const waitTick = 5 * time.Millisecond

func (h *harness) wait(request WaitRequest) ([]state.Run, error) {
	h.t.Helper()
	if request.Interval == 0 {
		request.Interval = waitTick
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return h.engine.Wait(ctx, request)
}

func ids(runs []state.Run) string {
	out := ""
	for _, run := range runs {
		out += run.ID + ","
	}
	return out
}

func TestWaitReturnsWhenTheAgentStops(t *testing.T) {
	h := newHarness(t, nil)
	h.start("a", "task a")
	h.agentReports("a", "working")
	go func() {
		time.Sleep(10 * waitTick)
		h.agentReports("a", "done")
	}()
	runs, err := h.wait(WaitRequest{Refs: []string{"a"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != state.Waiting || runs[0].AgentState != state.AgentDone {
		t.Fatalf("got %+v", runs)
	}
	if !h.run("a").SeenWorking {
		t.Fatal("wait returned before it saw the agent working")
	}
}

func TestWaitReturnsAtOnceForSettledRuns(t *testing.T) {
	h := newHarness(t, nil)
	h.start("a", "task a")
	h.agentReports("a", "blocked")
	h.reconcile()
	h.start("b", "task b")
	if _, err := h.engine.Done(context.Background(), "b"); err != nil {
		t.Fatal(err)
	}
	runs, err := h.wait(WaitRequest{Refs: []string{"b", "a", "b"}, Interval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if ids(runs) != "b,a," {
		t.Fatalf("got %s", ids(runs))
	}
}

func TestWaitAnyReturnsTheFirstRunToStop(t *testing.T) {
	h := newHarness(t, nil)
	h.start("a", "task a")
	h.start("b", "task b")
	h.agentReports("a", "working")
	h.agentReports("b", "working")
	go func() {
		time.Sleep(10 * waitTick)
		h.agentReports("b", "done")
	}()
	runs, err := h.wait(WaitRequest{Refs: []string{"a", "b"}, Any: true})
	if err != nil {
		t.Fatal(err)
	}
	if ids(runs) != "b," {
		t.Fatalf("got %s", ids(runs))
	}
	h.expectStatus("a", state.Running)
}

func TestWaitForAllKeepsWaitingForTheLastRun(t *testing.T) {
	h := newHarness(t, nil)
	h.start("a", "task a")
	h.start("b", "task b")
	h.agentReports("a", "done")
	h.agentReports("b", "working")
	runs, err := h.wait(WaitRequest{Refs: []string{"a", "b"}, Timeout: 10 * waitTick})
	if !errors.Is(err, ErrWaitTimeout) {
		t.Fatalf("got %v, want a timeout", err)
	}
	if ids(runs) != "a,b," || runs[0].Status != state.Waiting || runs[1].Status != state.Running {
		t.Fatalf("got %+v", runs)
	}
}

func TestWaitStartsQueuedRunsAsSlotsFree(t *testing.T) {
	h := newHarness(t, nil)
	h.setConfig(func(c *config.Config) { c.MaxRunning = 1 })
	h.start("a", "task a")
	h.start("b", "task b")
	h.agentReports("a", "working")
	h.agentReports("b", "done")
	h.expectStatus("b", state.Queued)
	go func() {
		time.Sleep(10 * waitTick)
		h.agentReports("a", "done")
	}()
	runs, err := h.wait(WaitRequest{Refs: []string{"b"}})
	if err != nil {
		t.Fatal(err)
	}
	if ids(runs) != "b," || runs[0].Status != state.Waiting || runs[0].Sessions != 1 {
		t.Fatalf("got %+v", runs)
	}
}

func TestWaitReportsAFailedStart(t *testing.T) {
	h := newHarness(t, map[config.HookName]string{config.StartAgent: "echo no pane >&2; exit 1"})
	h.start("a", "task a")
	runs, err := h.wait(WaitRequest{Refs: []string{"a"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != state.Failed {
		t.Fatalf("got %+v", runs)
	}
}

func TestWaitErrors(t *testing.T) {
	h := newHarness(t, nil)
	h.start("a", "task a")
	h.agentReports("a", "working")
	if _, err := h.wait(WaitRequest{Refs: []string{"a", "nope"}}); err == nil {
		t.Fatal("an unknown run was accepted")
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * waitTick)
		cancel()
	}()
	if _, err := h.engine.Wait(ctx, WaitRequest{Refs: []string{"a"}, Interval: waitTick}); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want cancelled", err)
	}

	go func() {
		time.Sleep(10 * waitTick)
		if _, err := h.engine.Remove("a", true); err != nil {
			t.Error(err)
		}
	}()
	if _, err := h.wait(WaitRequest{Refs: []string{"a"}}); err == nil {
		t.Fatal("waiting for a removed run succeeded")
	}
}
