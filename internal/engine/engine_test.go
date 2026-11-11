package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glundgren93/muster/internal/config"
	"github.com/glundgren93/muster/internal/layout"
	"github.com/glundgren93/muster/internal/state"
)

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type harness struct {
	t      *testing.T
	dir    layout.Dir
	config config.Config
	clock  *clock
	engine *Engine
}

const (
	fakeStartAgent  = `echo {id} >> "$MUSTER_DIR/started"; echo agent-{id}-$(wc -l < "$MUSTER_DIR/started" | tr -d ' ')`
	fakeAgentStatus = `cat "$MUSTER_DIR/agent-state-$MUSTER_RUN_ID"`
	fakeOnChange    = `echo "$MUSTER_RUN_ID ${MUSTER_PREVIOUS_STATUS:--} $MUSTER_STATUS" >> "$MUSTER_DIR/events"`
)

func newHarness(t *testing.T, hooks map[config.HookName]string) *harness {
	t.Helper()
	cfg := config.Default()
	cfg.StartGrace = time.Minute
	all := map[config.HookName]string{config.StartAgent: fakeStartAgent, config.AgentStatus: fakeAgentStatus}
	for name, command := range hooks {
		all[name] = command
	}
	for name, command := range all {
		if command != "" {
			cfg.Hooks[name] = config.Hook{Command: command, Timeout: 10 * time.Second}
		}
	}
	h := &harness{t: t, dir: layout.Dir{Root: t.TempDir()}, config: cfg, clock: &clock{now: time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)}}
	h.engine = h.newEngine()
	return h
}

func (h *harness) newEngine() *Engine {
	h.t.Helper()
	engine, err := New(Options{Dir: h.dir, Config: h.config, Bin: "muster", Now: h.clock.Now})
	if err != nil {
		h.t.Fatal(err)
	}
	return engine
}

func (h *harness) setConfig(mutate func(*config.Config)) {
	mutate(&h.config)
	h.engine = h.newEngine()
}

func (h *harness) start(id, prompt string) state.Run {
	h.t.Helper()
	result, err := h.engine.Start(context.Background(), StartRequest{ID: id, Prompt: prompt})
	if err != nil {
		h.t.Fatal(err)
	}
	return result.Run
}

func (h *harness) agentReports(id string, agentState string) {
	h.t.Helper()
	if err := os.WriteFile(filepath.Join(h.dir.Root, "agent-state-"+id), []byte(agentState+"\n"), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) reconcile() []Transition {
	h.t.Helper()
	transitions, err := h.engine.Reconcile(context.Background())
	if err != nil {
		h.t.Fatal(err)
	}
	return transitions
}

func (h *harness) dispatch() []Transition {
	h.t.Helper()
	transitions, err := h.engine.Dispatch(context.Background())
	if err != nil {
		h.t.Fatal(err)
	}
	return transitions
}

func (h *harness) run(id string) state.Run {
	h.t.Helper()
	run, err := h.engine.Find(id)
	if err != nil {
		h.t.Fatal(err)
	}
	return run
}

func (h *harness) expectStatus(id string, want state.Status) state.Run {
	h.t.Helper()
	run := h.run(id)
	if run.Status != want {
		h.t.Fatalf("run %s is %s (error %q), want %s", id, run.Status, run.Error, want)
	}
	return run
}

func (h *harness) lines(name string) []string {
	h.t.Helper()
	data, err := os.ReadFile(filepath.Join(h.dir.Root, name))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		h.t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

func (h *harness) mutate(id string, change func(*state.Run)) {
	h.t.Helper()
	store := state.NewStore(h.dir.StateFile(), h.dir.LockFile())
	if err := store.Update(func(file *state.File) error {
		change(file.Find(id))
		return nil
	}); err != nil {
		h.t.Fatal(err)
	}
}

func TestFirstStartReachesRunning(t *testing.T) {
	h := newHarness(t, nil)
	run := h.start("first", "Fix the login page\n\nIt returns 500.")

	if run.Status != state.Running || run.Sessions != 1 || run.Agent != "agent-first-1" || run.SeenWorking {
		t.Fatalf("unexpected run %+v", run)
	}
	if run.Workspace != h.dir.WorkspaceDir("first") || !isDir(run.Workspace) {
		t.Fatalf("workspace %q was not created", run.Workspace)
	}
	prompt, err := os.ReadFile(h.dir.PromptFile("first"))
	if err != nil || string(prompt) != "Fix the login page\n\nIt returns 500." {
		t.Fatalf("prompt file %q, %v", prompt, err)
	}
	if !run.StartedAt.Equal(h.clock.Now()) {
		t.Fatalf("started_at %s", run.StartedAt)
	}
}

func TestReconcileFollowsTheAgent(t *testing.T) {
	h := newHarness(t, nil)
	h.start("r", "task")

	h.agentReports("r", "idle")
	h.clock.Advance(30 * time.Second)
	h.reconcile()
	run := h.expectStatus("r", state.Running)
	if run.AgentState != state.AgentIdle {
		t.Fatalf("agent_state %q, want idle stored even when the status holds", run.AgentState)
	}

	h.agentReports("r", "working")
	h.reconcile()
	if run := h.expectStatus("r", state.Running); !run.SeenWorking {
		t.Fatal("seen_working not set")
	}

	h.agentReports("r", "idle")
	transitions := h.reconcile()
	h.expectStatus("r", state.Waiting)
	if len(transitions) != 1 || transitions[0] != (Transition{ID: "r", From: state.Running, To: state.Waiting}) {
		t.Fatalf("transitions %v", transitions)
	}

	h.agentReports("r", "working")
	h.reconcile()
	h.expectStatus("r", state.Running)
}

func TestIdleAfterStartGraceIsWaiting(t *testing.T) {
	h := newHarness(t, nil)
	h.start("r", "task")
	h.agentReports("r", "idle")
	h.clock.Advance(h.config.StartGrace + time.Second)
	h.reconcile()
	h.expectStatus("r", state.Waiting)
}

func TestStoppedAgentStatesAreWaiting(t *testing.T) {
	for _, agentState := range []string{"blocked", "gone", "done", "error"} {
		t.Run(agentState, func(t *testing.T) {
			h := newHarness(t, nil)
			h.start("r", "task")
			h.agentReports("r", agentState)
			h.reconcile()
			if run := h.expectStatus("r", state.Waiting); string(run.AgentState) != agentState {
				t.Fatalf("agent_state %q", run.AgentState)
			}
		})
	}
}

func TestUnusableAgentStatusChangesNothing(t *testing.T) {
	for name, report := range map[string]string{"unknown": "unknown", "unrecognised": "thinking", "hook fails": ""} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, nil)
			h.start("r", "task")
			h.agentReports("r", "working")
			h.reconcile()
			before := h.run("r")
			if report == "" {
				os.Remove(filepath.Join(h.dir.Root, "agent-state-r"))
			} else {
				h.agentReports("r", report)
			}
			h.clock.Advance(time.Hour)
			if transitions := h.reconcile(); len(transitions) != 0 {
				t.Fatalf("transitions %v", transitions)
			}
			after := h.run("r")
			if after.Status != state.Running || after.AgentState != state.AgentWorking || !after.AgentStateAt.Equal(before.AgentStateAt) || !after.UpdatedAt.Equal(before.UpdatedAt) {
				t.Fatalf("run changed: %+v", after)
			}
		})
	}
}

func TestMaxRunningHoldsRunsAndReleasesThemInQueueOrder(t *testing.T) {
	h := newHarness(t, nil)
	h.setConfig(func(c *config.Config) { c.MaxRunning = 1 })
	h.start("a", "task a")
	h.clock.Advance(time.Second)
	h.start("b", "task b")
	h.clock.Advance(time.Second)
	h.start("c", "task c")
	h.expectStatus("a", state.Running)
	h.expectStatus("b", state.Queued)
	h.expectStatus("c", state.Queued)
	h.mutate("c", func(run *state.Run) { run.QueuedAt = run.QueuedAt.Add(-time.Hour) })

	if _, err := h.engine.Done(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	h.dispatch()
	h.expectStatus("c", state.Running)
	h.expectStatus("b", state.Queued)

	if _, err := h.engine.Done(context.Background(), "c"); err != nil {
		t.Fatal(err)
	}
	h.dispatch()
	h.expectStatus("b", state.Running)
	if got := h.lines("started"); strings.Join(got, ",") != "a,c,b" {
		t.Fatalf("start order %v", got)
	}
}

func TestMaxRunningZeroDispatchesNothing(t *testing.T) {
	h := newHarness(t, nil)
	h.setConfig(func(c *config.Config) { c.MaxRunning = 0 })
	h.start("a", "task")
	h.expectStatus("a", state.Queued)
}

func TestFailingStartAgentFailsWithStderrAndRetryRequeues(t *testing.T) {
	h := newHarness(t, map[config.HookName]string{
		config.CreateWorkspace: `echo created >> "$MUSTER_DIR/workspaces-created"; mkdir -p "$MUSTER_DIR/ws/{id}" && echo "$MUSTER_DIR/ws/{id}"`,
		config.StartAgent:      `echo "pane limit reached" >&2; exit 3`,
	})
	run := h.start("r", "task")
	if run.Status != state.Failed || !strings.Contains(run.Error, "start_agent") || !strings.Contains(run.Error, "pane limit reached") {
		t.Fatalf("unexpected run %+v", run)
	}
	if run.Workspace == "" || run.FinishedAt.IsZero() {
		t.Fatalf("failed start should keep the workspace and set finished_at: %+v", run)
	}

	h.setConfig(func(c *config.Config) {
		c.Hooks[config.StartAgent] = config.Hook{Command: fakeStartAgent, Timeout: 10 * time.Second}
	})
	retried, err := h.engine.Retry(context.Background(), "r")
	if err != nil {
		t.Fatal(err)
	}
	if retried.Status != state.Running || retried.Error != "" || !retried.FinishedAt.IsZero() || retried.Workspace != run.Workspace {
		t.Fatalf("unexpected retried run %+v", retried)
	}
	if got := h.lines("workspaces-created"); len(got) != 1 {
		t.Fatalf("workspace created %d times", len(got))
	}
}

func TestRetryRefusesRunsThatHaveNotFailed(t *testing.T) {
	h := newHarness(t, nil)
	h.start("r", "task")
	if _, err := h.engine.Retry(context.Background(), "r"); err == nil {
		t.Fatal("retry of a running run should fail")
	}
}

func TestStaleStartingRunFails(t *testing.T) {
	h := newHarness(t, nil)
	h.setConfig(func(c *config.Config) { c.MaxRunning = 0 })
	h.start("stale", "task")
	h.start("fresh", "task")
	h.mutate("stale", func(run *state.Run) {
		run.Status = state.Starting
		run.StartingAt = h.clock.Now().Add(-h.config.StartTimeout() - startInterruptedMargin - time.Second)
	})
	h.mutate("fresh", func(run *state.Run) {
		run.Status = state.Starting
		run.StartingAt = h.clock.Now().Add(-time.Minute)
	})
	h.reconcile()
	if run := h.expectStatus("stale", state.Failed); run.Error != startInterruptedError {
		t.Fatalf("error %q", run.Error)
	}
	h.expectStatus("fresh", state.Starting)
}

func TestEventHooksFireOncePerTransitionInOrder(t *testing.T) {
	h := newHarness(t, map[config.HookName]string{
		config.OnChange:  fakeOnChange,
		config.OnQueued:  `echo "on_queued $MUSTER_RUN_ID" >> "$MUSTER_DIR/events"`,
		config.OnRunning: `echo "on_running $MUSTER_RUN_ID" >> "$MUSTER_DIR/events"`,
		config.OnWaiting: `echo on_waiting {id} >> "$MUSTER_DIR/events"`,
		config.OnDone:    `echo "on_done $MUSTER_RUN_ID" >> "$MUSTER_DIR/events"`,
	})
	h.start("r", "task")
	h.agentReports("r", "blocked")
	h.reconcile()
	h.reconcile()
	if _, err := h.engine.Done(context.Background(), "r"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"r - queued", "on_queued r",
		"r queued starting",
		"r starting running", "on_running r",
		"r running waiting", "on_waiting r",
		"r waiting done", "on_done r",
	}
	if got := h.lines("events"); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("events:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestFailingEventHookDoesNotAffectTheRun(t *testing.T) {
	h := newHarness(t, map[config.HookName]string{config.OnChange: `exit 1`, config.OnRunning: `exit 1`})
	run := h.start("r", "task")
	if run.Status != state.Running {
		t.Fatalf("status %s", run.Status)
	}
}

func TestSameSourceTwiceCreatesOneRun(t *testing.T) {
	h := newHarness(t, nil)
	first, err := h.engine.Start(context.Background(), StartRequest{Prompt: "task", Source: "obsidian:Tasks/a.md"})
	if err != nil || !first.Created {
		t.Fatalf("%+v, %v", first, err)
	}
	second, err := h.engine.Start(context.Background(), StartRequest{Prompt: "other text", Source: "obsidian:Tasks/a.md"})
	if err != nil || second.Created || second.Run.ID != first.Run.ID {
		t.Fatalf("%+v, %v", second, err)
	}
	runs, _ := h.engine.Runs()
	if len(runs) != 1 || len(h.lines("started")) != 1 {
		t.Fatalf("runs %d, started %v", len(runs), h.lines("started"))
	}
}

func TestOverlappingPassesStartARunOnce(t *testing.T) {
	h := newHarness(t, map[config.HookName]string{config.StartAgent: `sleep 0.3; ` + fakeStartAgent})
	if _, err := h.engine.Start(context.Background(), StartRequest{ID: "r", Prompt: "task", Queue: true}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		engine := h.newEngine()
		go func() {
			defer wg.Done()
			if _, err := engine.Pass(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	h.expectStatus("r", state.Running)
	if got := h.lines("started"); len(got) != 1 {
		t.Fatalf("started %d times", len(got))
	}
}

func TestPickModelSeesThePromptAndFallsBackToDefault(t *testing.T) {
	h := newHarness(t, map[config.HookName]string{config.PickModel: `head -n 1 "$MUSTER_PROMPT_FILE" | grep -o 'model=[a-z]*' | cut -d= -f2`})
	h.setConfig(func(c *config.Config) { c.DefaultModel = "fallback" })
	if run := h.start("a", "model=opus please\nrest"); run.Model != "opus" {
		t.Fatalf("model %q", run.Model)
	}
	if run := h.start("b", "no hint here"); run.Model != "fallback" {
		t.Fatalf("model %q", run.Model)
	}
	result, err := h.engine.Start(context.Background(), StartRequest{ID: "c", Prompt: "model=opus", Model: "given"})
	if err != nil || result.Run.Model != "given" {
		t.Fatalf("model %q, %v", result.Run.Model, err)
	}
}

func TestInterruptedStartFails(t *testing.T) {
	h := newHarness(t, map[config.HookName]string{config.StartAgent: `sleep 5`})
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(200*time.Millisecond, cancel)
	result, _ := h.engine.Start(ctx, StartRequest{ID: "r", Prompt: "task"})
	if result.Run.ID != "r" {
		t.Fatalf("result %+v", result)
	}
	if run := h.expectStatus("r", state.Failed); !strings.Contains(run.Error, "interrupted") {
		t.Fatalf("error %q", run.Error)
	}
}

func TestDoneRefusesStartingRun(t *testing.T) {
	h := newHarness(t, nil)
	h.setConfig(func(c *config.Config) { c.MaxRunning = 0 })
	h.start("r", "task")
	h.mutate("r", func(run *state.Run) { run.Status = state.Starting; run.StartingAt = h.clock.Now() })
	if _, err := h.engine.Done(context.Background(), "r"); err == nil {
		t.Fatal("done should refuse a starting run")
	}
}

func TestRemoveRefusesActiveRunsUnlessForced(t *testing.T) {
	h := newHarness(t, nil)
	h.start("r", "task")
	if _, err := h.engine.Remove("r", false); err == nil {
		t.Fatal("rm of a running run should need --force")
	}
	if _, err := h.engine.Remove("r", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(h.dir.RunDir("r")); !os.IsNotExist(err) {
		t.Fatal("runs/<id> kept")
	}
	if !isDir(h.dir.WorkspaceDir("r")) {
		t.Fatal("workspace removed")
	}
}

func TestDefaultIDsAreUniqueSlugs(t *testing.T) {
	h := newHarness(t, nil)
	h.setConfig(func(c *config.Config) { c.MaxRunning = 0 })
	prompt := "Fix the Login page: it's BROKEN again, today!!"
	ids := []string{}
	for range 3 {
		result, err := h.engine.Start(context.Background(), StartRequest{Prompt: prompt})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, result.Run.ID)
	}
	if got := strings.Join(ids, " "); got != "fix-the-login-page-it-s fix-the-login-page-it-s-2 fix-the-login-page-it-s-3" {
		t.Fatalf("ids %s", got)
	}
}
