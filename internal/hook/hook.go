package hook

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/glundgren93/muster/internal/config"
)

const (
	stderrTailLines    = 5
	orphanedOutputWait = 2 * time.Second
)

type Vars struct {
	ID         string
	Title      string
	Workspace  string
	Agent      string
	Model      string
	Source     string
	PromptFile string
	Status     string
	Previous   string
	Message    string
	Lines      string
}

type placeholder struct {
	name  string
	env   string
	value func(Vars) string
}

var placeholders = []placeholder{
	{"id", "MUSTER_RUN_ID", func(v Vars) string { return v.ID }},
	{"title", "MUSTER_TITLE", func(v Vars) string { return v.Title }},
	{"workspace", "MUSTER_WORKSPACE", func(v Vars) string { return v.Workspace }},
	{"agent", "MUSTER_AGENT", func(v Vars) string { return v.Agent }},
	{"model", "MUSTER_MODEL", func(v Vars) string { return v.Model }},
	{"source", "MUSTER_SOURCE", func(v Vars) string { return v.Source }},
	{"prompt_file", "MUSTER_PROMPT_FILE", func(v Vars) string { return v.PromptFile }},
	{"status", "MUSTER_STATUS", func(v Vars) string { return v.Status }},
	{"previous", "MUSTER_PREVIOUS_STATUS", func(v Vars) string { return v.Previous }},
	{"message", "MUSTER_MESSAGE", func(v Vars) string { return v.Message }},
	{"lines", "MUSTER_LINES", func(v Vars) string { return v.Lines }},
}

var placeholderPattern = regexp.MustCompile(`\{([a-z_]+)\}`)

func Expand(command string, vars Vars) string {
	return placeholderPattern.ReplaceAllStringFunc(command, func(match string) string {
		name := match[1 : len(match)-1]
		for _, p := range placeholders {
			if p.name == name {
				return shellQuote(p.value(vars))
			}
		}
		return match
	})
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func LastLine(output string) string {
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}

type Error struct {
	Hook       config.HookName
	Reason     string
	StderrTail string
}

func (e *Error) Error() string {
	if e.StderrTail == "" {
		return fmt.Sprintf("%s: %s", e.Hook, e.Reason)
	}
	return fmt.Sprintf("%s: %s: %s", e.Hook, e.Reason, e.StderrTail)
}

type Runner struct {
	MusterDir string
	Bin       string
}

func (r Runner) Run(ctx context.Context, name config.HookName, hook config.Hook, vars Vars) (string, error) {
	hookCtx, cancel := context.WithTimeout(ctx, hook.Timeout)
	defer cancel()

	cmd := exec.CommandContext(hookCtx, "sh", "-c", Expand(hook.Command, vars))
	cmd.Dir = r.workingDir(vars)
	cmd.Env = r.environment(vars)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = orphanedOutputWait
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return stdout.String(), nil
	}
	reason := err.Error()
	switch {
	case ctx.Err() != nil:
		reason = "interrupted"
	case errors.Is(hookCtx.Err(), context.DeadlineExceeded):
		reason = fmt.Sprintf("timed out after %s", hook.Timeout)
	}
	return stdout.String(), &Error{Hook: name, Reason: reason, StderrTail: tail(stderr.String(), stderrTailLines)}
}

func (r Runner) workingDir(vars Vars) string {
	if vars.Workspace != "" {
		if info, err := os.Stat(vars.Workspace); err == nil && info.IsDir() {
			return vars.Workspace
		}
	}
	return r.MusterDir
}

func (r Runner) environment(vars Vars) []string {
	env := append(os.Environ(), "MUSTER_DIR="+r.MusterDir, "MUSTER_BIN="+r.Bin)
	for _, p := range placeholders {
		env = append(env, p.env+"="+p.value(vars))
	}
	return env
}

func tail(output string, count int) string {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
