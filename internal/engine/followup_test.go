package engine

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/glundgren93/muster/internal/config"
	"github.com/glundgren93/muster/internal/state"
)

func followUpHooks() map[config.HookName]string {
	return map[config.HookName]string{
		config.LastReport: `echo "report from $MUSTER_AGENT"`,
		config.StopAgent:  `echo {agent} >> "$MUSTER_DIR/stopped"`,
		config.StartAgent: `cp "$MUSTER_PROMPT_FILE" "$MUSTER_DIR/prompt-$(wc -l < "$MUSTER_DIR/started" 2>/dev/null | tr -d ' ')"; ` + fakeStartAgent,
	}
}

func TestFollowUpStartsANewSessionWithContext(t *testing.T) {
	h := newHarness(t, followUpHooks())
	h.start("r", "Fix the login page")
	h.agentReports("r", "blocked")
	h.reconcile()
	h.clock.Advance(time.Minute)

	run, err := h.engine.Follow(context.Background(), "r", "Also add a test", false)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != state.Running || run.Sessions != 2 || run.Agent != "agent-r-2" || run.AgentState != "" || len(run.FollowUps) != 1 || run.FollowUps[0].SentAt.IsZero() {
		t.Fatalf("unexpected run %+v", run)
	}
	if got := h.lines("stopped"); len(got) != 1 || got[0] != "agent-r-1" {
		t.Fatalf("stop_agent calls %v", got)
	}
	prompt, err := os.ReadFile(h.dir.PromptFile("r"))
	if err != nil {
		t.Fatal(err)
	}
	want := "Fix the login page\n\n---\n\nAn earlier agent session already worked on this task in this workspace.\n\nIts last report:\nreport from agent-r-1\n\nFollow-up:\nAlso add a test\n"
	if string(prompt) != want {
		t.Fatalf("prompt:\n%s\nwant:\n%s", prompt, want)
	}
	if first, _ := os.ReadFile(h.dir.Root + "/prompt-"); string(first) != "Fix the login page" {
		t.Fatalf("first session prompt %q", first)
	}

	h.agentReports("r", "idle")
	h.clock.Advance(2 * time.Minute)
	h.reconcile()
	if _, err := h.engine.Done(context.Background(), "r"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.engine.Follow(context.Background(), "r", "And update the changelog", false); err != nil {
		t.Fatal(err)
	}
	prompt, _ = os.ReadFile(h.dir.PromptFile("r"))
	if !strings.Contains(string(prompt), "Earlier follow-ups:\n- Also add a test\n") || !strings.HasSuffix(string(prompt), "Follow-up:\nAnd update the changelog\n") {
		t.Fatalf("third prompt:\n%s", prompt)
	}
}

func TestFollowUpPromptOmitsEmptyBlocks(t *testing.T) {
	run := state.Run{Prompt: "Task\n", FollowUps: []state.FollowUp{{Text: "one"}, {Text: "two"}}}
	want := "Task\n\n---\n\nAn earlier agent session already worked on this task in this workspace.\n\nFollow-up:\none\ntwo\n"
	if got := FollowUpPrompt(run, ""); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestFailingLastReportAndStopAgentDoNotBlockAFollowUp(t *testing.T) {
	hooks := followUpHooks()
	hooks[config.LastReport] = `exit 1`
	hooks[config.StopAgent] = `exit 1`
	h := newHarness(t, hooks)
	h.start("r", "task")
	h.agentReports("r", "gone")
	h.reconcile()
	run, err := h.engine.Follow(context.Background(), "r", "again", false)
	if err != nil || run.Status != state.Running {
		t.Fatalf("%+v, %v", run, err)
	}
}

func TestFollowRules(t *testing.T) {
	h := newHarness(t, nil)
	h.setConfig(func(c *config.Config) { c.MaxRunning = 0 })
	h.start("queued", "task")
	if _, err := h.engine.Follow(context.Background(), "queued", "more", false); err == nil {
		t.Fatal("follow before any session should fail")
	}

	h.setConfig(func(c *config.Config) { c.MaxRunning = 5 })
	h.start("running", "task")
	if _, err := h.engine.Follow(context.Background(), "running", "more", false); err == nil {
		t.Fatal("follow of a running run should fail")
	}

	h.agentReports("running", "blocked")
	h.reconcile()
	run, err := h.engine.Follow(context.Background(), "running", "first", true)
	if err != nil || run.Status != state.Queued {
		t.Fatalf("%+v, %v", run, err)
	}
	queuedAt := run.QueuedAt
	h.clock.Advance(time.Second)
	run, err = h.engine.Follow(context.Background(), "running", "second", true)
	if err != nil || run.Status != state.Queued || len(run.FollowUps) != 2 || !run.QueuedAt.Equal(queuedAt) {
		t.Fatalf("second follow-up on a queued run: %+v, %v", run, err)
	}
}

func TestSendRefusesBlockedAgentUnlessForced(t *testing.T) {
	h := newHarness(t, map[config.HookName]string{config.SendAgent: `printf '%s' {message} > "$MUSTER_DIR/sent"`})
	h.start("r", "task")
	h.agentReports("r", "blocked")
	if err := h.engine.Send(context.Background(), "r", "yes", false); !errors.Is(err, ErrAgentBlocked) {
		t.Fatalf("got %v", err)
	}
	if err := h.engine.Send(context.Background(), "r", "it's ok; go", true); err != nil {
		t.Fatal(err)
	}
	if sent, _ := os.ReadFile(h.dir.Root + "/sent"); string(sent) != "it's ok; go" {
		t.Fatalf("sent %q", sent)
	}
	h.agentReports("r", "idle")
	if err := h.engine.Send(context.Background(), "r", "next", false); err != nil {
		t.Fatal(err)
	}
}

func TestReadPassesLines(t *testing.T) {
	h := newHarness(t, map[config.HookName]string{config.ReadAgent: `echo {agent} "$MUSTER_LINES"`})
	h.start("r", "task")
	output, err := h.engine.Read(context.Background(), "r", 12)
	if err != nil || output != "agent-r-1 12\n" {
		t.Fatalf("%q, %v", output, err)
	}
}
