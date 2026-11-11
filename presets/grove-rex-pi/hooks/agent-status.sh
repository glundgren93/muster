#!/bin/sh
set -u
set -- $MUSTER_AGENT

if ! output=$(rex block call -s "$1" -b "$2" program_status 2>&1); then
	case $output in
	*"not found"*) echo gone; exit 0 ;;
	esac
	printf '%s\n' "$output" >&2
	exit 1
fi
printf '%s\n' "$output" | jq -r '[.records[] | select(.id == null) | .state][0] // "unknown"'
