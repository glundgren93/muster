package obsidian

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type CommandMuster struct {
	Bin string
}

func NewCommandMuster() CommandMuster {
	bin := os.Getenv("MUSTER_BIN")
	if bin == "" {
		bin = "muster"
	}
	return CommandMuster{Bin: bin}
}

func (m CommandMuster) run(stdin string, out any, args ...string) error {
	cmd := exec.Command(m.Bin, args...)
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	if stdout.Len() > 0 && out != nil {
		if err := json.Unmarshal(stdout.Bytes(), out); err != nil && runErr == nil {
			return fmt.Errorf("muster %s: %w", args[0], err)
		}
	}
	if runErr != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = runErr.Error()
		}
		return fmt.Errorf("muster %s: %s", args[0], strings.TrimPrefix(message, "muster: "))
	}
	return nil
}

func (m CommandMuster) Runs() ([]Run, error) {
	var runs []Run
	err := m.run("", &runs, "status", "--all", "--cached", "-o", "json")
	return runs, err
}

func (m CommandMuster) Start(request StartRequest) (Run, error) {
	args := []string{"start", "-o", "json", "-f", "-", "--title=" + request.Title, "--source=" + request.Source}
	if request.Model != "" {
		args = append(args, "--model="+request.Model)
	}
	if request.Workspace != "" {
		args = append(args, "--workspace="+request.Workspace)
	}
	var run Run
	err := m.run(request.Prompt, &run, args...)
	return run, err
}

func (m CommandMuster) Follow(id, message string) (Run, error) {
	var run Run
	err := m.run("", &run, "follow", "-o", "json", "--", id, message)
	return run, err
}

func (m CommandMuster) Retry(id string) (Run, error) {
	var run Run
	err := m.run("", &run, "retry", "-o", "json", id)
	return run, err
}

func (m CommandMuster) Done(id string) (Run, error) {
	var run Run
	err := m.run("", &run, "done", "-o", "json", id)
	return run, err
}

func (m CommandMuster) Tick() error {
	return m.run("", nil, "tick")
}
