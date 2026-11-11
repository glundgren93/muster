package cli

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/glundgren93/muster/internal/layout"
	"github.com/glundgren93/muster/presets"
)

func newInitCommand() *cobra.Command {
	var force bool
	command := &cobra.Command{
		Use:   "init [PRESET]",
		Short: "Copy a preset into the muster directory, or list the presets",
		Long:  "Copy a preset's config.toml and hook scripts into MUSTER_DIR. Existing files are left alone unless --force. Without PRESET, list the presets.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return listPresets(cmd.OutOrStdout())
			}
			dir, err := layout.Resolve()
			if err != nil {
				return err
			}
			return installPreset(cmd.OutOrStdout(), dir, args[0], force)
		},
	}
	command.Flags().BoolVar(&force, "force", false, "overwrite existing files")
	return command
}

func presetNames() ([]string, error) {
	entries, err := fs.ReadDir(presets.Files, ".")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

func listPresets(w io.Writer) error {
	names, err := presetNames()
	if err != nil {
		return err
	}
	for _, name := range names {
		readme, _ := fs.ReadFile(presets.Files, path.Join(name, "README.md"))
		title, _, _ := strings.Cut(string(readme), "\n")
		fmt.Fprintf(w, "%-24s %s\n", name, strings.TrimPrefix(title, "# "))
	}
	return nil
}

func installPreset(w io.Writer, dir layout.Dir, name string, force bool) error {
	if _, err := fs.Stat(presets.Files, name); err != nil {
		names, _ := presetNames()
		return fmt.Errorf("no preset %q; presets: %s", name, strings.Join(names, ", "))
	}
	var targets []string
	err := fs.WalkDir(presets.Files, name, func(source string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || entry.Name() == "README.md" {
			return err
		}
		targets = append(targets, source)
		return nil
	})
	if err != nil {
		return err
	}
	if !force {
		var existing []string
		for _, source := range targets {
			target := filepath.Join(dir.Root, filepath.FromSlash(strings.TrimPrefix(source, name+"/")))
			if _, err := os.Stat(target); err == nil {
				existing = append(existing, target)
			}
		}
		if len(existing) > 0 {
			return fmt.Errorf("these files exist; pass --force to overwrite them:\n  %s", strings.Join(existing, "\n  "))
		}
	}
	for _, source := range targets {
		relative := strings.TrimPrefix(source, name+"/")
		target := filepath.Join(dir.Root, filepath.FromSlash(relative))
		data, err := fs.ReadFile(presets.Files, source)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if strings.HasPrefix(relative, "hooks/") {
			mode = 0o755
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, mode); err != nil {
			return err
		}
		if err := os.Chmod(target, mode); err != nil {
			return err
		}
		fmt.Fprintf(w, "wrote %s\n", target)
	}
	fmt.Fprintf(w, "edit %s, then run muster doctor\n", dir.ConfigFile())
	return nil
}
