package hook

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glundgren93/muster/internal/config"
)

func TestPlaceholderValueReachesHookIntactWithoutInjection(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "injected")
	title := `it's a "title"; touch ` + marker + ` $(touch ` + marker + `) ` + "`touch " + marker + "`"
	runner := Runner{MusterDir: dir, Bin: "muster"}
	command := config.Hook{Command: `printf '%s\n' {title} "$MUSTER_TITLE"`, Timeout: 5 * time.Second}

	output, err := runner.Run(context.Background(), config.OnChange, command, Vars{Title: title})
	if err != nil {
		t.Fatal(err)
	}
	if want := title + "\n" + title + "\n"; output != want {
		t.Fatalf("output %q, want %q", output, want)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a placeholder value ran a command")
	}
}

func TestExpandLeavesUnknownBracesAlone(t *testing.T) {
	got := Expand(`awk '{print $1}' {nope} {id} ${HOME} {}`, Vars{ID: "run-1"})
	want := `awk '{print $1}' {nope} 'run-1' ${HOME} {}`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestEmptyPlaceholderExpandsToEmptyArgument(t *testing.T) {
	runner := Runner{MusterDir: t.TempDir()}
	output, err := runner.Run(context.Background(), config.OnChange, config.Hook{Command: `set -- {agent}; echo $#`, Timeout: 5 * time.Second}, Vars{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(output) != "1" {
		t.Fatalf("got %q, want one empty argument", output)
	}
}

func TestFailureCarriesStderrTail(t *testing.T) {
	runner := Runner{MusterDir: t.TempDir()}
	_, err := runner.Run(context.Background(), config.StartAgent, config.Hook{Command: `for i in 1 2 3 4 5 6 7; do echo line$i >&2; done; exit 4`, Timeout: 5 * time.Second}, Vars{})
	var hookErr *Error
	if !errors.As(err, &hookErr) {
		t.Fatalf("got %v, want a hook error", err)
	}
	if hookErr.Hook != config.StartAgent || strings.Contains(hookErr.StderrTail, "line2") || !strings.HasSuffix(hookErr.StderrTail, "line7") {
		t.Fatalf("unexpected error %#v", hookErr)
	}
}

func TestTimeoutKillsTheWholeProcessGroup(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "survived")
	runner := Runner{MusterDir: dir}
	started := time.Now()
	_, err := runner.Run(context.Background(), config.AgentStatus, config.Hook{Command: `(sleep 1; touch ` + marker + `) & sleep 5`, Timeout: 200 * time.Millisecond}, Vars{})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("got %v, want a timeout", err)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("timeout took %s", elapsed)
	}
	time.Sleep(1500 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a child of the timed-out hook kept running")
	}
}

func TestBackgroundChildHoldingStdoutDoesNotFailTheHook(t *testing.T) {
	runner := Runner{MusterDir: t.TempDir()}
	output, err := runner.Run(context.Background(), config.StartAgent, config.Hook{Command: `sleep 4 & echo handle`, Timeout: 10 * time.Second}, Vars{})
	if err != nil {
		t.Fatal(err)
	}
	if LastLine(output) != "handle" {
		t.Fatalf("got %q", output)
	}
}

func TestHookRunsInWorkspaceWhenItExists(t *testing.T) {
	musterDir, workspace := t.TempDir(), t.TempDir()
	runner := Runner{MusterDir: musterDir}
	hook := config.Hook{Command: `pwd -P`, Timeout: 5 * time.Second}
	for _, tc := range []struct {
		workspace, want string
	}{
		{workspace, workspace},
		{filepath.Join(workspace, "missing"), musterDir},
		{"", musterDir},
	} {
		output, err := runner.Run(context.Background(), config.OnChange, hook, Vars{Workspace: tc.workspace})
		if err != nil {
			t.Fatal(err)
		}
		want, _ := filepath.EvalSymlinks(tc.want)
		if LastLine(output) != want {
			t.Errorf("workspace %q: ran in %q, want %q", tc.workspace, LastLine(output), want)
		}
	}
}
