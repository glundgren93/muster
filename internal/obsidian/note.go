package obsidian

import (
	"regexp"
	"strconv"
	"strings"
)

type Note struct {
	front    []string
	body     string
	hasFront bool
	newline  string
}

var keyLine = regexp.MustCompile(`^([A-Za-z0-9_][A-Za-z0-9_ -]*?)\s*:(?:\s+(.*?))?\s*$`)

func ParseNote(content string) Note {
	newline := "\n"
	if strings.Contains(content, "\r\n") {
		newline = "\r\n"
	}
	lines := strings.Split(content, newline)
	if len(lines) < 2 || strings.TrimRight(lines[0], " \t") != "---" {
		return Note{body: content, newline: newline}
	}
	for i := 1; i < len(lines); i++ {
		if end := strings.TrimRight(lines[i], " \t"); end == "---" || end == "..." {
			return Note{
				front:    append([]string{}, lines[1:i]...),
				body:     strings.Join(lines[i+1:], newline),
				hasFront: true,
				newline:  newline,
			}
		}
	}
	return Note{body: content, newline: newline}
}

func (n Note) String() string {
	if !n.hasFront {
		return n.body
	}
	return "---" + n.newline + joinLines(n.front, n.newline) + "---" + n.newline + n.body
}

func joinLines(lines []string, newline string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, newline) + newline
}

func (n Note) Body() string { return n.body }

func (n Note) HasFrontmatter() bool { return n.hasFront }

func (n Note) entry(key string) (start, end int, rest string, found bool) {
	for i, line := range n.front {
		match := keyLine.FindStringSubmatch(line)
		if match == nil || match[1] != key {
			continue
		}
		end = i + 1
		for end < len(n.front) && isContinuation(n.front[end]) {
			end++
		}
		for end > i+1 && strings.TrimSpace(n.front[end-1]) == "" {
			end--
		}
		return i, end, match[2], true
	}
	return 0, 0, "", false
}

func isContinuation(line string) bool {
	return line == "" || line[0] == ' ' || line[0] == '\t' || line == "-" || strings.HasPrefix(line, "- ") || strings.TrimSpace(line) == ""
}

func (n Note) Get(key string) (string, bool) {
	start, end, rest, found := n.entry(key)
	if !found {
		return "", false
	}
	return scalar(rest, n.front[start+1:end]), true
}

func scalar(rest string, continuation []string) string {
	switch {
	case strings.HasPrefix(rest, "|") || strings.HasPrefix(rest, ">"):
		return blockScalar(rest, continuation)
	case strings.HasPrefix(rest, `"`):
		return doubleQuoted(joinFolded(rest, continuation))
	case strings.HasPrefix(rest, "'"):
		return singleQuoted(joinFolded(rest, continuation))
	case rest == "" && len(continuation) > 0 && strings.HasPrefix(strings.TrimSpace(continuation[0]), "- "):
		return ""
	}
	value := joinFolded(rest, continuation)
	if i := strings.Index(value, " #"); i >= 0 {
		value = value[:i]
	}
	value = strings.TrimSpace(value)
	if value == "~" || value == "null" {
		return ""
	}
	return value
}

func joinFolded(rest string, continuation []string) string {
	parts := []string{rest}
	for _, line := range continuation {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func doubleQuoted(value string) string {
	if unquoted, err := strconv.Unquote(value); err == nil {
		return unquoted
	}
	return strings.Trim(value, `"`)
}

func singleQuoted(value string) string {
	if len(value) >= 2 && strings.HasSuffix(value, "'") {
		value = value[1 : len(value)-1]
	}
	return strings.ReplaceAll(value, "''", "'")
}

func blockScalar(header string, lines []string) string {
	indent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		width := len(line) - len(strings.TrimLeft(line, " \t"))
		if indent < 0 || width < indent {
			indent = width
		}
	}
	dedented := make([]string, len(lines))
	for i, line := range lines {
		if len(line) >= indent && indent >= 0 {
			dedented[i] = line[indent:]
		} else {
			dedented[i] = strings.TrimSpace(line)
		}
	}
	text := strings.Join(dedented, "\n")
	if strings.HasPrefix(header, ">") {
		text = foldParagraphs(dedented)
	}
	text = strings.TrimRight(text, "\n")
	if strings.Contains(header, "-") {
		return text
	}
	return text + "\n"
}

func foldParagraphs(lines []string) string {
	var paragraphs []string
	var current []string
	for _, line := range lines {
		if line == "" {
			paragraphs = append(paragraphs, strings.Join(current, " "))
			current = nil
			continue
		}
		current = append(current, line)
	}
	paragraphs = append(paragraphs, strings.Join(current, " "))
	return strings.Join(paragraphs, "\n")
}

func (n *Note) Set(key, value string) {
	line := key + ": " + formatScalar(value)
	if start, end, _, found := n.entry(key); found {
		n.front = append(append(n.front[:start:start], line), n.front[end:]...)
		return
	}
	n.hasFront = true
	n.front = append(n.front, line)
}

func (n *Note) Delete(key string) {
	if start, end, _, found := n.entry(key); found {
		n.front = append(n.front[:start:start], n.front[end:]...)
	}
}

var plainScalar = regexp.MustCompile(`^[A-Za-z]([A-Za-z0-9._/ -]*[A-Za-z0-9._/-])?$`)

var yamlKeywords = map[string]bool{"true": true, "false": true, "yes": true, "no": true, "on": true, "off": true, "null": true, "y": true, "n": true}

func formatScalar(value string) string {
	if plainScalar.MatchString(value) && !yamlKeywords[strings.ToLower(value)] {
		return value
	}
	return strconv.Quote(value)
}
