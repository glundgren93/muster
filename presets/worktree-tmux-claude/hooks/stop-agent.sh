#!/bin/sh
set -u
set -- $MUSTER_AGENT

tmux kill-session -t "=$1" 2>/dev/null
exit 0
