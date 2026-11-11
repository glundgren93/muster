#!/bin/sh
set -eu

cd_file=$(mktemp)
trap 'rm -f "$cd_file"' EXIT

GROVE_CD_FILE=$cd_file gw create "$MUSTER_RUN_ID" -b "$MUSTER_RUN_ID" -p "$1" \
	--source-provider muster --source-ref "$MUSTER_RUN_ID" --source-title "$MUSTER_TITLE" >&2

workspace=$(cat "$cd_file")
if [ -z "$workspace" ]; then
	echo "gw create did not report the workspace directory" >&2
	exit 1
fi
printf '%s\n' "$workspace"
