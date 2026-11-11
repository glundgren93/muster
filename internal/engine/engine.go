package engine

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/glundgren93/muster/internal/config"
	"github.com/glundgren93/muster/internal/hook"
	"github.com/glundgren93/muster/internal/layout"
	"github.com/glundgren93/muster/internal/state"
)

type Transition struct {
	ID   string       `json:"id"`
	From state.Status `json:"from"`
	To   state.Status `json:"to"`
}

type change struct {
	transition Transition
	run        state.Run
}

var statusHooks = map[state.Status]config.HookName{
	state.Queued:  config.OnQueued,
	state.Running: config.OnRunning,
	state.Waiting: config.OnWaiting,
	state.Done:    config.OnDone,
	state.Failed:  config.OnFailed,
}

type Engine struct {
	dir    layout.Dir
	config config.Config
	store  state.Store
	hooks  hook.Runner
	now    func() time.Time
}

type Options struct {
	Dir    layout.Dir
	Config config.Config
	Bin    string
	Now    func() time.Time
}

func New(options Options) (*Engine, error) {
	if err := options.Dir.Ensure(); err != nil {
		return nil, err
	}
	return &Engine{
		dir:    options.Dir,
		config: options.Config,
		store:  state.NewStore(options.Dir.StateFile(), options.Dir.LockFile()),
		hooks:  hook.Runner{MusterDir: options.Dir.Root, Bin: options.Bin},
		now:    options.Now,
	}, nil
}

func (e *Engine) Runs() ([]state.Run, error) {
	file, err := e.store.Load()
	if err != nil {
		return nil, err
	}
	return file.Runs, nil
}

func (e *Engine) Find(ref string) (state.Run, error) {
	file, err := e.store.Load()
	if err != nil {
		return state.Run{}, err
	}
	run, err := file.Resolve(ref)
	if err != nil {
		return state.Run{}, err
	}
	return *run, nil
}

func (e *Engine) Pass(ctx context.Context) ([]Transition, error) {
	reconciled, err := e.Reconcile(ctx)
	if err != nil {
		return reconciled, err
	}
	dispatched, err := e.Dispatch(ctx)
	return append(reconciled, dispatched...), err
}

func (e *Engine) timestamp() time.Time {
	return e.now().UTC()
}

func move(run *state.Run, to state.Status, at time.Time) change {
	from := run.Status
	run.Status = to
	run.UpdatedAt = at
	return change{transition: Transition{ID: run.ID, From: from, To: to}, run: *run}
}

func (e *Engine) publish(ctx context.Context, changes []change) []Transition {
	transitions := make([]Transition, 0, len(changes))
	for _, c := range changes {
		e.logf("%s", c.transition)
		transitions = append(transitions, c.transition)
	}
	for _, c := range changes {
		e.fireEvent(ctx, config.OnChange, c)
		if name, ok := statusHooks[c.transition.To]; ok {
			e.fireEvent(ctx, name, c)
		}
	}
	return transitions
}

func (e *Engine) fireEvent(ctx context.Context, name config.HookName, c change) {
	if _, configured := e.config.Hook(name); !configured {
		return
	}
	vars := e.vars(c.run)
	vars.Previous = string(c.transition.From)
	e.runHook(ctx, name, c.run.ID, vars)
}

func (e *Engine) runHook(ctx context.Context, name config.HookName, id string, vars hook.Vars) (string, error) {
	spec, configured := e.config.Hook(name)
	if !configured {
		err := &hook.Error{Hook: name, Reason: "not configured"}
		e.logf("%s: %s", id, err)
		return "", err
	}
	output, err := e.hooks.Run(ctx, name, spec, vars)
	if err != nil {
		e.logf("%s: %s", id, err)
	}
	return output, err
}

func (e *Engine) vars(run state.Run) hook.Vars {
	vars := hook.Vars{
		ID:        run.ID,
		Title:     run.Title,
		Workspace: run.Workspace,
		Agent:     run.Agent,
		Model:     run.Model,
		Source:    run.Source,
		Status:    string(run.Status),
	}
	if promptFile := e.dir.PromptFile(run.ID); fileExists(promptFile) {
		vars.PromptFile = promptFile
	}
	return vars
}

func (e *Engine) logf(format string, args ...any) {
	message := strings.ReplaceAll(fmt.Sprintf(format, args...), "\n", " | ")
	line := fmt.Sprintf("%s %s\n", e.timestamp().Format(time.RFC3339), message)
	log, err := os.OpenFile(e.dir.LogFile(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "muster: write log: %v\n", err)
		return
	}
	defer log.Close()
	if _, err := log.WriteString(line); err != nil {
		fmt.Fprintf(os.Stderr, "muster: write log: %v\n", err)
	}
}

func (t Transition) String() string {
	if t.From == "" {
		return fmt.Sprintf("%s: created as %s", t.ID, t.To)
	}
	return fmt.Sprintf("%s: %s -> %s", t.ID, t.From, t.To)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
