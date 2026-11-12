package obsidian

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	StatusKey    = "status"
	RunKey       = "run"
	FollowUpKey  = "follow_up"
	ModelKey     = "model"
	WorkspaceKey = "workspace"

	StatusNow  = "now"
	StatusDone = "done"

	SourcePrefix = "obsidian:"
)

type Run struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Source   string `json:"source"`
	Sessions int    `json:"sessions"`
}

type StartRequest struct {
	Title     string
	Prompt    string
	Source    string
	Model     string
	Workspace string
}

type Muster interface {
	Runs() ([]Run, error)
	Start(StartRequest) (Run, error)
	Follow(id, message string) (Run, error)
	Retry(id string) (Run, error)
	Done(id string) (Run, error)
}

type Syncer struct {
	Dir    string
	Muster Muster
	Log    io.Writer
}

type Result struct {
	Started, FollowedUp, Retried, Closed, Updated int
	Errors                                        []error
}

func (r Result) Changed() bool {
	return r.Started+r.FollowedUp+r.Retried+r.Closed+r.Updated > 0
}

func (s Syncer) Sync() (Result, error) {
	var result Result
	runs, err := s.Muster.Runs()
	if err != nil {
		return result, err
	}
	byID := map[string]Run{}
	bySource := map[string]Run{}
	for _, run := range runs {
		byID[run.ID] = run
		if run.Source != "" {
			bySource[run.Source] = run
		}
	}
	paths, err := s.notes()
	if err != nil {
		return result, err
	}
	for _, path := range paths {
		if err := s.syncNote(path, byID, bySource, &result); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("%s: %w", s.relative(path), err))
		}
	}
	return result, nil
}

func (s Syncer) notes() ([]string, error) {
	var paths []string
	err := filepath.WalkDir(s.Dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && path != s.Dir && strings.HasPrefix(entry.Name(), ".") {
			return filepath.SkipDir
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			paths = append(paths, path)
		}
		return nil
	})
	slices.Sort(paths)
	return paths, err
}

func (s Syncer) relative(path string) string {
	if rel, err := filepath.Rel(s.Dir, path); err == nil {
		return filepath.ToSlash(rel)
	}
	return path
}

func (s Syncer) syncNote(path string, byID, bySource map[string]Run, result *Result) error {
	original, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	note := ParseNote(string(original))
	written, isTask := note.Get(StatusKey)
	if !isTask {
		return nil
	}
	status := strings.ToLower(strings.TrimSpace(written))
	followUp, _ := note.Get(FollowUpKey)
	followUp = strings.TrimSpace(followUp)
	source := SourcePrefix + s.relative(path)
	run, linked := s.linkedRun(note, source, byID, bySource)

	var actionErr error
	holdNow := false
	switch {
	case status == StatusNow && !linked:
		run, actionErr = s.Muster.Start(s.startRequest(path, note, source, followUp))
		if run.ID != "" {
			linked = true
			note.Delete(FollowUpKey)
			result.Started++
			s.logf("started %s from %s", run.ID, s.relative(path))
		}
	case status == StatusNow:
		switch {
		case followUp != "" && run.Sessions > 0 && slices.Contains([]string{"waiting", "done", "failed", "queued"}, run.Status):
			if run, actionErr = s.Muster.Follow(run.ID, followUp); actionErr == nil {
				note.Delete(FollowUpKey)
				result.FollowedUp++
				s.logf("followed up %s", run.ID)
			}
		case followUp != "":
			holdNow = true
		case run.Status == "failed":
			if run, actionErr = s.Muster.Retry(run.ID); actionErr == nil {
				result.Retried++
				s.logf("retried %s", run.ID)
			}
		}
	case status == StatusDone && linked && run.Status != StatusDone && run.Status != "starting":
		if run, actionErr = s.Muster.Done(run.ID); actionErr == nil {
			result.Closed++
			s.logf("closed %s", run.ID)
		}
	}
	if linked && run.ID != "" {
		if !holdNow {
			note.Set(StatusKey, run.Status)
		}
		note.Set(RunKey, run.ID)
	}
	updated := note.String()
	if updated != string(original) {
		if err := replaceIfUnchanged(path, original, []byte(updated)); err != nil {
			return errors.Join(actionErr, err)
		}
		result.Updated++
	}
	return actionErr
}

func (s Syncer) linkedRun(note Note, source string, byID, bySource map[string]Run) (Run, bool) {
	if id, ok := note.Get(RunKey); ok && id != "" {
		if run, found := byID[id]; found {
			return run, true
		}
	}
	run, found := bySource[source]
	return run, found
}

func (s Syncer) startRequest(path string, note Note, source, followUp string) StartRequest {
	title := strings.TrimSuffix(filepath.Base(path), ".md")
	prompt := title
	if body := strings.TrimSpace(note.Body()); body != "" {
		prompt = title + "\n\n" + body
	}
	if followUp != "" {
		prompt += "\n\nFollow-up:\n" + followUp
	}
	model, _ := note.Get(ModelKey)
	workspace, _ := note.Get(WorkspaceKey)
	if workspace != "" && strings.HasPrefix(workspace, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			workspace = filepath.Join(home, workspace[2:])
		}
	}
	return StartRequest{Title: title, Prompt: prompt + "\n", Source: source, Model: model, Workspace: workspace}
}

func (s Syncer) logf(format string, args ...any) {
	if s.Log != nil {
		fmt.Fprintf(s.Log, format+"\n", args...)
	}
}

var ErrNoteChanged = errors.New("the note changed while muster was updating it; it will be updated on the next sync")

func replaceIfUnchanged(path string, expected, content []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, expected) {
		return ErrNoteChanged
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".muster-*.md")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(content); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(info.Mode().Perm()); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}
