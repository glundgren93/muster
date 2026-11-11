package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/glundgren93/muster/internal/config"
	"github.com/glundgren93/muster/internal/hook"
	"github.com/glundgren93/muster/internal/state"
)

func (e *Engine) Reconcile(ctx context.Context) ([]Transition, error) {
	transitions, err := e.expireStarts(ctx)
	if err != nil {
		return transitions, err
	}
	file, err := e.store.Load()
	if err != nil {
		return transitions, err
	}
	for _, run := range file.Runs {
		if ctx.Err() != nil {
			break
		}
		if !observable(run) {
			continue
		}
		observed, err := e.reconcileRun(ctx, run)
		transitions = append(transitions, observed...)
		if err != nil {
			return transitions, err
		}
	}
	return transitions, ctx.Err()
}

func observable(run state.Run) bool {
	return (run.Status == state.Running || run.Status == state.Waiting) && run.Agent != ""
}

func (e *Engine) reconcileRun(ctx context.Context, observed state.Run) ([]Transition, error) {
	agentState, ok := e.agentStatus(ctx, observed)
	if !ok || agentState == state.AgentUnknown {
		return nil, nil
	}
	var changes []change
	err := e.store.Update(func(file *state.File) error {
		run := file.Find(observed.ID)
		if run == nil || run.Status != observed.Status || run.Agent != observed.Agent {
			return nil
		}
		now := e.timestamp()
		next := reconciledStatus(*run, agentState, now, e.config.StartGrace)
		if run.AgentState != agentState || (agentState == state.AgentWorking && !run.SeenWorking) {
			run.UpdatedAt = now
		}
		run.AgentState = agentState
		run.AgentStateAt = now
		if agentState == state.AgentWorking {
			run.SeenWorking = true
		}
		if next != run.Status {
			changes = append(changes, move(run, next, now))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return e.publish(ctx, changes), nil
}

func (e *Engine) agentStatus(ctx context.Context, run state.Run) (state.AgentState, bool) {
	output, err := e.runHook(ctx, config.AgentStatus, run.ID, e.vars(run))
	if err != nil {
		return "", false
	}
	value := hook.LastLine(output)
	agentState, known := state.ParseAgentState(value)
	if !known {
		e.logf("%s: %s", run.ID, fmt.Errorf("%s: returned unrecognised state %q", config.AgentStatus, value))
		return "", false
	}
	return agentState, true
}

func reconciledStatus(run state.Run, agentState state.AgentState, now time.Time, startGrace time.Duration) state.Status {
	switch agentState {
	case state.AgentWorking:
		return state.Running
	case state.AgentBlocked, state.AgentDone, state.AgentError, state.AgentGone:
		return state.Waiting
	case state.AgentIdle:
		if run.SeenWorking || now.Sub(run.StartedAt) > startGrace {
			return state.Waiting
		}
	}
	return run.Status
}
