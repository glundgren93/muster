package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func load(t *testing.T, content string) (Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func TestMissingFileMeansDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxRunning != 5 || cfg.Interval != 10*time.Second || cfg.StartGrace != time.Minute || len(cfg.Hooks) != 0 {
		t.Fatalf("defaults %+v", cfg)
	}
}

func TestHooksAsStringsAndTables(t *testing.T) {
	cfg, err := load(t, `
max_running = 0
start_grace = "0s"

[hooks]
agent_status = "rex-status {agent}"

[hooks.start_agent]
command = "start.sh"
timeout = "5m"
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxRunning != 0 || cfg.StartGrace != 0 {
		t.Fatalf("settings %+v", cfg)
	}
	if hook := cfg.Hooks[AgentStatus]; hook.Command != "rex-status {agent}" || hook.Timeout != 15*time.Second {
		t.Fatalf("agent_status %+v", hook)
	}
	if hook := cfg.Hooks[StartAgent]; hook.Command != "start.sh" || hook.Timeout != 5*time.Minute {
		t.Fatalf("start_agent %+v", hook)
	}
}

func TestTyposAreErrors(t *testing.T) {
	for name, content := range map[string]string{
		"unknown setting":     "max_runing = 3\n",
		"unknown hook":        "[hooks]\nstart_agnet = \"x\"\n",
		"unknown hook field":  "[hooks.start_agent]\ncommand = \"x\"\ntimout = \"1m\"\n",
		"empty command":       "[hooks]\nstart_agent = \" \"\n",
		"bad duration":        "interval = \"10\"\n",
		"negative max":        "max_running = -1\n",
		"hook of wrong type":  "[hooks]\nstart_agent = 3\n",
		"zero hook timeout":   "[hooks.start_agent]\ncommand = \"x\"\ntimeout = \"0s\"\n",
		"top-level hook name": "start_agent = \"x\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := load(t, content); err == nil {
				t.Fatal("want an error")
			} else if !strings.Contains(err.Error(), "config.toml") {
				t.Fatalf("error %q does not name the file", err)
			}
		})
	}
}

func TestPresetsLoad(t *testing.T) {
	paths, err := filepath.Glob("../../presets/*/config.toml")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no presets found: %v", err)
	}
	for _, path := range paths {
		cfg, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range RequiredHooks {
			if _, ok := cfg.Hook(name); !ok {
				t.Errorf("%s lacks %s", path, name)
			}
		}
	}
}
