package engine

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/glundgren93/muster/internal/state"
)

func (e *Engine) Done(ctx context.Context, ref string) (state.Run, error) {
	var result state.Run
	var changes []change
	err := e.store.Update(func(file *state.File) error {
		run, err := file.Resolve(ref)
		if err != nil {
			return err
		}
		switch run.Status {
		case state.Starting:
			return fmt.Errorf("run %s is starting; try again when it has started", run.ID)
		case state.Done:
		default:
			now := e.timestamp()
			run.FinishedAt = now
			changes = append(changes, move(run, state.Done, now))
		}
		result = *run
		return nil
	})
	if err != nil {
		return state.Run{}, err
	}
	e.publish(ctx, changes)
	return result, nil
}

func (e *Engine) Retry(ctx context.Context, ref string) (state.Run, error) {
	var id string
	var changes []change
	err := e.store.Update(func(file *state.File) error {
		run, err := file.Resolve(ref)
		if err != nil {
			return err
		}
		if run.Status != state.Failed {
			return fmt.Errorf("run %s is %s, only failed runs can be retried", run.ID, run.Status)
		}
		now := e.timestamp()
		run.Error = ""
		run.QueuedAt = now
		run.FinishedAt = time.Time{}
		changes = append(changes, move(run, state.Queued, now))
		id = run.ID
		return nil
	})
	if err != nil {
		return state.Run{}, err
	}
	e.publish(ctx, changes)
	if _, err := e.Dispatch(ctx); err != nil {
		return state.Run{}, err
	}
	return e.Find(id)
}

func (e *Engine) Remove(ref string, force bool) (state.Run, error) {
	var removed state.Run
	err := e.store.Update(func(file *state.File) error {
		run, err := file.Resolve(ref)
		if err != nil {
			return err
		}
		if (run.Status == state.Starting || run.Status == state.Running) && !force {
			return fmt.Errorf("run %s is %s; pass --force to remove it anyway", run.ID, run.Status)
		}
		removed = *run
		file.Remove(run.ID)
		return nil
	})
	if err != nil {
		return state.Run{}, err
	}
	if err := os.RemoveAll(e.dir.RunDir(removed.ID)); err != nil {
		return removed, fmt.Errorf("remove %s: %w", e.dir.RunDir(removed.ID), err)
	}
	return removed, nil
}
