package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "muster-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary = filepath.Join(dir, "muster")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "build muster:", err)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type shell struct {
	t   *testing.T
	dir string
}

func (c shell) run(args ...string) (string, string, error) {
	c.t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(), "MUSTER_DIR="+c.dir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func (c shell) must(args ...string) string {
	c.t.Helper()
	stdout, stderr, err := c.run(args...)
	if err != nil {
		c.t.Fatalf("muster %s: %v\n%s", strings.Join(args, " "), err, stderr)
	}
	return stdout
}

func (c shell) write(name, content string, mode os.FileMode) {
	c.t.Helper()
	path := filepath.Join(c.dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		c.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		c.t.Fatal(err)
	}
}

func TestEndToEnd(t *testing.T) {
	c := shell{t: t, dir: t.TempDir()}
	c.write("config.toml", `
[hooks]
start_agent = "echo pane-{id}"
agent_status = "cat \"$MUSTER_DIR/agent-state\""
`, 0o644)
	c.write("agent-state", "working\n", 0o644)

	var started struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		Agent   string `json:"agent"`
		Source  string `json:"source"`
		Created bool   `json:"created"`
	}
	if err := json.Unmarshal([]byte(c.must("start", "--source", "test:1", "-o", "json", "Write the release notes")), &started); err != nil {
		t.Fatal(err)
	}
	if started.ID != "write-the-release-notes" || started.Status != "running" || started.Agent != "pane-write-the-release-notes" || !started.Created {
		t.Fatalf("start printed %+v", started)
	}
	if err := json.Unmarshal([]byte(c.must("start", "--source", "test:1", "-o", "json", "Again")), &started); err != nil || started.Created {
		t.Fatalf("second start with the same source: %+v, %v", started, err)
	}

	c.must("tick")
	c.write("agent-state", "idle\n", 0o644)
	var transitions []map[string]string
	if err := json.Unmarshal([]byte(c.must("tick", "-o", "json")), &transitions); err != nil {
		t.Fatal(err)
	}
	if len(transitions) != 1 || transitions[0]["to"] != "waiting" {
		t.Fatalf("tick printed %v", transitions)
	}

	var runs []map[string]any
	if err := json.Unmarshal([]byte(c.must("status", "-o", "json")), &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0]["status"] != "waiting" || runs[0]["agent_state"] != "idle" {
		t.Fatalf("status printed %v", runs)
	}

	c.must("done", "write")
	if out := c.must("status", "-o", "json"); strings.TrimSpace(out) != "[]" {
		t.Fatalf("status without --all after done: %s", out)
	}
	if out := c.must("status", "--all"); !strings.Contains(out, "done") {
		t.Fatalf("status --all: %s", out)
	}

	c.write("plugins/muster-hello", "#!/bin/sh\necho \"hello $1 $MUSTER_DIR $(\"$MUSTER_BIN\" status --all --cached -o json | grep -c '\"id\"')\"\n", 0o755)
	if out := c.must("hello", "world"); strings.TrimSpace(out) != "hello world "+c.dir+" 1" {
		t.Fatalf("plugin printed %q", out)
	}

	if _, stderr, err := c.run("nosuchcommand"); err == nil || !strings.Contains(stderr, "unknown command") {
		t.Fatalf("unknown command: %v %s", err, stderr)
	}
}

func TestDoctorFailsWithoutRequiredHooks(t *testing.T) {
	c := shell{t: t, dir: t.TempDir()}
	stdout, _, err := c.run("doctor")
	if err == nil || !strings.Contains(stdout, "start_agent is required") {
		t.Fatalf("doctor: %v\n%s", err, stdout)
	}
}

func TestBadConfigIsReported(t *testing.T) {
	c := shell{t: t, dir: t.TempDir()}
	c.write("config.toml", "max_runing = 2\n", 0o644)
	_, stderr, err := c.run("status")
	if err == nil || !strings.Contains(stderr, "max_runing") {
		t.Fatalf("status with a typo in config: %v %s", err, stderr)
	}
}
