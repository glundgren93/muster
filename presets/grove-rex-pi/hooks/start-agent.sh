#!/bin/sh
set -eu

if [ "$#" -eq 0 ]; then
	set -- pi
fi
if [ -n "${MUSTER_MODEL:-}" ]; then
	set -- "$@" --model "$MUSTER_MODEL"
fi

created=$(rex new "muster-$MUSTER_RUN_ID" -c "$MUSTER_WORKSPACE" --json -- "$MUSTER_DIR/hooks/run-pi.sh" "$MUSTER_PROMPT_FILE" "$@")
printf '%s\n' "$created" | jq -er '"\(.session_id) \(.initial_windows[0].block_ids[0])"'
