package cli

import (
	"slices"
	"testing"
)

func TestTextStartingWithADashIsNotParsedAsFlags(t *testing.T) {
	for name, test := range map[string]struct{ args, want []string }{
		"bullet message": {
			args: []string{"send", "run-1", "- do this\n- then that", "--force"},
			want: []string{"send", "run-1", "--force", "--", "- do this\n- then that"},
		},
		"rule line as the prompt": {
			args: []string{"start", "--- see below", "--queue"},
			want: []string{"start", "--queue", "--", "--- see below"},
		},
		"value of a flag stays with its flag": {
			args: []string{"start", "--title", "- a title", "task"},
			want: []string{"start", "--title", "- a title", "task"},
		},
		"value after a boolean flag is text": {
			args: []string{"follow", "run-1", "--queue", "- more"},
			want: []string{"follow", "run-1", "--queue", "--", "- more"},
		},
		"explicit separator is left alone": {
			args: []string{"send", "run-1", "--", "- text"},
			want: []string{"send", "run-1", "--", "- text"},
		},
		"stdin marker is left alone": {
			args: []string{"start", "-f", "-"},
			want: []string{"start", "-f", "-"},
		},
		"commands without text are left alone": {
			args: []string{"status", "- x"},
			want: []string{"status", "- x"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := textLast(newRootCommand(), test.args); !slices.Equal(got, test.want) {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}
