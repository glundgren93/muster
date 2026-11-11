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
		"Refactor the auth module.md":          "---\nstatus: backlog\nlane: Backlog\n---\n\n[[Tasks/Refactor the auth module]]\n\nSplit the session code out of handlers.\n",
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
