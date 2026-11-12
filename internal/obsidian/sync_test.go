package obsidian

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeMuster struct {
	runs  map[string]*Run
	calls []string
	fail  error
}

func newFakeMuster(runs ...Run) *fakeMuster {
	f := &fakeMuster{runs: map[string]*Run{}}
	for _, run := range runs {
		f.runs[run.ID] = &run
	}
	return f
}

func (f *fakeMuster) Runs() ([]Run, error) {
	var runs []Run
	for _, run := range f.runs {
		runs = append(runs, *run)
	}
	return runs, nil
}

func (f *fakeMuster) Start(request StartRequest) (Run, error) {
	f.calls = append(f.calls, fmt.Sprintf("start %s %q %s model=%s workspace=%s", request.Source, request.Prompt, request.Title, request.Model, request.Workspace))
	if f.fail != nil {
		return Run{}, f.fail
	}
	run := &Run{ID: "run-" + fmt.Sprint(len(f.runs)+1), Status: "running", Source: request.Source, Sessions: 1}
	f.runs[run.ID] = run
	return *run, nil
}

func (f *fakeMuster) change(call, id, status string) (Run, error) {
	f.calls = append(f.calls, call+" "+id)
	if f.fail != nil {
		return Run{}, f.fail
	}
	run := f.runs[id]
	run.Status = status
	return *run, nil
}

func (f *fakeMuster) Follow(id, message string) (Run, error) {
	return f.change("follow "+fmt.Sprintf("%q", message), id, "running")
}

func (f *fakeMuster) Retry(id string) (Run, error) { return f.change("retry", id, "running") }

func (f *fakeMuster) Done(id string) (Run, error) { return f.change("done", id, "done") }

func writeNote(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readNote(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func sync(t *testing.T, dir string, muster Muster) Result {
	t.Helper()
	result, err := Syncer{Dir: dir, Muster: muster}.Sync()
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestNowStartsARunAndWritesItBack(t *testing.T) {
	dir := t.TempDir()
	path := writeNote(t, dir, "Projects/Fix login.md", "---\nstatus: now\nmodel: opus\npriority: high\n---\nThe login page returns 500.\n")
	writeNote(t, dir, "Reference.md", "---\ntags: [x]\n---\nNot a task.\n")
	writeNote(t, dir, ".trash/Old.md", "---\nstatus: now\n---\n")
	muster := newFakeMuster()

	result := sync(t, dir, muster)
	if result.Started != 1 || len(result.Errors) != 0 {
		t.Fatalf("result %+v", result)
	}
	if want := `start obsidian:Projects/Fix login.md "Fix login\n\nThe login page returns 500.\n" Fix login model=opus workspace=`; strings.Join(muster.calls, "\n") != want {
		t.Fatalf("calls:\n%s\nwant:\n%s", strings.Join(muster.calls, "\n"), want)
	}
	if got := readNote(t, path); got != "---\nstatus: running\nmodel: opus\npriority: high\nrun: run-1\n---\nThe login page returns 500.\n" {
		t.Fatalf("note:\n%s", got)
	}

	muster.calls = nil
	sync(t, dir, muster)
	if len(muster.calls) != 0 {
		t.Fatalf("second sync called %v", muster.calls)
	}
}

func TestStatusIsWrittenBackOnlyWhenItChanges(t *testing.T) {
	dir := t.TempDir()
	path := writeNote(t, dir, "Task.md", "---\nstatus: running\nrun: r\n---\n")
	muster := newFakeMuster(Run{ID: "r", Status: "waiting", Sessions: 1})
	if result := sync(t, dir, muster); result.Updated != 1 {
		t.Fatalf("result %+v", result)
	}
	if got := readNote(t, path); got != "---\nstatus: waiting\nrun: r\n---\n" {
		t.Fatalf("note %q", got)
	}
	info, _ := os.Stat(path)
	if result := sync(t, dir, muster); result.Updated != 0 {
		t.Fatalf("result %+v", result)
	}
	if after, _ := os.Stat(path); !after.ModTime().Equal(info.ModTime()) {
		t.Fatal("an unchanged note was rewritten")
	}
}

func TestFollowUpIsSentWhenTheRunStopped(t *testing.T) {
	dir := t.TempDir()
	path := writeNote(t, dir, "Task.md", "---\nstatus: now\nrun: r\nfollow_up: |\n  Also add a test.\n---\nBody\n")
	muster := newFakeMuster(Run{ID: "r", Status: "running", Sessions: 1})

	sync(t, dir, muster)
	if len(muster.calls) != 0 || !strings.Contains(readNote(t, path), "status: now\n") {
		t.Fatalf("a follow-up for a running run should wait with status now; calls %v, note:\n%s", muster.calls, readNote(t, path))
	}

	muster.runs["r"].Status = "waiting"
	sync(t, dir, muster)
	if strings.Join(muster.calls, "\n") != `follow "Also add a test." r` {
		t.Fatalf("calls %v", muster.calls)
	}
	if got := readNote(t, path); got != "---\nstatus: running\nrun: r\n---\nBody\n" {
		t.Fatalf("note:\n%s", got)
	}
}

func TestNowOnAFailedRunRetriesAndDoneClosesTheRun(t *testing.T) {
	dir := t.TempDir()
	failed := writeNote(t, dir, "Failed.md", "---\nstatus: now\nrun: f\n---\n")
	finished := writeNote(t, dir, "Finished.md", "---\nstatus: done\nrun: w\n---\n")
	waiting := writeNote(t, dir, "Waiting.md", "---\nstatus: now\nrun: x\n---\n")
	muster := newFakeMuster(Run{ID: "f", Status: "failed", Sessions: 0}, Run{ID: "w", Status: "waiting", Sessions: 1}, Run{ID: "x", Status: "waiting", Sessions: 1})

	sync(t, dir, muster)
	if strings.Join(muster.calls, ",") != "retry f,done w" {
		t.Fatalf("calls %v", muster.calls)
	}
	for path, want := range map[string]string{failed: "running", finished: "done", waiting: "waiting"} {
		if !strings.Contains(readNote(t, path), "status: "+want+"\n") {
			t.Errorf("%s:\n%s", filepath.Base(path), readNote(t, path))
		}
	}
}

func TestFailedStartKeepsNowAndReportsTheNote(t *testing.T) {
	dir := t.TempDir()
	path := writeNote(t, dir, "Task.md", "---\nstatus: now\n---\n")
	muster := newFakeMuster()
	muster.fail = errors.New("start_agent is not configured")
	result := sync(t, dir, muster)
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Error(), "Task.md") {
		t.Fatalf("errors %v", result.Errors)
	}
	if got := readNote(t, path); got != "---\nstatus: now\n---\n" {
		t.Fatalf("note %q", got)
	}
}

func TestRenamedNoteKeepsItsRun(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "New name.md", "---\nstatus: now\nrun: r\n---\n")
	muster := newFakeMuster(Run{ID: "r", Status: "waiting", Source: "obsidian:Old name.md", Sessions: 1})
	sync(t, dir, muster)
	if len(muster.calls) != 0 {
		t.Fatalf("calls %v", muster.calls)
	}
}

func TestNoteEditedDuringSyncIsNotOverwritten(t *testing.T) {
	path := writeNote(t, t.TempDir(), "Task.md", "---\nstatus: now\n---\n")
	err := replaceIfUnchanged(path, []byte("---\nstatus: backlog\n---\n"), []byte("new"))
	if !errors.Is(err, ErrNoteChanged) {
		t.Fatalf("got %v", err)
	}
	if got := readNote(t, path); got != "---\nstatus: now\n---\n" {
		t.Fatalf("note %q", got)
	}
}

func TestFollowUpOnANoteWithoutARunJoinsTheFirstPrompt(t *testing.T) {
	dir := t.TempDir()
	path := writeNote(t, dir, "Imported.md", "---\nstatus: now\nworkspace: /work/earlier\nfollow_up: Also cover logout\n---\nRefactor auth.\n")
	muster := newFakeMuster()

	result := sync(t, dir, muster)
	if result.Started != 1 || len(result.Errors) != 0 {
		t.Fatalf("result %+v", result)
	}
	if want := `start obsidian:Imported.md "Imported\n\nRefactor auth.\n\nFollow-up:\nAlso cover logout\n" Imported model= workspace=/work/earlier`; strings.Join(muster.calls, "\n") != want {
		t.Fatalf("calls:\n%s\nwant:\n%s", strings.Join(muster.calls, "\n"), want)
	}
	if got := readNote(t, path); got != "---\nstatus: running\nworkspace: /work/earlier\nrun: run-1\n---\nRefactor auth.\n" {
		t.Fatalf("note:\n%s", got)
	}
}

func TestStatusIsReadWhateverItsCase(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "Start me.md", "---\nstatus: Now\n---\n")
	writeNote(t, dir, "Close me.md", "---\nstatus: DONE\nrun: run-9\n---\n")
	muster := newFakeMuster(Run{ID: "run-9", Status: "waiting", Sessions: 1})

	result := sync(t, dir, muster)
	if result.Started != 1 || result.Closed != 1 || len(result.Errors) != 0 {
		t.Fatalf("result %+v, calls %v", result, muster.calls)
	}
}
