#!/bin/sh
set -u

workspace=$(cd "$MUSTER_WORKSPACE" 2>/dev/null && pwd -P) || workspace=$MUSTER_WORKSPACE
sessions="$HOME/.pi/agent/sessions/--$(printf '%s' "$workspace" | sed 's|^/||; s|/|-|g')--"
latest=$(ls -t "$sessions"/*.jsonl 2>/dev/null | head -n 1)
if [ -n "$latest" ]; then
	report=$(jq -rs '[.[] | select(.type == "message" and .message.role == "assistant") | .message.content[]? | select(.type == "text") | .text] | last // empty' "$latest")
	if [ -n "$report" ]; then
		printf '%s\n' "$report"
		exit 0
	fi
fi

set -- $MUSTER_AGENT
rex capture -s "$1" -b "$2" --trim | tail -n 40
