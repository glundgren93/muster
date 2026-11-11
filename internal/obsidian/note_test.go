package obsidian

import (
	"strings"
	"testing"
)

const sampleNote = `---
status: now
title: "Fix the \"login\" page"
quoted: 'it''s here'
tags:
  - work
  - auth
aliases:
- one
follow_up: |
  Also add a test.
  And update the changelog.
folded: >-
  one
  two
plain: some text # a comment
empty:
---
# Body

Keep **everything** here exactly.
---
Not frontmatter.
`

func TestGetReadsScalarForms(t *testing.T) {
	note := ParseNote(sampleNote)
	for key, want := range map[string]string{
		"status":    "now",
		"title":     `Fix the "login" page`,
		"quoted":    "it's here",
		"follow_up": "Also add a test.\nAnd update the changelog.\n",
		"folded":    "one two",
		"plain":     "some text",
		"empty":     "",
		"tags":      "",
	} {
		got, ok := note.Get(key)
		if !ok || got != want {
			t.Errorf("%s = %q, %v; want %q", key, got, ok, want)
		}
	}
	if _, ok := note.Get("missing"); ok {
		t.Error("missing key found")
	}
}

func TestEditsKeepEverythingElseByteForByte(t *testing.T) {
	note := ParseNote(sampleNote)
	note.Set("status", "running")
	note.Set("run", "fix-the-login-page")
	note.Delete("follow_up")
	got := note.String()
	want := strings.Replace(sampleNote, "status: now", "status: running", 1)
	want = strings.Replace(want, "follow_up: |\n  Also add a test.\n  And update the changelog.\n", "", 1)
	want = strings.Replace(want, "empty:\n---\n", "empty:\nrun: fix-the-login-page\n---\n", 1)
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	reparsed := ParseNote(got)
	if tags, _ := reparsed.Get("aliases"); tags != "" || !strings.Contains(got, "aliases:\n- one\n") {
		t.Fatal("list under aliases damaged")
	}
}

func TestUnchangedNoteRoundTrips(t *testing.T) {
	for _, content := range []string{sampleNote, "no frontmatter\n", "---\nstatus: x\r\n---\r\nbody\r\n", "---\nunterminated: yes\n", ""} {
		if got := ParseNote(content).String(); got != content {
			t.Errorf("round trip changed %q to %q", content, got)
		}
	}
}

func TestSetAddsFrontmatterAndQuotesUnsafeValues(t *testing.T) {
	note := ParseNote("Just a body\n")
	note.Set("status", "now")
	note.Set("run", "yes")
	note.Set("model", "anthropic/claude: big")
	want := "---\nstatus: now\nrun: \"yes\"\nmodel: \"anthropic/claude: big\"\n---\nJust a body\n"
	if got := note.String(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if model, _ := ParseNote(want).Get("model"); model != "anthropic/claude: big" {
		t.Fatalf("model read back as %q", model)
	}
}

func TestCRLFNotesKeepTheirLineEndings(t *testing.T) {
	note := ParseNote("---\r\nstatus: now\r\n---\r\nbody\r\n")
	note.Set("status", "queued")
	if got := note.String(); got != "---\r\nstatus: queued\r\n---\r\nbody\r\n" {
		t.Fatalf("got %q", got)
	}
}
