package board

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/glundgren93/muster/internal/state"
)

const (
	headerLines = 2 // title bar, column headers
	footerLines = 2 // status line, help line
	cardHeight  = 5 // border, two title lines, meta line, border
)

var (
	columnColors = [columnCount]lipgloss.AdaptiveColor{
		colQueued:  {Light: "245", Dark: "247"},
		colRunning: {Light: "26", Dark: "39"},
		colWaiting: {Light: "130", Dark: "214"},
		colDone:    {Light: "28", Dark: "42"},
		colFailed:  {Light: "160", Dark: "203"},
	}
	dim        = lipgloss.AdaptiveColor{Light: "250", Dark: "240"}
	faint      = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "244", Dark: "244"})
	bold       = lipgloss.NewStyle().Bold(true)
	errorStyle = lipgloss.NewStyle().Foreground(columnColors[colFailed])
	okStyle    = lipgloss.NewStyle().Foreground(columnColors[colDone])
	dashed     = lipgloss.Border{Top: "╌", Bottom: "╌", Left: "┆", Right: "┆", TopLeft: "┌", TopRight: "┐", BottomLeft: "└", BottomRight: "┘"}
)

func (m *Model) bodyHeight() int {
	return max(0, m.height-headerLines-footerLines)
}

func (m *Model) visibleCards() int {
	return max(1, m.bodyHeight()/cardHeight)
}

func (m *Model) columnX(c column) int {
	return int(c) * m.width / int(columnCount)
}

func (m *Model) columnWidth(c column) int {
	return m.columnX(c+1) - m.columnX(c)
}

func (m *Model) columnAt(x int) column {
	for c := columnCount - 1; c > 0; c-- {
		if x >= m.columnX(c) {
			return c
		}
	}
	return colQueued
}

// cardAt finds the card drawn at screen row y in column c.
func (m *Model) cardAt(c column, y int) (int, bool) {
	y -= headerLines
	if y < 0 || y >= m.bodyHeight() {
		return 0, false
	}
	slot := y / cardHeight
	if slot >= m.visibleCards() {
		return 0, false
	}
	i := m.offset[c] + slot
	return i, i < len(m.cols[c])
}

func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if m.mode == modePager {
		top := bold.Render(m.heading)
		help := faint.Render("↑/↓ pgup/pgdn scroll · esc close")
		return lipgloss.JoinVertical(lipgloss.Left, top, m.pager.View(), help)
	}
	columns := make([]string, columnCount)
	for c := range columnCount {
		columns[c] = m.renderColumn(c)
	}
	lines := []string{
		m.renderTitle(),
		lipgloss.JoinHorizontal(lipgloss.Top, columns...),
		m.renderStatus(),
		m.renderHelp(),
	}
	return strings.Join(lines, "\n")
}

func (m *Model) renderTitle() string {
	active := 0
	for c := colQueued; c <= colWaiting; c++ {
		active += len(m.cols[c])
	}
	left := bold.Render("muster board") + faint.Render(fmt.Sprintf("  %d active · %d total", active, len(m.runs)))
	right := ""
	switch {
	case m.loadFailure != "":
		right = errorStyle.Render(m.loadFailure)
	case m.passing:
		right = faint.Render("checking agents…")
	case m.opts.Watch:
		right = faint.Render("watching")
	}
	gap := max(1, m.width-lipgloss.Width(left)-lipgloss.Width(right))
	return ansi.Truncate(left+strings.Repeat(" ", gap)+right, m.width, "…")
}

func (m *Model) renderColumn(c column) string {
	width := m.columnWidth(c)
	cardWidth := max(8, width-1)
	color := columnColors[c]
	cards := m.cols[c]

	header := fmt.Sprintf(" %s %d", strings.ToUpper(columnTitles[c]), len(cards))
	if above := m.offset[c]; above > 0 {
		header += fmt.Sprintf(" ↑%d", above)
	}
	visible := m.visibleCards()
	var body []string
	if dragged, ok := m.draggedRun(); ok && m.drag.over == c && c != m.drag.from {
		body = append(body, m.renderDropTarget(dragged, c, cardWidth))
		visible--
	}
	end := min(len(cards), m.offset[c]+visible)
	if below := len(cards) - end; below > 0 {
		header += fmt.Sprintf(" ↓%d", below)
	}
	headerStyle := lipgloss.NewStyle().Bold(true).Width(cardWidth).Foreground(color)
	if c == m.col {
		headerStyle = headerStyle.Reverse(true)
	}
	for i := m.offset[c]; i < end; i++ {
		body = append(body, m.renderCard(cards[i], c, i, cardWidth))
	}
	if len(cards) == 0 && len(body) == 0 {
		body = append(body, faint.Render(" nothing here"))
	}
	column := lipgloss.JoinVertical(lipgloss.Left, append([]string{headerStyle.Render(ansi.Truncate(header, cardWidth, "…"))}, body...)...)
	return lipgloss.NewStyle().Width(width).Height(m.bodyHeight() + 1).MaxHeight(m.bodyHeight() + 1).Render(column)
}

func (m *Model) renderCard(run state.Run, c column, i, width int) string {
	inner := max(1, width-4)
	selected := c == m.col && i == m.row[c]
	lifted := m.drag != nil && m.drag.moved && m.drag.id == run.ID

	title := strings.Split(ansi.Wrap(oneLine(run.Title), inner, ""), "\n")
	if len(title) > 2 {
		title = []string{title[0], ansi.Truncate(title[1]+" "+title[2], inner, "…")}
	}
	for len(title) < 2 {
		title = append(title, "")
	}
	meta := m.meta(run, inner)

	titleStyle := lipgloss.NewStyle()
	if selected {
		titleStyle = titleStyle.Bold(true)
	}
	content := strings.Join([]string{titleStyle.Render(title[0]), titleStyle.Render(title[1]), meta}, "\n")

	style := lipgloss.NewStyle().Width(width-2).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(dim)
	switch {
	case lifted:
		style = style.Border(dashed).BorderForeground(columnColors[c]).Faint(true)
	case selected:
		style = style.Border(lipgloss.ThickBorder()).BorderForeground(columnColors[c])
	}
	return style.Render(content)
}

func (m *Model) meta(run state.Run, width int) string {
	parts := []string{}
	if run.Status == state.Starting {
		parts = append(parts, "starting")
	}
	if agent := agentSummary(run); agent != "" {
		parts = append(parts, agent)
	}
	if m.opts.Age != nil {
		parts = append(parts, m.opts.Age(run.UpdatedAt, m.now))
	}
	parts = append(parts, run.ID)
	text := strings.Join(parts, " · ")
	if run.Status == state.Failed && run.Error != "" {
		text = oneLine(run.Error)
	}
	style := faint
	if run.Status == state.Failed {
		style = errorStyle
	}
	if run.AgentState == state.AgentBlocked {
		style = lipgloss.NewStyle().Foreground(columnColors[colWaiting]).Bold(true)
	}
	return style.Render(ansi.Truncate(text, width, "…"))
}

func agentSummary(run state.Run) string {
	if run.Status == state.Waiting && run.AgentState == state.AgentIdle && !run.SeenWorking {
		return "idle, never worked"
	}
	return string(run.AgentState)
}

func (m *Model) renderDropTarget(run state.Run, c column, width int) string {
	planned := planMove(run, c)
	text := "drop to " + planned.describe()
	color := columnColors[c]
	if planned.kind == moveNone {
		text = "✗ " + planned.reason
		color = columnColors[colFailed]
	}
	lines := strings.Split(ansi.Wrap(text, max(1, width-4), ""), "\n")
	for len(lines) < 3 {
		lines = append(lines, "")
	}
	return lipgloss.NewStyle().Width(width-2).Padding(0, 1).Border(dashed).BorderForeground(color).Foreground(color).
		Render(strings.Join(lines[:3], "\n"))
}

func (m *Model) renderStatus() string {
	var line string
	switch m.mode {
	case modeFollow, modeStart:
		line = m.input.View()
	case modeConfirmRemove:
		line = errorStyle.Render(fmt.Sprintf("remove %s? the record and its prompt files go; the workspace and agent stay. y/n", m.target.ID))
	default:
		switch {
		case m.busy != "":
			line = faint.Render(m.busy + "…")
		case m.drag != nil && m.drag.moved:
			if run, ok := m.draggedRun(); ok {
				line = faint.Render(fmt.Sprintf("moving %s to %s: %s", run.ID, columnTitles[m.drag.over], planMove(run, m.drag.over).describe()))
			}
		case m.flash != "" && m.flashErr:
			line = errorStyle.Render(m.flash)
		case m.flash != "":
			line = okStyle.Render(m.flash)
		case !m.loadedOnce:
			line = faint.Render("loading…")
		default:
			if run, ok := m.selected(); ok {
				line = faint.Render(run.ID + "  " + oneLine(run.Title))
			}
		}
	}
	return ansi.Truncate(line, m.width, "…")
}

func (m *Model) renderHelp() string {
	var help string
	switch m.mode {
	case modeFollow, modeStart:
		help = "enter send · esc cancel"
	case modeConfirmRemove:
		help = "y remove · any other key keeps it"
	default:
		help = "←→↑↓ select · drag a card or H/L to move it · enter details · o output · f follow up · d done · x remove · n new · r refresh · q quit"
	}
	return faint.Render(ansi.Truncate(help, m.width, "…"))
}

func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
