package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/glundgren93/muster/internal/state"
)

func TestAnAgentThatNeverWorkedIsToldApartFromOneThatFinished(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	runs := []state.Run{
		{ID: "never", Status: state.Waiting, AgentState: state.AgentIdle, UpdatedAt: now},
		{ID: "finished", Status: state.Waiting, AgentState: state.AgentIdle, SeenWorking: true, UpdatedAt: now},
		{ID: "starting-up", Status: state.Running, AgentState: state.AgentIdle, UpdatedAt: now},
	}
	var table strings.Builder
	if err := writeRunTable(&table, runs, now); err != nil {
		t.Fatal(err)
	}
	flagged := map[string]bool{}
	for line := range strings.Lines(table.String()) {
		flagged[strings.Fields(line)[0]] = strings.Contains(line, "never worked")
	}
	if !flagged["never"] || flagged["finished"] || flagged["starting-up"] {
		t.Fatalf("flagged %v in:\n%s", flagged, table.String())
	}
}
