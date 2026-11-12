package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/glundgren93/muster/internal/engine"
	"github.com/glundgren93/muster/internal/state"
)

type format string

const (
	formatTable format = "table"
	formatJSON  format = "json"
)

func addOutputFlag(command *cobra.Command, target *format) {
	*target = formatTable
	command.Flags().VarP((*formatValue)(target), "output", "o", "output format: table or json")
}

type formatValue format

func (f *formatValue) String() string { return string(*f) }

func (f *formatValue) Type() string { return "format" }

func (f *formatValue) Set(value string) error {
	switch format(value) {
	case formatTable, formatJSON:
		*f = formatValue(value)
		return nil
	}
	return fmt.Errorf("%q is not table or json", value)
}

var statusOrder = []state.Status{state.Waiting, state.Failed, state.Running, state.Starting, state.Queued, state.Done}

func sortRuns(runs []state.Run) {
	slices.SortStableFunc(runs, func(a, b state.Run) int {
		if order := slices.Index(statusOrder, a.Status) - slices.Index(statusOrder, b.Status); order != 0 {
			return order
		}
		return a.CreatedAt.Compare(b.CreatedAt)
	})
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func writeRunTable(w io.Writer, runs []state.Run, now time.Time) error {
	if len(runs) == 0 {
		_, err := fmt.Fprintln(w, "no runs")
		return err
	}
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tSTATUS\tAGENT\tUPDATED\tTITLE")
	for _, run := range runs {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n", run.ID, run.Status, orDash(agentSummary(run)), age(run.UpdatedAt, now), run.Title)
	}
	return table.Flush()
}

func writeRun(w io.Writer, run state.Run, now time.Time) error {
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	field := func(name, value string) {
		if value != "" {
			fmt.Fprintf(table, "%s:\t%s\n", name, value)
		}
	}
	timestamp := func(name string, at time.Time) {
		if !at.IsZero() {
			field(name, fmt.Sprintf("%s (%s ago)", at.Local().Format(time.DateTime), age(at, now)))
		}
	}
	field("id", run.ID)
	field("title", run.Title)
	field("status", string(run.Status))
	if run.AgentState != "" {
		field("agent state", fmt.Sprintf("%s (seen %s ago)", agentSummary(run), age(run.AgentStateAt, now)))
	}
	field("error", run.Error)
	field("source", run.Source)
	field("model", run.Model)
	field("workspace", run.Workspace)
	field("agent", run.Agent)
	field("sessions", fmt.Sprint(run.Sessions))
	timestamp("created", run.CreatedAt)
	timestamp("queued", run.QueuedAt)
	timestamp("started", run.StartedAt)
	timestamp("updated", run.UpdatedAt)
	timestamp("finished", run.FinishedAt)
	if err := table.Flush(); err != nil {
		return err
	}
	if len(run.FollowUps) > 0 {
		fmt.Fprintln(w, "\nfollow-ups:")
		for _, followUp := range run.FollowUps {
			mark := "pending"
			if !followUp.SentAt.IsZero() {
				mark = "sent " + followUp.SentAt.Local().Format(time.DateTime)
			}
			fmt.Fprintf(w, "  [%s] %s\n", mark, indentContinuation(followUp.Text, "    "))
		}
	}
	_, err := fmt.Fprintf(w, "\nprompt:\n%s\n", indent(strings.TrimRight(run.Prompt, "\n"), "  "))
	return err
}

func agentSummary(run state.Run) string {
	if run.Status == state.Waiting && run.AgentState == state.AgentIdle && !run.SeenWorking {
		return string(run.AgentState) + ", never worked"
	}
	return string(run.AgentState)
}

func writeTransitions(w io.Writer, transitions []engine.Transition) error {
	for _, transition := range transitions {
		if _, err := fmt.Fprintln(w, transition); err != nil {
			return err
		}
	}
	return nil
}

func age(at, now time.Time) string {
	if at.IsZero() {
		return "-"
	}
	elapsed := now.Sub(at)
	switch {
	case elapsed < time.Minute:
		return fmt.Sprintf("%ds", max(0, int(elapsed.Seconds())))
	case elapsed < time.Hour:
		return fmt.Sprintf("%dm", int(elapsed.Minutes()))
	case elapsed < 48*time.Hour:
		return fmt.Sprintf("%dh", int(elapsed.Hours()))
	default:
		return fmt.Sprintf("%dd", int(elapsed.Hours()/24))
	}
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func indent(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

func indentContinuation(text, prefix string) string {
	first, rest, found := strings.Cut(text, "\n")
	if !found {
		return text
	}
	return first + "\n" + indent(rest, prefix)
}
