package cli

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
)

const takesText = "takes-text"

func markTakesText(command *cobra.Command) {
	command.Annotations = map[string]string{takesText: "true"}
}

func textLast(root *cobra.Command, args []string) []string {
	if len(args) == 0 || slices.Contains(args, "--") {
		return args
	}
	command := builtinCommand(root, args[0])
	if command == nil || command.Annotations[takesText] == "" {
		return args
	}
	kept := []string{args[0]}
	var text []string
	for i, arg := range args[1:] {
		if cannotBeAFlag(arg) && !expectsValue(command, args[i]) {
			text = append(text, arg)
			continue
		}
		kept = append(kept, arg)
	}
	if len(text) == 0 {
		return args
	}
	return append(append(kept, "--"), text...)
}

func cannotBeAFlag(arg string) bool {
	afterDashes := strings.TrimLeft(arg, "-")
	return afterDashes != arg && afterDashes != "" && unicode.IsSpace([]rune(afterDashes)[0])
}

func expectsValue(command *cobra.Command, previous string) bool {
	if !strings.HasPrefix(previous, "-") || strings.Contains(previous, "=") || cannotBeAFlag(previous) {
		return false
	}
	lookup, name := command.Flags().ShorthandLookup, previous[len(previous)-1:]
	if long, isLong := strings.CutPrefix(previous, "--"); isLong {
		lookup, name = command.Flags().Lookup, long
	}
	flag := lookup(name)
	return flag != nil && flag.NoOptDefVal == ""
}

func hintAtDoubleDash(command *cobra.Command, err error) error {
	if command.Annotations[takesText] == "" {
		return err
	}
	return fmt.Errorf("%w\nif the text starts with -, put -- before it: muster %s ... -- \"-text\"", err, command.Name())
}
