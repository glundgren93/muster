package engine

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/glundgren93/muster/internal/config"
	"github.com/glundgren93/muster/internal/state"
)

var ErrAgentBlocked = errors.New("the agent is blocked on a question or a permission prompt; look at it with muster read, or pass --force to send anyway")

func (e *Engine) Follow(ctx context.Context, ref, message string, queue bool) (state.Run, error) {
	if strings.TrimSpace(message) == "" {
		return state.Run{}, errors.New("the follow-up is empty")
	}
	var id string
	var changes []change
	err := e.store.Update(func(file *state.File) error {
		run, err := file.Resolve(ref)
		if err != nil {
			return err
		}
		if run.Sessions == 0 {
			return fmt.Errorf("run %s has not had an agent session yet; change the task instead of following up", run.ID)
		}
		switch run.Status {
		case state.Waiting, state.Done, state.Failed, state.Queued:
		default:
			return fmt.Errorf("run %s is %s; follow up when it is waiting, or talk to the agent with muster send", run.ID, run.Status)
		}
		now := e.timestamp()
		run.FollowUps = append(run.FollowUps, state.FollowUp{Text: message, CreatedAt: now})
		run.UpdatedAt = now
		if run.Status != state.Queued {
			run.Error = ""
			run.QueuedAt = now
			run.FinishedAt = time.Time{}
			changes = append(changes, move(run, state.Queued, now))
		}
		id = run.ID
		return nil
	})
	if err != nil {
		return state.Run{}, err
	}
	e.publish(ctx, changes)
	if !queue {
		if _, err := e.Dispatch(ctx); err != nil {
			return state.Run{}, err
		}
	}
	return e.Find(id)
}

func (e *Engine) followUpPrompt(ctx context.Context, run state.Run) string {
	report := ""
	if _, configured := e.config.Hook(config.LastReport); configured {
		if output, err := e.runHook(ctx, config.LastReport, run.ID, e.vars(run)); err == nil {
			report = strings.TrimSpace(output)
		}
	}
	if _, configured := e.config.Hook(config.StopAgent); configured && run.Agent != "" {
		e.runHook(ctx, config.StopAgent, run.ID, e.vars(run))
	}
	return FollowUpPrompt(run, report)
}

func FollowUpPrompt(run state.Run, report string) string {
	var sent, pending []string
	for _, followUp := range run.FollowUps {
		if followUp.SentAt.IsZero() {
			pending = append(pending, followUp.Text)
		} else {
			sent = append(sent, followUp.Text)
		}
	}
	var b strings.Builder
	b.WriteString(strings.TrimRight(run.Prompt, "\n"))
	b.WriteString("\n\n---\n\nAn earlier agent session already worked on this task in this workspace.\n")
	if len(sent) > 0 {
		b.WriteString("\nEarlier follow-ups:\n")
		for _, text := range sent {
			b.WriteString("- " + text + "\n")
		}
	}
	if report != "" {
		b.WriteString("\nIts last report:\n" + report + "\n")
	}
	b.WriteString("\nFollow-up:\n")
	for _, text := range pending {
		b.WriteString(text + "\n")
	}
	return b.String()
}

func (e *Engine) Read(ctx context.Context, ref string, lines int) (string, error) {
	run, err := e.liveRun(ref)
	if err != nil {
		return "", err
	}
	vars := e.vars(run)
	vars.Lines = strconv.Itoa(lines)
	return e.runHook(ctx, config.ReadAgent, run.ID, vars)
}

func (e *Engine) Send(ctx context.Context, ref, message string, force bool) error {
	if message == "" {
		return errors.New("the message is empty")
	}
	run, err := e.liveRun(ref)
	if err != nil {
		return err
	}
	if !force {
		if agentState, ok := e.agentStatus(ctx, run); ok && agentState == state.AgentBlocked {
			return ErrAgentBlocked
		}
	}
	vars := e.vars(run)
	vars.Message = message
	_, err = e.runHook(ctx, config.SendAgent, run.ID, vars)
	return err
}

func (e *Engine) liveRun(ref string) (state.Run, error) {
	run, err := e.Find(ref)
	if err != nil {
		return state.Run{}, err
	}
	if run.Agent == "" {
		return state.Run{}, fmt.Errorf("run %s has no agent session", run.ID)
	}
	return run, nil
}
