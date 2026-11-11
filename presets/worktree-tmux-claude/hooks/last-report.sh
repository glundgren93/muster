#!/bin/sh
set -u

transcript_file="$MUSTER_DIR/runs/$MUSTER_RUN_ID/claude-transcript"
if [ -s "$transcript_file" ]; then
	transcript=$(cat "$transcript_file")
	if [ -f "$transcript" ]; then
		report=$(jq -rs '[.[] | select(.type == "assistant") | .message.content[]? | select(.type == "text") | .text] | last // empty' "$transcript")
		if [ -n "$report" ]; then
			printf '%s\n' "$report"
			exit 0
		fi
	fi
fi

set -- $MUSTER_AGENT
tmux capture-pane -p -J -t "$2" -S -40 2>/dev/null
