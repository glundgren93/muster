package state

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func newTestStore(dir string) Store {
	return NewStore(filepath.Join(dir, "state.json"), filepath.Join(dir, "state.lock"))
}

func TestConcurrentWritersDoNotLoseUpdates(t *testing.T) {
	dir := t.TempDir()
	const writers, updates = 4, 50
	var wg sync.WaitGroup
	errs := make(chan error, writers*updates)
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store := newTestStore(dir)
			for i := range updates {
				errs <- store.Update(func(file *File) error {
					file.Runs = append(file.Runs, Run{ID: fmt.Sprintf("w%d-%d", w, i)})
					return nil
				})
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	file, err := newTestStore(dir).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Runs) != writers*updates {
		t.Fatalf("got %d runs, want %d", len(file.Runs), writers*updates)
	}
}

func TestFailedMutationLeavesStateUnchanged(t *testing.T) {
	store := newTestStore(t.TempDir())
	if err := store.Update(func(file *File) error {
		file.Runs = append(file.Runs, Run{ID: "kept"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(func(file *File) error {
		file.Runs = nil
		return fmt.Errorf("refused")
	}); err == nil {
		t.Fatal("want the mutation error")
	}
	file, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Runs) != 1 || file.Runs[0].ID != "kept" {
		t.Fatalf("state changed: %+v", file.Runs)
	}
}

const writerEnv = "MUSTER_TEST_STATE_WRITER"

func TestInterruptedSaveNeverLeavesTruncatedFile(t *testing.T) {
	if dir := os.Getenv(writerEnv); dir != "" {
		writeForever(dir)
		return
	}
	dir := t.TempDir()
	store := newTestStore(dir)
	if err := store.Update(func(file *File) error {
		file.Runs = append(file.Runs, Run{ID: "seed", Prompt: strings.Repeat("x", 200_000)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for attempt := range 20 {
		cmd := exec.Command(os.Args[0], "-test.run=^TestInterruptedSaveNeverLeavesTruncatedFile$")
		cmd.Env = append(os.Environ(), writerEnv+"="+dir)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Duration(30+attempt*7) * time.Millisecond)
		cmd.Process.Signal(syscall.SIGKILL)
		cmd.Wait()

		data, err := os.ReadFile(filepath.Join(dir, "state.json"))
		if err != nil {
			t.Fatal(err)
		}
		var file File
		if err := json.Unmarshal(data, &file); err != nil {
			t.Fatalf("attempt %d: state is not valid JSON after a kill: %v", attempt, err)
		}
		if _, err := store.Load(); err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}
}

func writeForever(dir string) {
	store := newTestStore(dir)
	for i := 0; ; i++ {
		store.Update(func(file *File) error {
			file.Runs = append(file.Runs, Run{ID: fmt.Sprintf("r%d", i), Prompt: strings.Repeat("y", 50_000)})
			if len(file.Runs) > 20 {
				file.Runs = file.Runs[len(file.Runs)-20:]
			}
			return nil
		})
	}
}

func TestResolveByUniquePrefix(t *testing.T) {
	file := File{Runs: []Run{{ID: "fix-login"}, {ID: "fix-logout"}, {ID: "fix"}}}
	for ref, want := range map[string]string{"fix": "fix", "fix-logi": "fix-login", "fix-logo": "fix-logout"} {
		run, err := file.Resolve(ref)
		if err != nil || run.ID != want {
			t.Errorf("Resolve(%q) = %v, %v; want %s", ref, run, err, want)
		}
	}
	for _, ref := range []string{"fix-log", "nope", ""} {
		if _, err := file.Resolve(ref); err == nil {
			t.Errorf("Resolve(%q) should fail", ref)
		}
	}
}
