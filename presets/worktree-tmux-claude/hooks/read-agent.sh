#!/bin/sh
set -eu
set -- $MUSTER_AGENT

tmux capture-pane -p -J -t "$2" -S "-$MUSTER_LINES" | tail -n "$MUSTER_LINES"
