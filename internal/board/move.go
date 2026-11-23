// Package board is the terminal kanban view of muster runs: one column per
// status, where moving a card to another column runs the muster command that
// makes that transition.
package board

import (
	"fmt"
	"slices"

	"github.com/glundgren93/muster/internal/state"
)

type column int

const (
	colQueued column = iota
	colRunning
	colWaiting
	colDone
	colFailed
	columnCount
)

var columnTitles = [columnCount]string{"Queued", "Running", "Waiting", "Done", "Failed"}

func columnOf(status state.Status) column {
	switch status {
	case state.Running:
		return colRunning
	case state.Waiting:
		return colWaiting
	case state.Done:
		return colDone
	case state.Failed:
		return colFailed
	default: // queued, starting
		return colQueued
	}
}

type moveKind int

const (
	moveNone moveKind = iota
	moveDone
	moveRetry
	moveFollow
	moveDispatch
)

// move is what dropping a run on a column does: a command to run, or the
// reason the drop is refused.
type move struct {
	kind   moveKind
	queue  bool
	reason string
}

// planMove maps a drop of run on column to to the muster command that makes
// that transition. Columns a run only reaches on its own (waiting, failed)
// refuse drops.
func planMove(run state.Run, to column) move {
	from := columnOf(run.Status)
	if from == to {
		return move{}
	}
	if run.Status == state.Starting {
		return move{reason: fmt.Sprintf("%s is starting; move it once it has started", run.ID)}
	}
	switch to {
	case colDone:
		return move{kind: moveDone}
	case colWaiting:
		return move{reason: "a run waits when its agent stops; it can't be moved there"}
	case colFailed:
		return move{reason: "a run fails on its own; drag it to Done to close it"}
	}
	switch run.Status {
	case state.Failed:
		return move{kind: moveRetry}
	case state.Waiting, state.Done:
		if run.Sessions == 0 {
			return move{reason: fmt.Sprintf("%s never had an agent session to follow up", run.ID)}
		}
		return move{kind: moveFollow, queue: to == colQueued}
	case state.Queued:
		return move{kind: moveDispatch}
	case state.Running:
		return move{reason: fmt.Sprintf("%s is already running", run.ID)}
	}
	return move{reason: fmt.Sprintf("%s can't move from %s to %s", run.ID, columnTitles[from], columnTitles[to])}
}

// describe says in a few words what a drop does, for the drop target.
func (m move) describe() string {
	switch m.kind {
	case moveDone:
		return "mark done"
	case moveRetry:
		return "retry"
	case moveFollow:
		if m.queue {
			return "queue a follow-up"
		}
		return "follow up"
	case moveDispatch:
		return "start now"
	}
	return m.reason
}

// nextColumn finds the nearest column in direction step (-1 or +1) that
// accepts run, so a keyboard move skips columns a run can't be dropped on.
func nextColumn(run state.Run, step int) (column, move, bool) {
	var refused move
	for c := int(columnOf(run.Status)) + step; c >= 0 && c < int(columnCount); c += step {
		planned := planMove(run, column(c))
		if planned.kind != moveNone {
			return column(c), planned, true
		}
		if refused.reason == "" {
			refused = planned
		}
	}
	return 0, refused, false
}

// sortColumn orders queued runs by when they will start and every other
// column by most recent change first.
func sortColumn(c column, runs []state.Run) {
	slices.SortStableFunc(runs, func(a, b state.Run) int {
		if c == colQueued {
			return a.QueuedAt.Compare(b.QueuedAt)
		}
		return b.UpdatedAt.Compare(a.UpdatedAt)
	})
}
