#!/bin/sh
set -u
set -- $MUSTER_AGENT

if ! screen=$(tmux capture-pane -p -t "$2" 2>/dev/null); then
	echo gone
	exit 0
fi
case $screen in
*"Do you trust the files in this folder"* | *"Is this a project you created or one you trust"*)
	echo blocked
	exit 0
	;;
esac
state_file="$MUSTER_DIR/runs/$MUSTER_RUN_ID/claude-state"
if [ -s "$state_file" ]; then
	cat "$state_file"
else
	echo unknown
fi
