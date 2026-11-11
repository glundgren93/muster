package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type HookName string

const (
	PickModel       HookName = "pick_model"
	CreateWorkspace HookName = "create_workspace"
	StartAgent      HookName = "start_agent"
	AgentStatus     HookName = "agent_status"
	ReadAgent       HookName = "read_agent"
	SendAgent       HookName = "send_agent"
	StopAgent       HookName = "stop_agent"
	LastReport      HookName = "last_report"
	OnChange        HookName = "on_change"
	OnQueued        HookName = "on_queued"
	OnRunning       HookName = "on_running"
	OnWaiting       HookName = "on_waiting"
	OnDone          HookName = "on_done"
	OnFailed        HookName = "on_failed"
)

const eventHookTimeout = 30 * time.Second

var defaultHookTimeouts = map[HookName]time.Duration{
	PickModel:       2 * time.Minute,
	CreateWorkspace: 10 * time.Minute,
	StartAgent:      2 * time.Minute,
	AgentStatus:     15 * time.Second,
	ReadAgent:       30 * time.Second,
	SendAgent:       30 * time.Second,
	StopAgent:       30 * time.Second,
	LastReport:      30 * time.Second,
	OnChange:        eventHookTimeout,
	OnQueued:        eventHookTimeout,
	OnRunning:       eventHookTimeout,
	OnWaiting:       eventHookTimeout,
	OnDone:          eventHookTimeout,
	OnFailed:        eventHookTimeout,
}

var RequiredHooks = []HookName{StartAgent, AgentStatus}

var startHooks = []HookName{OnChange, PickModel, CreateWorkspace, LastReport, StopAgent, StartAgent}

type Hook struct {
	Command string
	Timeout time.Duration
}

type Config struct {
	MaxRunning   int
	Interval     time.Duration
	StartGrace   time.Duration
	DefaultModel string
	Hooks        map[HookName]Hook
}

func Default() Config {
	return Config{
		MaxRunning: 5,
		Interval:   10 * time.Second,
		StartGrace: 60 * time.Second,
		Hooks:      map[HookName]Hook{},
	}
}

func (c Config) Hook(name HookName) (Hook, bool) {
	hook, ok := c.Hooks[name]
	return hook, ok
}

func (c Config) StartTimeout() time.Duration {
	var total time.Duration
	for _, name := range startHooks {
		if hook, ok := c.Hooks[name]; ok {
			total += hook.Timeout
		}
	}
	return total
}

type fileConfig struct {
	MaxRunning   *int                      `toml:"max_running"`
	Interval     *string                   `toml:"interval"`
	StartGrace   *string                   `toml:"start_grace"`
	DefaultModel *string                   `toml:"default_model"`
	Hooks        map[string]toml.Primitive `toml:"hooks"`
}

type hookTable struct {
	Command string `toml:"command"`
	Timeout string `toml:"timeout"`
}

func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	if err := parse(string(data), &cfg); err != nil {
		return Config{}, fmt.Errorf("config %s: %w", path, err)
	}
	return cfg, nil
}

func parse(data string, cfg *Config) error {
	var raw fileConfig
	meta, err := toml.Decode(data, &raw)
	if err != nil {
		return err
	}
	if raw.MaxRunning != nil {
		if *raw.MaxRunning < 0 {
			return fmt.Errorf("max_running must not be negative, got %d", *raw.MaxRunning)
		}
		cfg.MaxRunning = *raw.MaxRunning
	}
	if raw.Interval != nil {
		if cfg.Interval, err = positiveDuration("interval", *raw.Interval); err != nil {
			return err
		}
	}
	if raw.StartGrace != nil {
		grace, err := time.ParseDuration(*raw.StartGrace)
		if err != nil || grace < 0 {
			return fmt.Errorf("start_grace: %q is not a non-negative duration", *raw.StartGrace)
		}
		cfg.StartGrace = grace
	}
	if raw.DefaultModel != nil {
		cfg.DefaultModel = *raw.DefaultModel
	}
	for key, value := range raw.Hooks {
		name := HookName(key)
		defaultTimeout, known := defaultHookTimeouts[name]
		if !known {
			return fmt.Errorf("unknown hook %q", key)
		}
		hook, err := decodeHook(meta, key, value, defaultTimeout)
		if err != nil {
			return fmt.Errorf("hook %s: %w", key, err)
		}
		cfg.Hooks[name] = hook
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, 0, len(undecoded))
		for _, key := range undecoded {
			keys = append(keys, key.String())
		}
		slices.Sort(keys)
		return fmt.Errorf("unknown settings: %s", strings.Join(keys, ", "))
	}
	return nil
}

func decodeHook(meta toml.MetaData, key string, value toml.Primitive, defaultTimeout time.Duration) (Hook, error) {
	hook := Hook{Timeout: defaultTimeout}
	switch meta.Type("hooks", key) {
	case "String":
		if err := meta.PrimitiveDecode(value, &hook.Command); err != nil {
			return Hook{}, err
		}
	case "Hash":
		var table hookTable
		if err := meta.PrimitiveDecode(value, &table); err != nil {
			return Hook{}, err
		}
		hook.Command = table.Command
		if table.Timeout != "" {
			timeout, err := positiveDuration("timeout", table.Timeout)
			if err != nil {
				return Hook{}, err
			}
			hook.Timeout = timeout
		}
	default:
		return Hook{}, errors.New("must be a command string or a table with command and timeout")
	}
	if strings.TrimSpace(hook.Command) == "" {
		return Hook{}, errors.New("command is empty")
	}
	return hook, nil
}

func positiveDuration(setting, value string) (time.Duration, error) {
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s: %q is not a positive duration", setting, value)
	}
	return duration, nil
}
