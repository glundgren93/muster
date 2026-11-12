package obsidian

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportBoardFixture(t *testing.T) {
	board, err := os.ReadFile("testdata/board.md")
	if err != nil {
		t.Fatal(err)
	}
	cards, err := ParseBoard(string(board))
	if err != nil {
		t.Fatal(err)
	}
	var lanes []string
	for _, card := range cards {
		lanes = append(lanes, card.Lane+": "+card.Title())
	}
	want := []string{
		"Backlog: Refactor the auth module",
		"Backlog: Upgrade Go to 1.26",
		"Next: Write release notes",
		"Now: Fix login 500",
		"Now: Fix login 500",
		"Waiting on review: Review PR 42 for the billing team",
		"Done: Ship v1",
	}
	if strings.Join(lanes, "\n") != strings.Join(want, "\n") {
		t.Fatalf("cards:\n%s\nwant:\n%s", strings.Join(lanes, "\n"), strings.Join(want, "\n"))
	}

	out := filepath.Join(t.TempDir(), "Tasks")
	converted, err := ConvertBoard(cards, ConvertOptions{Out: out, Vault: "testdata/vault"})
	if err != nil {
		t.Fatal(err)
	}
	if len(converted) != len(cards) {
		t.Fatalf("converted %d of %d", len(converted), len(cards))
	}
	for name, want := range map[string]string{
		"Refactor the auth module.md":          "---\nstatus: backlog\nlane: Backlog\n---\n\nSplit the session code out of handlers.\n",
		"Fix login 500.md":                     "---\nstatus: ready\nlane: Now\n---\n\nFix login 500\nThe stack trace is in Sentry.\n- check the cookie domain\n",
		"Fix login 500 2.md":                   "---\nstatus: ready\nlane: Now\n---\n\nFix login 500\n",
		"Review PR 42 for the billing team.md": "---\nstatus: waiting-on-review\nlane: Waiting on review\n---\n\nReview [[PR 42|PR 42 for the billing team]]\n",
		"Ship v1.md":                           "---\nstatus: done\nlane: Done\n---\n\nShip v1\n",
	} {
		got, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("%s:\n%s\nwant:\n%s", name, got, want)
		}
	}
	if _, err := ConvertBoard(cards, ConvertOptions{Out: out}); err == nil {
		t.Fatal("converting into a folder with the same notes should refuse to overwrite")
	}
}

func TestKeepNow(t *testing.T) {
	if got := LaneStatus("Now", false, true); got != "now" {
		t.Fatalf("got %s", got)
	}
	if got := LaneStatus("Now", false, false); got != "ready" {
		t.Fatalf("got %s", got)
	}
}

func TestNotABoard(t *testing.T) {
	if _, err := ParseBoard("## Lane\n- [ ] card\n"); err == nil {
		t.Fatal("want an error for a note without kanban-plugin")
	}
}

func TestDispatcherLinesBecomeAWorkspaceInsteadOfPromptText(t *testing.T) {
	board := "---\nkanban-plugin: board\n---\n\n## Review\n\n" +
		"- [ ] [[Tasks/Refactor the auth module]]\n\t- sent: also cover the logout path\n\t- 🤖 idle, check · opus · todo-refactor-auth\n" +
		"- [ ] Fix the export\n\t- 🤖 running · todo-gone\n" +
		"- [ ] Ship the preview\n\t- next: call Bo\n\t- 🤖 Builder 1 ✅ draft PR 7 · branch feat/preview\n"
	cards, err := ParseBoard(board)
	if err != nil {
		t.Fatal(err)
	}
	workspaces := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspaces, "todo-refactor-auth"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "Tasks")
	converted, err := ConvertBoard(cards, ConvertOptions{Out: out, Vault: "testdata/vault", Workspaces: workspaces})
	if err != nil {
		t.Fatal(err)
	}

	workspace := filepath.Join(workspaces, "todo-refactor-auth")
	for name, want := range map[string]string{
		"Refactor the auth module.md": "---\nstatus: review\nlane: Review\nworkspace: " + formatScalar(workspace) + "\n---\n\nSplit the session code out of handlers.\n\n- also cover the logout path\n",
		"Fix the export.md":           "---\nstatus: review\nlane: Review\n---\n\nFix the export\n",
		"Ship the preview.md":         "---\nstatus: review\nlane: Review\n---\n\nShip the preview\n- next: call Bo\n- 🤖 Builder 1 ✅ draft PR 7 · branch feat/preview\n",
	} {
		got, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("%s:\n%s\nwant:\n%s", name, got, want)
		}
	}
	if got, _ := ParseNote(readNote(t, filepath.Join(out, "Refactor the auth module.md"))).Get(WorkspaceKey); got != workspace {
		t.Errorf("workspace reads back as %q, want %q", got, workspace)
	}
	if want := filepath.Join("testdata/vault", "Tasks", "Refactor the auth module.md"); converted[0].Replaces != want || converted[1].Replaces != "" {
		t.Errorf("replaces %q and %q, want %q and none", converted[0].Replaces, converted[1].Replaces, want)
	}
}
