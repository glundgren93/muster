package cli

import (
	"testing"

	"github.com/glundgren93/muster/internal/state"
)

func TestWaitExitCode(t *testing.T) {
	waiting := func(agent state.AgentState) state.Run {
		return state.Run{Status: state.Waiting, AgentState: agent}
	}
	cases := []struct {
		name string
		runs []state.Run
		want int
	}{
		{"finished", []state.Run{waiting(state.AgentDone), waiting(state.AgentIdle), {Status: state.Done}}, 0},
		{"closed while blocked", []state.Run{{Status: state.Done, AgentState: state.AgentBlocked}}, 0},
		{"blocked", []state.Run{waiting(state.AgentDone), waiting(state.AgentBlocked)}, exitRunBlocked},
		{"failed start", []state.Run{{Status: state.Failed}}, exitRunFailed},
		{"agent error", []state.Run{waiting(state.AgentError)}, exitRunFailed},
		{"agent gone", []state.Run{waiting(state.AgentGone)}, exitRunFailed},
		{"failure outranks blocked", []state.Run{waiting(state.AgentBlocked), waiting(state.AgentGone)}, exitRunFailed},
		{"timeout leaves a running run", []state.Run{{Status: state.Running, AgentState: state.AgentWorking}}, 0},
	}
	for _, c := range cases {
		if got := waitExitCode(c.runs); got != c.want {
			t.Errorf("%s: exit %d, want %d", c.name, got, c.want)
		}
	}
}
