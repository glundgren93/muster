#!/bin/sh
set -eu
set -- $MUSTER_AGENT

tmux send-keys -t "$2" -l -- "$MUSTER_MESSAGE"
tmux send-keys -t "$2" Enter
