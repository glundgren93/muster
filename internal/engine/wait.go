package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/glundgren93/muster/internal/state"
)

var ErrWaitTimeout = errors.New("timed out waiting")

type WaitRequest struct {
	Refs     []string
	Any      bool
	Interval time.Duration
	Timeout  time.Duration
}

// Settled reports whether a run has stopped moving until someone acts on it.
func Settled(run state.Run) bool {
	return run.Status == state.Waiting || run.Status == state.Done || run.Status == state.Failed
}

// Wait runs passes until the runs have settled: all of them, or the first with Any.
// It returns the settled runs in argument order. On ErrWaitTimeout it returns
// every run as it stands. The timeout is only checked between passes, so it never
// interrupts a start hook.
func (e *Engine) Wait(ctx context.Context, request WaitRequest) ([]state.Run, error) {
	ids, err := e.resolveAll(request.Refs)
	if err != nil {
		return nil, err
	}
	var deadline time.Time
	if request.Timeout > 0 {
		deadline = time.Now().Add(request.Timeout)
	}
	for {
		if _, err := e.Pass(ctx); err != nil {
			return nil, err
		}
		runs, err := e.findAll(ids)
		if err != nil {
			return nil, err
		}
		settled := []state.Run{}
		for _, run := range runs {
			if Settled(run) {
				settled = append(settled, run)
			}
		}
		if len(settled) == len(runs) || (request.Any && len(settled) > 0) {
			return settled, nil
		}
		pause := request.Interval
		if !deadline.IsZero() {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return runs, ErrWaitTimeout
			}
			pause = min(pause, remaining)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pause):
		}
	}
}

func (e *Engine) resolveAll(refs []string) ([]string, error) {
	file, err := e.store.Load()
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		run, err := file.Resolve(ref)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(ids, run.ID) {
			ids = append(ids, run.ID)
		}
	}
	return ids, nil
}

func (e *Engine) findAll(ids []string) ([]state.Run, error) {
	file, err := e.store.Load()
	if err != nil {
		return nil, err
	}
	runs := make([]state.Run, 0, len(ids))
	for _, id := range ids {
		run := file.Find(id)
		if run == nil {
			return nil, fmt.Errorf("run %s was removed while waiting", id)
		}
		runs = append(runs, *run)
	}
	return runs, nil
}
