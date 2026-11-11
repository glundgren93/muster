package layout

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const dirEnv = "MUSTER_DIR"

type Dir struct {
	Root string
}

func Resolve() (Dir, error) {
	root := os.Getenv(dirEnv)
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Dir{}, fmt.Errorf("resolve muster directory: %w", err)
		}
		root = filepath.Join(home, ".muster")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return Dir{}, fmt.Errorf("resolve muster directory %s: %w", root, err)
	}
	return Dir{Root: absolute}, nil
}

func (d Dir) Ensure() error {
	if d.Root == "" {
		return errors.New("muster directory is not set")
	}
	return os.MkdirAll(d.Root, 0o755)
}

func (d Dir) ConfigFile() string { return filepath.Join(d.Root, "config.toml") }

func (d Dir) StateFile() string { return filepath.Join(d.Root, "state.json") }

func (d Dir) LockFile() string { return filepath.Join(d.Root, "state.lock") }

func (d Dir) LogFile() string { return filepath.Join(d.Root, "muster.log") }

func (d Dir) PluginsDir() string { return filepath.Join(d.Root, "plugins") }

func (d Dir) RunDir(id string) string { return filepath.Join(d.Root, "runs", id) }

func (d Dir) PromptFile(id string) string { return filepath.Join(d.RunDir(id), "prompt.md") }

func (d Dir) WorkspaceDir(id string) string { return filepath.Join(d.Root, "workspaces", id) }
