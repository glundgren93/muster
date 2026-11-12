package obsidian

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Card struct {
	Lane     string
	Text     string
	Complete bool
}

var (
	cardLine  = regexp.MustCompile(`^- \[([ xX])\] ?(.*)$`)
	wikiLink  = regexp.MustCompile(`\[\[([^\]|#]+)(?:#[^\]|]*)?(?:\|([^\]]+))?\]\]`)
	wholeLink = regexp.MustCompile(`^\[\[([^\]|#]+)(?:#[^\]|]*)?(?:\|[^\]]+)?\]\]$`)
	nonSlug   = regexp.MustCompile(`[^a-z0-9]+`)

	dispatcherStatus   = regexp.MustCompile(`^\s*- 🤖 (?:running|idle, check) · (?:[a-z]+ · )?([A-Za-z0-9][A-Za-z0-9._-]*)\s*$`)
	dispatcherFollowUp = regexp.MustCompile(`^(\s*- )(?:next|sent): ?`)
)

func ParseBoard(content string) ([]Card, error) {
	note := ParseNote(strings.ReplaceAll(content, "\r\n", "\n"))
	if plugin, _ := note.Get("kanban-plugin"); plugin == "" {
		return nil, errors.New("not a Kanban board: the kanban-plugin property is missing")
	}
	var cards []Card
	lane := ""
	var current *Card
	for line := range strings.Lines(note.Body()) {
		line = strings.TrimRight(line, "\n")
		switch {
		case strings.HasPrefix(line, "%% kanban:settings"):
			return cards, nil
		case strings.HasPrefix(line, "## "):
			lane = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			current = nil
		case line == "***":
			return cards, nil
		case lane == "":
		case cardLine.MatchString(line):
			match := cardLine.FindStringSubmatch(line)
			cards = append(cards, Card{Lane: lane, Text: match[2], Complete: match[1] != " "})
			current = &cards[len(cards)-1]
		case current != nil && (strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "  ")):
			current.Text += "\n" + strings.TrimPrefix(strings.TrimPrefix(line, "\t"), "    ")
		default:
			current = nil
		}
	}
	return cards, nil
}

func (c Card) Title() string {
	first, _, _ := strings.Cut(c.Text, "\n")
	title := wikiLink.ReplaceAllStringFunc(first, func(link string) string {
		match := wikiLink.FindStringSubmatch(link)
		if match[2] != "" {
			return match[2]
		}
		return filepath.Base(match[1])
	})
	title = strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|#^[]`, r) {
			return ' '
		}
		return r
	}, title)), " ")
	if len([]rune(title)) > 100 {
		title = strings.TrimSpace(string([]rune(title)[:100]))
	}
	return title
}

func (c Card) LinkedNote() string {
	first, _, _ := strings.Cut(c.Text, "\n")
	if match := wholeLink.FindStringSubmatch(strings.TrimSpace(first)); match != nil {
		return match[1]
	}
	return ""
}

func (c Card) withoutDispatcherLines() (text, workspaceName string) {
	lines := strings.Split(c.Text, "\n")
	kept := lines[:1:1]
	for _, line := range lines[1:] {
		if match := dispatcherStatus.FindStringSubmatch(line); match != nil {
			workspaceName = match[1]
			continue
		}
		kept = append(kept, line)
	}
	if workspaceName == "" {
		return c.Text, ""
	}
	for i, line := range kept {
		kept[i] = dispatcherFollowUp.ReplaceAllString(line, "$1")
	}
	return strings.Join(kept, "\n"), workspaceName
}

type ConvertOptions struct {
	Out        string
	Vault      string
	Workspaces string
	KeepNow    bool
}

type Converted struct {
	Path     string
	Status   string
	Replaces string
}

func LaneStatus(lane string, complete, keepNow bool) string {
	if complete {
		return StatusDone
	}
	status := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(lane), "-"), "-")
	if status == "" {
		status = "backlog"
	}
	if status == StatusNow && !keepNow {
		return "ready"
	}
	return status
}

func ConvertBoard(cards []Card, options ConvertOptions) ([]Converted, error) {
	if err := os.MkdirAll(options.Out, 0o755); err != nil {
		return nil, err
	}
	var converted []Converted
	used := map[string]bool{}
	for _, card := range cards {
		title := card.Title()
		if title == "" {
			continue
		}
		name := title
		for n := 2; used[strings.ToLower(name)]; n++ {
			name = fmt.Sprintf("%s %d", title, n)
		}
		used[strings.ToLower(name)] = true

		status := LaneStatus(card.Lane, card.Complete, options.KeepNow)
		text, workspaceName := card.withoutDispatcherLines()
		body, replaces := noteBody(text, card.LinkedNote(), options.Vault)
		note := Note{newline: "\n", body: body}
		note.Set(StatusKey, status)
		note.Set("lane", card.Lane)
		if workspace := existingWorkspace(options.Workspaces, workspaceName); workspace != "" {
			note.Set(WorkspaceKey, workspace)
		}

		path := filepath.Join(options.Out, name+".md")
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return converted, err
		}
		_, writeErr := file.WriteString(note.String())
		if err := errors.Join(writeErr, file.Close()); err != nil {
			return converted, err
		}
		converted = append(converted, Converted{Path: path, Status: status, Replaces: replaces})
	}
	return converted, nil
}

func noteBody(text, link, vault string) (body, replaces string) {
	text = strings.TrimSpace(text)
	if link != "" && vault != "" {
		if path, linked, err := readLinkedNote(vault, link); err == nil {
			if linkedBody := strings.TrimSpace(ParseNote(linked).Body()); linkedBody != "" {
				_, details, _ := strings.Cut(text, "\n")
				return "\n" + strings.TrimSpace(linkedBody+"\n\n"+strings.TrimSpace(details)) + "\n", path
			}
		}
	}
	return "\n" + text + "\n", ""
}

func existingWorkspace(root, name string) string {
	if root == "" || name == "" {
		return ""
	}
	path := filepath.Join(root, name)
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return ""
	}
	return path
}

func readLinkedNote(vault, link string) (path, content string, err error) {
	candidate := filepath.Join(vault, filepath.FromSlash(link))
	if !strings.HasSuffix(candidate, ".md") {
		candidate += ".md"
	}
	if data, err := os.ReadFile(candidate); err == nil {
		return candidate, string(data), nil
	}
	var found string
	base := filepath.Base(candidate)
	filepath.WalkDir(vault, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() && strings.HasPrefix(entry.Name(), ".") && path != vault {
			return filepath.SkipDir
		}
		if !entry.IsDir() && entry.Name() == base {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if found == "" {
		return "", "", os.ErrNotExist
	}
	data, err := os.ReadFile(found)
	return found, string(data), err
}
