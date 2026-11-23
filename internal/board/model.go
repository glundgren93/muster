package board

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/glundgren93/muster/internal/engine"
	"github.com/glundgren93/muster/internal/state"
)

// Engine is the part of the muster engine the board drives.
type Engine interface {
	Runs() ([]state.Run, error)
	Reconcile(ctx context.Context) ([]engine.Transition, error)
	Pass(ctx context.Context) ([]engine.Transition, error)
	Dispatch(ctx context.Context) ([]engine.Transition, error)
	Start(ctx context.Context, request engine.StartRequest) (engine.StartResult, error)
	Done(ctx context.Context, ref string) (state.Run, error)
	Retry(ctx context.Context, ref string) (state.Run, error)
	Follow(ctx context.Context, ref, message string, queue bool) (state.Run, error)
	Remove(ref string, force bool) (state.Run, error)
	Read(ctx context.Context, ref string, lines int) (string, error)
}

type Options struct {
	// Interval between reconcile passes, which ask each agent for its state.
	Interval time.Duration
	// Watch runs full passes, which also start queued runs, instead of reconciles.
	Watch bool
	// Describe renders a run in full for the details pane.
	Describe func(state.Run) string
	// Age renders how long ago a time was, as in muster status.
	Age func(at, now time.Time) string
	Now func() time.Time
}

type mode int

const (
	modeBoard mode = iota
	modeFollow
	modeStart
	modeConfirmRemove
	modePager
)

type drag struct {
	id    string
	from  column
	over  column
	x, y  int
	moved bool
}

type Model struct {
	ctx  context.Context
	eng  Engine
	opts Options

	runs   []state.Run
	cols   [columnCount][]state.Run
	col    column
	row    [columnCount]int
	offset [columnCount]int
	selIDs [columnCount]string

	width, height int
	now           time.Time

	mode    mode
	input   textinput.Model
	target  state.Run
	queue   bool
	pager   viewport.Model
	heading string

	drag *drag

	flash       string
	flashErr    bool
	busy        string
	passing     bool
	lastPass    time.Time
	loadedOnce  bool
	loadFailure string
}

func New(ctx context.Context, eng Engine, opts Options) *Model {
	if opts.Interval <= 0 {
		opts.Interval = 10 * time.Second
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	input := textinput.New()
	input.CharLimit = 0
	return &Model{ctx: ctx, eng: eng, opts: opts, input: input, now: opts.Now()}
}

// Run shows the board until the user quits or ctx ends.
func Run(ctx context.Context, eng Engine, opts Options) error {
	program := tea.NewProgram(New(ctx, eng, opts), tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithContext(ctx))
	_, err := program.Run()
	if ctx.Err() != nil {
		return nil
	}
	return err
}

type (
	tickMsg   time.Time
	loadedMsg struct {
		runs []state.Run
		err  error
	}
	passedMsg struct{ err error }
	actedMsg  struct {
		follow string
		note   string
		err    error
	}
	readMsg struct {
		id     string
		output string
		err    error
	}
)

const refreshEvery = time.Second

func tick() tea.Cmd {
	return tea.Tick(refreshEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.load(), m.pass(), tick())
}

func (m *Model) load() tea.Cmd {
	eng := m.eng
	return func() tea.Msg {
		runs, err := eng.Runs()
		return loadedMsg{runs: runs, err: err}
	}
}

// pass asks the agents for their state, so cards move as agents stop.
func (m *Model) pass() tea.Cmd {
	if m.passing {
		return nil
	}
	m.passing = true
	m.lastPass = m.opts.Now()
	ctx, eng, watch := m.ctx, m.eng, m.opts.Watch
	return func() tea.Msg {
		var err error
		if watch {
			_, err = eng.Pass(ctx)
		} else {
			_, err = eng.Reconcile(ctx)
		}
		return passedMsg{err: err}
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.pager.Width, m.pager.Height = msg.Width, max(1, msg.Height-2)
		m.clamp()
		return m, nil
	case tickMsg:
		m.now = time.Time(msg)
		cmds := []tea.Cmd{tick(), m.load()}
		if m.now.Sub(m.lastPass) >= m.opts.Interval {
			cmds = append(cmds, m.pass())
		}
		return m, tea.Batch(cmds...)
	case loadedMsg:
		if msg.err != nil {
			m.loadFailure = msg.err.Error()
			return m, nil
		}
		m.loadFailure = ""
		m.setRuns(msg.runs, "")
		return m, nil
	case passedMsg:
		m.passing = false
		if msg.err != nil && m.ctx.Err() == nil {
			m.setFlash(msg.err.Error(), true)
		}
		return m, m.load()
	case actedMsg:
		m.busy = ""
		if msg.err != nil {
			m.setFlash(msg.err.Error(), true)
		} else {
			m.setFlash(msg.note, false)
		}
		eng := m.eng
		follow := msg.follow
		return m, func() tea.Msg {
			runs, err := eng.Runs()
			if err != nil {
				return loadedMsg{err: err}
			}
			return followMsg{runs: runs, id: follow}
		}
	case followMsg:
		m.setRuns(msg.runs, msg.id)
		return m, nil
	case readMsg:
		m.busy = ""
		if msg.err != nil {
			m.setFlash(msg.err.Error(), true)
			return m, nil
		}
		m.openPager("output of "+msg.id, strings.TrimRight(msg.output, "\n"), true)
		return m, nil
	case tea.KeyMsg:
		return m.key(msg)
	case tea.MouseMsg:
		return m.mouse(msg)
	}
	return m, nil
}

type followMsg struct {
	runs []state.Run
	id   string
}

// setRuns rebuilds the columns, keeping each column's selected card. With
// follow set, the cursor moves to that run wherever it now is.
func (m *Model) setRuns(runs []state.Run, follow string) {
	m.loadedOnce = true
	m.runs = runs
	var cols [columnCount][]state.Run
	for _, run := range runs {
		c := columnOf(run.Status)
		cols[c] = append(cols[c], run)
	}
	for c := range cols {
		sortColumn(column(c), cols[c])
		if i := indexOf(cols[c], m.selIDs[c]); i >= 0 {
			m.row[c] = i
		}
		if follow != "" {
			if i := indexOf(cols[c], follow); i >= 0 {
				m.col, m.row[c] = column(c), i
			}
		}
	}
	m.cols = cols
	m.clamp()
}

func indexOf(runs []state.Run, id string) int {
	if id == "" {
		return -1
	}
	return slices.IndexFunc(runs, func(run state.Run) bool { return run.ID == id })
}

// clamp keeps every cursor on a card and every selected card on screen.
func (m *Model) clamp() {
	visible := m.visibleCards()
	for c := range m.cols {
		n := len(m.cols[c])
		m.row[c] = min(max(m.row[c], 0), max(n-1, 0))
		if m.row[c] < m.offset[c] {
			m.offset[c] = m.row[c]
		}
		if m.row[c] >= m.offset[c]+visible {
			m.offset[c] = m.row[c] - visible + 1
		}
		m.offset[c] = min(max(m.offset[c], 0), max(n-visible, 0))
		m.selIDs[c] = ""
		if n > 0 {
			m.selIDs[c] = m.cols[c][m.row[c]].ID
		}
	}
}

func (m *Model) selected() (state.Run, bool) {
	cards := m.cols[m.col]
	if len(cards) == 0 {
		return state.Run{}, false
	}
	return cards[m.row[m.col]], true
}

func (m *Model) setFlash(text string, isErr bool) {
	m.flash = strings.ReplaceAll(strings.TrimSpace(text), "\n", " | ")
	m.flashErr = isErr
}

func (m *Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	switch m.mode {
	case modePager:
		switch msg.String() {
		case "esc", "q", "enter":
			m.mode = modeBoard
			return m, nil
		}
		var cmd tea.Cmd
		m.pager, cmd = m.pager.Update(msg)
		return m, cmd
	case modeFollow, modeStart:
		switch msg.String() {
		case "esc":
			m.mode = modeBoard
			m.input.Blur()
			return m, nil
		case "enter":
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}
			m.input.Blur()
			if m.mode == modeStart {
				m.mode = modeBoard
				return m, m.start(text)
			}
			m.mode = modeBoard
			return m, m.followUp(m.target, text, m.queue)
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	case modeConfirmRemove:
		m.mode = modeBoard
		if msg.String() == "y" || msg.String() == "Y" {
			return m, m.remove(m.target)
		}
		m.setFlash("kept "+m.target.ID, false)
		return m, nil
	}

	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "esc":
		m.flash = ""
	case "left", "h":
		m.focusColumn(-1)
	case "right", "l":
		m.focusColumn(+1)
	case "up", "k":
		m.row[m.col]--
		m.clamp()
	case "down", "j":
		m.row[m.col]++
		m.clamp()
	case "pgup":
		m.row[m.col] -= max(1, m.visibleCards())
		m.clamp()
	case "pgdown":
		m.row[m.col] += max(1, m.visibleCards())
		m.clamp()
	case "home", "g":
		m.row[m.col] = 0
		m.clamp()
	case "end", "G":
		m.row[m.col] = len(m.cols[m.col]) - 1
		m.clamp()
	case "shift+left", "H", "<":
		return m, m.shift(-1)
	case "shift+right", "L", ">":
		return m, m.shift(+1)
	case "enter", " ":
		if run, ok := m.selected(); ok {
			m.openPager(run.ID, m.opts.Describe(run), false)
		}
	case "o":
		if run, ok := m.selected(); ok {
			return m, m.read(run)
		}
	case "f":
		if run, ok := m.selected(); ok {
			return m, m.moveTo(run, colRunning)
		}
	case "d":
		if run, ok := m.selected(); ok {
			return m, m.moveTo(run, colDone)
		}
	case "x", "delete":
		if run, ok := m.selected(); ok {
			if run.Status == state.Starting || run.Status == state.Running {
				m.setFlash(fmt.Sprintf("%s is %s; stop it or use muster rm --force", run.ID, run.Status), true)
				return m, nil
			}
			m.target = run
			m.mode = modeConfirmRemove
		}
	case "n":
		m.mode = modeStart
		return m, m.ask("new task: ", "what should the agent do?")
	case "r":
		return m, m.pass()
	}
	return m, nil
}

func (m *Model) ask(prompt, placeholder string) tea.Cmd {
	m.input.Reset()
	m.input.Prompt = prompt
	m.input.Placeholder = placeholder
	m.input.Width = max(10, m.width-len(prompt)-2)
	return m.input.Focus()
}

func (m *Model) focusColumn(step int) {
	m.col = column(min(max(int(m.col)+step, 0), int(columnCount)-1))
}

// shift moves the selected card to the nearest column in direction step that
// accepts it.
func (m *Model) shift(step int) tea.Cmd {
	run, ok := m.selected()
	if !ok {
		return nil
	}
	to, planned, found := nextColumn(run, step)
	if !found {
		if planned.reason != "" {
			m.setFlash(planned.reason, true)
		}
		return nil
	}
	return m.moveTo(run, to)
}

// moveTo runs the command that moves run to column to, asking for the
// follow-up text first when the move needs one.
func (m *Model) moveTo(run state.Run, to column) tea.Cmd {
	planned := planMove(run, to)
	if m.busy != "" && planned.kind != moveNone {
		m.setFlash("still busy: "+m.busy, true)
		return nil
	}
	ctx, eng, id := m.ctx, m.eng, run.ID
	switch planned.kind {
	case moveNone:
		if planned.reason != "" {
			m.setFlash(planned.reason, true)
		}
		return nil
	case moveFollow:
		m.target, m.queue = run, planned.queue
		m.mode = modeFollow
		return m.ask("follow-up for "+run.ID+": ", "what should the agent do next?")
	case moveDone:
		return m.act("marking "+id+" done", id, func() (string, error) {
			_, err := eng.Done(ctx, id)
			return id + " is done", err
		})
	case moveRetry:
		return m.act("retrying "+id, id, func() (string, error) {
			run, err := eng.Retry(ctx, id)
			return outcome("retried "+id, run, err)
		})
	case moveDispatch:
		return m.act("starting queued runs", id, func() (string, error) {
			transitions, err := eng.Dispatch(ctx)
			for _, t := range transitions {
				if t.ID == id && t.To != state.Starting {
					return fmt.Sprintf("%s is now %s", id, t.To), err
				}
			}
			return id + " stays queued; every slot is taken", err
		})
	}
	return nil
}

func (m *Model) followUp(run state.Run, text string, queue bool) tea.Cmd {
	ctx, eng, id := m.ctx, m.eng, run.ID
	return m.act("following up on "+id, id, func() (string, error) {
		updated, err := eng.Follow(ctx, id, text, queue)
		return outcome("followed up on "+id, updated, err)
	})
}

func (m *Model) start(prompt string) tea.Cmd {
	if m.busy != "" {
		m.setFlash("still busy: "+m.busy, true)
		return nil
	}
	ctx, eng := m.ctx, m.eng
	m.busy = "starting a new task"
	return func() tea.Msg {
		result, err := eng.Start(ctx, engine.StartRequest{Prompt: prompt})
		if err != nil {
			return actedMsg{err: err}
		}
		note, err := outcome("started "+result.Run.ID, result.Run, nil)
		return actedMsg{follow: result.Run.ID, note: note, err: err}
	}
}

func (m *Model) remove(run state.Run) tea.Cmd {
	if m.busy != "" {
		m.setFlash("still busy: "+m.busy, true)
		return nil
	}
	eng, id := m.eng, run.ID
	return m.act("removing "+id, "", func() (string, error) {
		_, err := eng.Remove(id, false)
		return "removed " + id, err
	})
}

func (m *Model) read(run state.Run) tea.Cmd {
	if run.Agent == "" {
		m.setFlash(run.ID+" has no agent session to read", true)
		return nil
	}
	ctx, eng, id := m.ctx, m.eng, run.ID
	lines := max(80, m.height)
	m.busy = "reading " + id
	return func() tea.Msg {
		output, err := eng.Read(ctx, id, lines)
		return readMsg{id: id, output: output, err: err}
	}
}

// outcome reports where a run ended up after a command that may start it, as
// an error when its start failed.
func outcome(done string, run state.Run, err error) (string, error) {
	if err != nil {
		return "", err
	}
	if run.Status == state.Failed {
		return "", fmt.Errorf("%s, but it failed: %s", done, run.Error)
	}
	return fmt.Sprintf("%s; now %s", done, run.Status), nil
}

func (m *Model) act(label, follow string, do func() (string, error)) tea.Cmd {
	m.busy = label
	m.flash = ""
	return func() tea.Msg {
		note, err := do()
		return actedMsg{follow: follow, note: note, err: err}
	}
}

func (m *Model) openPager(heading, content string, bottom bool) {
	m.heading = heading
	m.pager = viewport.New(m.width, max(1, m.height-2))
	m.pager.SetContent(content)
	if bottom {
		m.pager.GotoBottom()
	}
	m.mode = modePager
}

func (m *Model) mouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.mode == modePager {
		var cmd tea.Cmd
		m.pager, cmd = m.pager.Update(msg)
		return m, cmd
	}
	if m.mode != modeBoard {
		return m, nil
	}
	event := tea.MouseEvent(msg)
	switch {
	case event.IsWheel():
		c := m.columnAt(event.X)
		m.col = c
		if event.Button == tea.MouseButtonWheelUp {
			m.row[c]--
		} else if event.Button == tea.MouseButtonWheelDown {
			m.row[c]++
		}
		m.clamp()
	case event.Action == tea.MouseActionPress && event.Button == tea.MouseButtonLeft:
		c := m.columnAt(event.X)
		m.col = c
		if i, ok := m.cardAt(c, event.Y); ok {
			m.row[c] = i
			m.clamp()
			m.drag = &drag{id: m.cols[c][i].ID, from: c, over: c, x: event.X, y: event.Y}
		}
	case event.Action == tea.MouseActionMotion && m.drag != nil:
		if event.X != m.drag.x || event.Y != m.drag.y {
			m.drag.moved = true
		}
		m.drag.over = m.columnAt(event.X)
	case event.Action == tea.MouseActionRelease && m.drag != nil:
		d := m.drag
		m.drag = nil
		d.over = m.columnAt(event.X)
		if !d.moved || d.over == d.from {
			return m, nil
		}
		if i := indexOf(m.cols[d.from], d.id); i >= 0 {
			return m, m.moveTo(m.cols[d.from][i], d.over)
		}
	}
	return m, nil
}

func (m *Model) draggedRun() (state.Run, bool) {
	if m.drag == nil || !m.drag.moved {
		return state.Run{}, false
	}
	if i := indexOf(m.cols[m.drag.from], m.drag.id); i >= 0 {
		return m.cols[m.drag.from][i], true
	}
	return state.Run{}, false
}
