package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/glundgren93/muster/internal/config"
	"github.com/glundgren93/muster/internal/hook"
	"github.com/glundgren93/muster/internal/state"
)

const (
	startInterruptedMargin = time.Minute
	startInterruptedError  = "start interrupted"
)

func StuckStarting(cfg config.Config, runs []state.Run, now time.Time) []state.Run {
	deadline := cfg.StartTimeout() + startInterruptedMargin
	var stuck []state.Run
	for _, run := range runs {
		if run.Status == state.Starting && now.Sub(run.StartingAt) > deadline {
			stuck = append(stuck, run)
		}
	}
	return stuck
}

func (e *Engine) Dispatch(ctx context.Context) ([]Transition, error) {
	transitions, err := e.expireStarts(ctx)
	if err != nil {
		return transitions, err
	}
	for ctx.Err() == nil {
		claimed, claimTransitions, err := e.claimNext(ctx)
		transitions = append(transitions, claimTransitions...)
		if err != nil || claimed == nil {
			return transitions, err
		}
		started, startErr := e.startSession(ctx, *claimed)
		finished, err := e.finishStart(ctx, *claimed, started, startErr)
		transitions = append(transitions, finished...)
		if err != nil {
			return transitions, err
		}
	}
	return transitions, ctx.Err()
}

func (e *Engine) expireStarts(ctx context.Context) ([]Transition, error) {
	var changes []change
	err := e.store.Update(func(file *state.File) error {
		now := e.timestamp()
		for _, stuck := range StuckStarting(e.config, file.Runs, now) {
			run := file.Find(stuck.ID)
			run.Error = startInterruptedError
			run.FinishedAt = now
			changes = append(changes, move(run, state.Failed, now))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return e.publish(ctx, changes), nil
}

func (e *Engine) claimNext(ctx context.Context) (*state.Run, []Transition, error) {
	var claimed *state.Run
	var changes []change
	err := e.store.Update(func(file *state.File) error {
		active := 0
		var next *state.Run
		for i := range file.Runs {
			run := &file.Runs[i]
			switch run.Status {
			case state.Starting, state.Running:
				active++
			case state.Queued:
				if next == nil || run.QueuedAt.Before(next.QueuedAt) {
					next = run
				}
			}
		}
		if next == nil || active >= e.config.MaxRunning {
			return nil
		}
		now := e.timestamp()
		next.StartingAt = now
		changes = append(changes, move(next, state.Starting, now))
		snapshot := *next
		claimed = &snapshot
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return claimed, e.publish(ctx, changes), nil
}

func (e *Engine) startSession(ctx context.Context, run state.Run) (state.Run, error) {
	if run.Sessions == 0 {
		if err := e.writePrompt(run, run.Prompt); err != nil {
			return run, err
		}
		if run.Model == "" {
			model, err := e.chooseModel(ctx, run)
			if err != nil {
				return run, err
			}
			run.Model = model
		}
	}
	if run.Workspace == "" {
		workspace, err := e.createWorkspace(ctx, run)
		if err != nil {
			return run, err
		}
		run.Workspace = workspace
	} else if !isDir(run.Workspace) {
		return run, fmt.Errorf("workspace %s no longer exists", run.Workspace)
	}
	if run.Sessions > 0 {
		if err := e.writePrompt(run, e.followUpPrompt(ctx, run)); err != nil {
			return run, err
		}
	}
	agent, err := e.startAgent(ctx, run)
	if err != nil {
		return run, err
	}
	run.Agent = agent
	return run, nil
}

func (e *Engine) chooseModel(ctx context.Context, run state.Run) (string, error) {
	if _, configured := e.config.Hook(config.PickModel); !configured {
		return e.config.DefaultModel, nil
	}
	output, err := e.runHook(ctx, config.PickModel, run.ID, e.vars(run))
	if err != nil {
		return "", err
	}
	if model := hook.LastLine(output); model != "" {
		return model, nil
	}
	return e.config.DefaultModel, nil
}

func (e *Engine) createWorkspace(ctx context.Context, run state.Run) (string, error) {
	if _, configured := e.config.Hook(config.CreateWorkspace); !configured {
		workspace := e.dir.WorkspaceDir(run.ID)
		if err := os.MkdirAll(workspace, 0o755); err != nil {
			return "", fmt.Errorf("create workspace: %w", err)
		}
		return workspace, nil
	}
	output, err := e.runHook(ctx, config.CreateWorkspace, run.ID, e.vars(run))
	if err != nil {
		return "", err
	}
	workspace := hook.LastLine(output)
	if !filepath.IsAbs(workspace) || !isDir(workspace) {
		err := &hook.Error{Hook: config.CreateWorkspace, Reason: fmt.Sprintf("returned %q, which is not the absolute path of an existing directory", workspace)}
		e.logf("%s: %s", run.ID, err)
		return "", err
	}
	return workspace, nil
}

func (e *Engine) writePrompt(run state.Run, prompt string) error {
	if err := os.MkdirAll(e.dir.RunDir(run.ID), 0o755); err != nil {
		return fmt.Errorf("write prompt: %w", err)
	}
	if err := os.WriteFile(e.dir.PromptFile(run.ID), []byte(prompt), 0o644); err != nil {
		return fmt.Errorf("write prompt: %w", err)
	}
	return nil
}

func (e *Engine) startAgent(ctx context.Context, run state.Run) (string, error) {
	output, err := e.runHook(ctx, config.StartAgent, run.ID, e.vars(run))
	if err != nil {
		return "", err
	}
	agent := hook.LastLine(output)
	if agent == "" {
		err := &hook.Error{Hook: config.StartAgent, Reason: "returned no agent handle"}
		e.logf("%s: %s", run.ID, err)
		return "", err
	}
	return agent, nil
}

func (e *Engine) finishStart(ctx context.Context, claimed, started state.Run, startErr error) ([]Transition, error) {
	var changes []change
	err := e.store.Update(func(file *state.File) error {
		run := file.Find(claimed.ID)
		if run == nil || run.Status != state.Starting || !run.StartingAt.Equal(claimed.StartingAt) {
			e.logf("%s: discarded the start result because the run changed while it was starting", claimed.ID)
			return nil
		}
		now := e.timestamp()
		run.Model = started.Model
		run.Workspace = started.Workspace
		if startErr != nil {
			run.Error = startErr.Error()
			run.FinishedAt = now
			changes = append(changes, move(run, state.Failed, now))
			return nil
		}
		run.Agent = started.Agent
		run.AgentState = ""
		run.AgentStateAt = time.Time{}
		run.StartedAt = now
		run.SeenWorking = false
		run.Sessions++
		for i := range min(len(claimed.FollowUps), len(run.FollowUps)) {
			if run.FollowUps[i].SentAt.IsZero() {
				run.FollowUps[i].SentAt = now
			}
		}
		changes = append(changes, move(run, state.Running, now))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return e.publish(ctx, changes), nil
}
