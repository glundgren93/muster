package engine

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/glundgren93/muster/internal/state"
)

const (
	titleLimit    = 80
	slugWordLimit = 6
	slugCharLimit = 48
	fallbackSlug  = "run"
)

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

type StartRequest struct {
	Prompt    string
	Title     string
	ID        string
	Source    string
	Model     string
	Workspace string
	Queue     bool
}

type StartResult struct {
	Run     state.Run
	Created bool
}

func (e *Engine) Start(ctx context.Context, request StartRequest) (StartResult, error) {
	if strings.TrimSpace(request.Prompt) == "" {
		return StartResult{}, errors.New("the prompt is empty")
	}
	title := request.Title
	if title == "" {
		title = defaultTitle(request.Prompt)
	}
	if strings.ContainsAny(title, "\r\n") {
		return StartResult{}, errors.New("the title must be one line")
	}
	if request.ID != "" && !idPattern.MatchString(request.ID) {
		return StartResult{}, fmt.Errorf("id %q must match %s", request.ID, idPattern)
	}
	workspace := ""
	if request.Workspace != "" {
		absolute, err := filepath.Abs(request.Workspace)
		if err != nil {
			return StartResult{}, fmt.Errorf("workspace %s: %w", request.Workspace, err)
		}
		if !isDir(absolute) {
			return StartResult{}, fmt.Errorf("workspace %s is not an existing directory", absolute)
		}
		workspace = absolute
	}

	var result StartResult
	var changes []change
	err := e.store.Update(func(file *state.File) error {
		if request.Source != "" {
			if existing := file.FindSource(request.Source); existing != nil {
				result = StartResult{Run: *existing}
				return nil
			}
		}
		id := request.ID
		if id == "" {
			id = uniqueID(file, slug(title))
		} else if file.Find(id) != nil {
			return fmt.Errorf("a run with id %s already exists", id)
		}
		now := e.timestamp()
		run := state.Run{
			ID:        id,
			Title:     title,
			Prompt:    request.Prompt,
			Source:    request.Source,
			Model:     request.Model,
			Workspace: workspace,
			Status:    state.Queued,
			FollowUps: []state.FollowUp{},
			CreatedAt: now,
			QueuedAt:  now,
			UpdatedAt: now,
		}
		file.Runs = append(file.Runs, run)
		changes = append(changes, change{transition: Transition{ID: id, To: state.Queued}, run: run})
		result = StartResult{Run: run, Created: true}
		return nil
	})
	if err != nil {
		return StartResult{}, err
	}
	if !result.Created {
		return result, nil
	}
	e.publish(ctx, changes)
	if request.Queue {
		return result, nil
	}
	if _, err := e.Dispatch(ctx); err != nil {
		return result, err
	}
	run, err := e.Find(result.Run.ID)
	if err != nil {
		return result, err
	}
	result.Run = run
	return result, nil
}

func defaultTitle(prompt string) string {
	for line := range strings.Lines(prompt) {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return truncateRunes(trimmed, titleLimit)
		}
	}
	return ""
}

func truncateRunes(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	return string([]rune(text)[:limit])
}

func slug(title string) string {
	words := strings.FieldsFunc(strings.ToLower(title), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	if len(words) > slugWordLimit {
		words = words[:slugWordLimit]
	}
	joined := strings.Join(words, "-")
	if len(joined) > slugCharLimit {
		joined = strings.TrimRight(joined[:slugCharLimit], "-")
	}
	if joined == "" {
		return fallbackSlug
	}
	return joined
}

func uniqueID(file *state.File, base string) string {
	if file.Find(base) == nil {
		return base
	}
	for n := 2; ; n++ {
		candidate := base + "-" + strconv.Itoa(n)
		if file.Find(candidate) == nil {
			return candidate
		}
	}
}
