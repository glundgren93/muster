#!/bin/sh
set -eu
set -- $MUSTER_AGENT

screen=$(rex capture -s "$1" -b "$2" --trim)
printf '%s\n' "$screen" | tail -n "$MUSTER_LINES"
