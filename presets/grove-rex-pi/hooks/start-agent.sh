#!/bin/sh
set -eu

set -- "$MUSTER_DIR/hooks/run-pi.sh" "$MUSTER_PROMPT_FILE"
if [ -n "${MUSTER_MODEL:-}" ]; then
	set -- "$@" "$MUSTER_MODEL"
fi

created=$(rex new "muster-$MUSTER_RUN_ID" -c "$MUSTER_WORKSPACE" --json -- "$@")
printf '%s\n' "$created" | jq -er '"\(.session_id) \(.initial_windows[0].block_ids[0])"'
