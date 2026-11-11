package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var musterBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "muster-obsidian-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	musterBin = filepath.Join(dir, "muster")
	for target, pkg := range map[string]string{musterBin: "../muster", filepath.Join(dir, "muster-obsidian"): "."} {
		build := exec.Command("go", "build", "-o", target, pkg)
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "build:", err)
			os.Exit(1)
		}
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func TestSyncThroughMuster(t *testing.T) {
	musterDir, notes := t.TempDir(), t.TempDir()
	write := func(path, content string, mode os.FileMode) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	read := func(path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	plugin, err := os.ReadFile(filepath.Join(filepath.Dir(musterBin), "muster-obsidian"))
	if err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(musterDir, "plugins", "muster-obsidian"), string(plugin), 0o755)
	write(filepath.Join(musterDir, "config.toml"), `
[hooks]
start_agent = "cp \"$MUSTER_PROMPT_FILE\" \"$MUSTER_DIR/prompt-$MUSTER_RUN_ID-$(date +%s%N)\"; echo pane"
agent_status = "cat \"$MUSTER_DIR/agent-state\""
`, 0o644)
	write(filepath.Join(musterDir, "agent-state"), "working\n", 0o644)
	note := filepath.Join(notes, "Fix the flaky test.md")
	write(note, "---\nstatus: now\n---\nIt fails one run in ten.\n", 0o644)

	muster := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(musterBin, args...)
		cmd.Env = append(os.Environ(), "MUSTER_DIR="+musterDir)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("muster %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
		}
		return stdout.String()
	}

	muster("obsidian", "sync", "--dir", notes)
	if got := read(note); got != "---\nstatus: running\nrun: fix-the-flaky-test\n---\nIt fails one run in ten.\n" {
		t.Fatalf("note after start:\n%s", got)
	}
	if out := muster("status", "fix", "-o", "json"); !strings.Contains(out, `"source": "obsidian:Fix the flaky test.md"`) || !strings.Contains(out, `"prompt": "Fix the flaky test\n\nIt fails one run in ten.\n"`) {
		t.Fatalf("run:\n%s", out)
	}

	muster("tick")
	write(filepath.Join(musterDir, "agent-state"), "idle\n", 0o644)
	muster("tick")
	muster("obsidian", "sync", "--dir", notes)
	if got := read(note); !strings.Contains(got, "status: waiting\n") {
		t.Fatalf("note after the agent stopped:\n%s", got)
	}

	write(note, strings.Replace(read(note), "status: waiting\n", "status: now\nfollow_up: Also fix the other flaky test\n", 1), 0o644)
	muster("obsidian", "sync", "--dir", notes)
	if got := read(note); got != "---\nstatus: running\nrun: fix-the-flaky-test\n---\nIt fails one run in ten.\n" {
		t.Fatalf("note after the follow-up:\n%s", got)
	}
	if out := muster("status", "fix", "-o", "json"); !strings.Contains(out, `"sessions": 2`) || !strings.Contains(out, "Also fix the other flaky test") {
		t.Fatalf("run after the follow-up:\n%s", out)
	}

	write(note, strings.Replace(read(note), "status: running\n", "status: done\n", 1), 0o644)
	muster("obsidian", "sync", "--dir", notes)
	if out := muster("status", "fix", "--cached", "-o", "json"); !strings.Contains(out, `"status": "done"`) {
		t.Fatalf("run after done:\n%s", out)
	}
	if got := read(note); !strings.Contains(got, "status: done\n") {
		t.Fatalf("note after done:\n%s", got)
	}
}
