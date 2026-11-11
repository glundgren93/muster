#!/bin/sh
set -eu

session="muster-$MUSTER_RUN_ID"
run_dir=$(dirname "$MUSTER_PROMPT_FILE")
rm -f "$run_dir/claude-state"

pane=$(tmux new-session -d -P -F '#{pane_id}' -s "$session" -c "$MUSTER_WORKSPACE" \
	"$MUSTER_DIR/hooks/run-claude.sh" "$MUSTER_PROMPT_FILE" "${MUSTER_MODEL:-}")
printf '%s %s\n' "$session" "$pane"
