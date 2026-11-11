package state

import (
	"fmt"
	"strings"
	"time"
)

type Status string

const (
	Queued   Status = "queued"
	Starting Status = "starting"
	Running  Status = "running"
	Waiting  Status = "waiting"
	Done     Status = "done"
	Failed   Status = "failed"
)

type AgentState string

const (
	AgentWorking AgentState = "working"
	AgentBlocked AgentState = "blocked"
	AgentIdle    AgentState = "idle"
	AgentDone    AgentState = "done"
	AgentError   AgentState = "error"
	AgentGone    AgentState = "gone"
	AgentUnknown AgentState = "unknown"
)

var agentStates = []AgentState{AgentWorking, AgentBlocked, AgentIdle, AgentDone, AgentError, AgentGone, AgentUnknown}

func ParseAgentState(value string) (AgentState, bool) {
	for _, known := range agentStates {
		if string(known) == value {
			return known, true
		}
	}
	return "", false
}

type FollowUp struct {
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
	SentAt    time.Time `json:"sent_at,omitzero"`
}

type Run struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	Prompt       string     `json:"prompt"`
	Source       string     `json:"source"`
	Model        string     `json:"model"`
	Workspace    string     `json:"workspace"`
	Agent        string     `json:"agent"`
	Status       Status     `json:"status"`
	AgentState   AgentState `json:"agent_state"`
	AgentStateAt time.Time  `json:"agent_state_at,omitzero"`
	SeenWorking  bool       `json:"seen_working"`
	Error        string     `json:"error"`
	FollowUps    []FollowUp `json:"follow_ups"`
	Sessions     int        `json:"sessions"`
	CreatedAt    time.Time  `json:"created_at,omitzero"`
	QueuedAt     time.Time  `json:"queued_at,omitzero"`
	StartingAt   time.Time  `json:"starting_at,omitzero"`
	StartedAt    time.Time  `json:"started_at,omitzero"`
	UpdatedAt    time.Time  `json:"updated_at,omitzero"`
	FinishedAt   time.Time  `json:"finished_at,omitzero"`
}

const FileVersion = 1

type File struct {
	Version int   `json:"version"`
	Runs    []Run `json:"runs"`
}

func (f *File) Find(id string) *Run {
	for i := range f.Runs {
		if f.Runs[i].ID == id {
			return &f.Runs[i]
		}
	}
	return nil
}

func (f *File) FindSource(source string) *Run {
	for i := range f.Runs {
		if f.Runs[i].Source == source {
			return &f.Runs[i]
		}
	}
	return nil
}

func (f *File) Resolve(ref string) (*Run, error) {
	if run := f.Find(ref); run != nil {
		return run, nil
	}
	var matches []*Run
	for i := range f.Runs {
		if ref != "" && strings.HasPrefix(f.Runs[i].ID, ref) {
			matches = append(matches, &f.Runs[i])
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no run matches %q", ref)
	case 1:
		return matches[0], nil
	default:
		ids := make([]string, len(matches))
		for i, run := range matches {
			ids[i] = run.ID
		}
		return nil, fmt.Errorf("%q matches several runs: %s", ref, strings.Join(ids, ", "))
	}
}

func (f *File) Remove(id string) {
	for i := range f.Runs {
		if f.Runs[i].ID == id {
			f.Runs = append(f.Runs[:i], f.Runs[i+1:]...)
			return
		}
	}
}
