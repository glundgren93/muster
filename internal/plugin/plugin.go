package plugin

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/glundgren93/muster/internal/layout"
)

const executablePrefix = "muster-"

func Find(dir layout.Dir, name string) (string, bool) {
	if name == "" || strings.ContainsRune(name, filepath.Separator) {
		return "", false
	}
	executable := executablePrefix + name
	candidate := filepath.Join(dir.PluginsDir(), executable)
	if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
		return candidate, true
	}
	if path, err := exec.LookPath(executable); err == nil {
		return path, true
	}
	return "", false
}

func Exec(dir layout.Dir, path string, args []string, bin string) error {
	env := append(os.Environ(),
		"MUSTER_DIR="+dir.Root,
		"MUSTER_CONFIG="+dir.ConfigFile(),
		"MUSTER_STATE="+dir.StateFile(),
		"MUSTER_BIN="+bin,
	)
	return syscall.Exec(path, append([]string{path}, args...), env)
}
