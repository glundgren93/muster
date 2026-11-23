package board

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/glundgren93/muster/internal/engine"
	"github.com/glundgren93/muster/internal/state"
)

func TestPlanMove(t *testing.T) {
	run := func(status state.Status, sessions int) state.Run {
		return state.Run{ID: "r", Status: status, Sessions: sessions}
	}
	cases := []struct {
		name  string
		run   state.Run
		to    column
		kind  moveKind
		queue bool
	}{
		{"waiting to done", run(state.Waiting, 1), colDone, moveDone, false},
		{"queued to done", run(state.Queued, 0), colDone, moveDone, false},
		{"running to done", run(state.Running, 1), colDone, moveDone, false},
		{"waiting to running follows up", run(state.Waiting, 1), colRunning, moveFollow, false},
		{"done to queued queues a follow-up", run(state.Done, 1), colQueued, moveFollow, true},
		{"failed to running retries", run(state.Failed, 0), colRunning, moveRetry, false},
		{"failed to queued retries", run(state.Failed, 1), colQueued, moveRetry, false},
		{"queued to running dispatches", run(state.Queued, 0), colRunning, moveDispatch, false},
		{"done without a session", run(state.Done, 0), colRunning, moveNone, false},
		{"running to queued", run(state.Running, 1), colQueued, moveNone, false},
		{"anything to waiting", run(state.Running, 1), colWaiting, moveNone, false},
		{"anything to failed", run(state.Waiting, 1), colFailed, moveNone, false},
		{"starting", run(state.Starting, 0), colDone, moveNone, false},
		{"same column", run(state.Starting, 0), colQueued, moveNone, false},
	}
	for _, c := range cases {
		got := planMove(c.run, c.to)
		if got.kind != c.kind || got.queue != c.queue {
			t.Errorf("%s: got %+v, want kind %d queue %v", c.name, got, c.kind, c.queue)
		}
		if got.kind == moveNone && c.name != "same column" && got.reason == "" {
			t.Errorf("%s: refused without a reason", c.name)
		}
	}
}

func TestNextColumnSkipsRefusingColumns(t *testing.T) {
	done := state.Run{ID: "r", Status: state.Done, Sessions: 1}
	if to, planned, ok := nextColumn(done, -1); !ok || to != colRunning || planned.kind != moveFollow {
		t.Errorf("done moving left: got %v %+v %v, want running follow-up", to, planned, ok)
	}
	waiting := state.Run{ID: "r", Status: state.Waiting, Sessions: 1}
	if to, _, ok := nextColumn(waiting, +1); !ok || to != colDone {
		t.Errorf("waiting moving right: got %v %v, want done", to, ok)
	}
	failed := state.Run{ID: "r", Status: state.Failed}
	if _, _, ok := nextColumn(failed, +1); ok {
		t.Errorf("failed moving right: nothing is right of failed")
	}
}

type fakeEngine struct {
	runs  []state.Run
	calls []string
}

func (f *fakeEngine) Runs() ([]state.Run, error) { return append([]state.Run(nil), f.runs...), nil }
func (f *fakeEngine) Reconcile(context.Context) ([]engine.Transition, error) {
	return nil, nil
}
func (f *fakeEngine) Pass(context.Context) ([]engine.Transition, error) { return nil, nil }
func (f *fakeEngine) Dispatch(context.Context) ([]engine.Transition, error) {
	f.calls = append(f.calls, "dispatch")
	return nil, nil
}
func (f *fakeEngine) Start(_ context.Context, request engine.StartRequest) (engine.StartResult, error) {
	f.calls = append(f.calls, "start "+request.Prompt)
	return engine.StartResult{Run: state.Run{ID: "new", Status: state.Queued}, Created: true}, nil
}
func (f *fakeEngine) set(id string, status state.Status) state.Run {
	for i := range f.runs {
		if f.runs[i].ID == id {
			f.runs[i].Status = status
			return f.runs[i]
		}
	}
	return state.Run{}
}
func (f *fakeEngine) Done(_ context.Context, ref string) (state.Run, error) {
	f.calls = append(f.calls, "done "+ref)
	return f.set(ref, state.Done), nil
}
func (f *fakeEngine) Retry(_ context.Context, ref string) (state.Run, error) {
	f.calls = append(f.calls, "retry "+ref)
	return f.set(ref, state.Running), nil
}
func (f *fakeEngine) Follow(_ context.Context, ref, message string, queue bool) (state.Run, error) {
	f.calls = append(f.calls, fmt.Sprintf("follow %s %q queue=%v", ref, message, queue))
	return f.set(ref, state.Running), nil
}
func (f *fakeEngine) Remove(ref string, force bool) (state.Run, error) {
	f.calls = append(f.calls, "rm "+ref)
	return state.Run{ID: ref}, nil
}
func (f *fakeEngine) Read(_ context.Context, ref string, lines int) (string, error) {
	return "agent output", nil
}

func newTestBoard(t *testing.T) (*Model, *fakeEngine) {
	t.Helper()
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	eng := &fakeEngine{runs: []state.Run{
		{ID: "wait-a", Title: "Waiting task A", Status: state.Waiting, Sessions: 1, AgentState: state.AgentDone, UpdatedAt: now.Add(-time.Hour)},
		{ID: "wait-b", Title: "Waiting task B", Status: state.Waiting, Sessions: 1, AgentState: state.AgentGone, UpdatedAt: now.Add(-2 * time.Hour)},
		{ID: "run-a", Title: "Running task", Status: state.Running, Sessions: 1, AgentState: state.AgentWorking, UpdatedAt: now},
		{ID: "fail-a", Title: "Failed task", Status: state.Failed, Error: "start_agent: boom", UpdatedAt: now},
	}}
	m := New(context.Background(), eng, Options{
		Describe: func(run state.Run) string { return "details of " + run.ID },
		Age:      func(at, now time.Time) string { return "1h" },
		Now:      func() time.Time { return now },
	})
	m.input.Cursor.SetMode(cursor.CursorStatic) // a blinking cursor would sleep in drain
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.setRuns(eng.runs, "")
	return m, eng
}

// run feeds msg to the board and runs the commands it returns until none are
// left, the way the tea runtime would.
func run(m *Model, msg tea.Msg) {
	queue := []tea.Msg{msg}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		_, cmd := m.Update(next)
		queue = append(queue, drain(cmd)...)
	}
}

func drain(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, drain(c)...)
		}
		return out
	case nil:
		return nil
	default:
		if _, ok := msg.(tickMsg); ok {
			return nil
		}
		return []tea.Msg{msg}
	}
}

func mouse(action tea.MouseAction, x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: action, Button: tea.MouseButtonLeft}
}

func TestDragWaitingCardToDoneMarksItDone(t *testing.T) {
	m, eng := newTestBoard(t)
	x := m.columnX(colWaiting) + 3
	y := headerLines + 1 // the first card
	run(m, mouse(tea.MouseActionPress, x, y))
	run(m, mouse(tea.MouseActionMotion, m.columnX(colDone)+3, y))
	if view := m.View(); !strings.Contains(view, "drop to mark done") {
		t.Fatalf("no drop target while dragging:\n%s", view)
	}
	run(m, mouse(tea.MouseActionRelease, m.columnX(colDone)+3, y))
	if want := []string{"done wait-a"}; fmt.Sprint(eng.calls) != fmt.Sprint(want) {
		t.Fatalf("calls %v, want %v", eng.calls, want)
	}
	if m.col != colDone || m.cols[colDone][0].ID != "wait-a" {
		t.Errorf("cursor did not follow the card to done: col %v", m.col)
	}
}

func TestDragWaitingCardToRunningAsksForFollowUp(t *testing.T) {
	m, eng := newTestBoard(t)
	y := headerLines + cardHeight + 1 // the second card: wait-b
	run(m, mouse(tea.MouseActionPress, m.columnX(colWaiting)+3, y))
	run(m, mouse(tea.MouseActionMotion, m.columnX(colRunning)+3, y))
	run(m, mouse(tea.MouseActionRelease, m.columnX(colRunning)+3, y))
	if m.mode != modeFollow {
		t.Fatalf("mode %v, want the follow-up prompt", m.mode)
	}
	for _, r := range "add a test" {
		run(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	run(m, tea.KeyMsg{Type: tea.KeyEnter})
	if want := `[follow wait-b "add a test" queue=false]`; fmt.Sprint(eng.calls) != want {
		t.Fatalf("calls %v, want %s", eng.calls, want)
	}
}

func TestDropOnWaitingIsRefused(t *testing.T) {
	m, eng := newTestBoard(t)
	run(m, mouse(tea.MouseActionPress, m.columnX(colRunning)+3, headerLines+1))
	run(m, mouse(tea.MouseActionMotion, m.columnX(colWaiting)+3, headerLines+1))
	run(m, mouse(tea.MouseActionRelease, m.columnX(colWaiting)+3, headerLines+1))
	if len(eng.calls) != 0 || !m.flashErr {
		t.Errorf("calls %v, flash %q: want a refusal", eng.calls, m.flash)
	}
}

func TestClickWithoutDragOnlySelects(t *testing.T) {
	m, eng := newTestBoard(t)
	run(m, mouse(tea.MouseActionPress, m.columnX(colWaiting)+3, headerLines+cardHeight+1))
	run(m, mouse(tea.MouseActionRelease, m.columnX(colWaiting)+3, headerLines+cardHeight+1))
	if len(eng.calls) != 0 {
		t.Errorf("a click ran %v", eng.calls)
	}
	if got, _ := m.selected(); got.ID != "wait-b" {
		t.Errorf("selected %q, want wait-b", got.ID)
	}
}

func TestKeyboardMove(t *testing.T) {
	m, eng := newTestBoard(t)
	m.col = colFailed
	run(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'H'}})
	run(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'H'}})
	if want := "[done fail-a]"; fmt.Sprint(eng.calls) != want {
		t.Fatalf("calls %v, want %s", eng.calls, want)
	}
	// fail-a is now done, and the cursor followed it; moving left again skips
	// waiting and asks for a follow-up, which needs a session.
	if m.col != colDone {
		t.Fatalf("cursor in %v, want done", m.col)
	}
}

func TestRemoveAsksFirst(t *testing.T) {
	m, eng := newTestBoard(t)
	m.col = colWaiting
	run(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	run(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if len(eng.calls) != 0 {
		t.Fatalf("removed without a yes: %v", eng.calls)
	}
	run(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	run(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if want := "[rm wait-a]"; fmt.Sprint(eng.calls) != want {
		t.Fatalf("calls %v, want %s", eng.calls, want)
	}
}

func TestViewFitsTheTerminal(t *testing.T) {
	m, _ := newTestBoard(t)
	for _, size := range [][2]int{{120, 40}, {60, 12}, {30, 6}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := m.View()
		lines := strings.Split(view, "\n")
		if len(lines) > size[1] {
			t.Errorf("%dx%d: view has %d lines", size[0], size[1], len(lines))
		}
		for i, line := range lines {
			if w := ansi.StringWidth(line); w > size[0] {
				t.Errorf("%dx%d: line %d is %d wide", size[0], size[1], i, w)
			}
		}
	}
}
