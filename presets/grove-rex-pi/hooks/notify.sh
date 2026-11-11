#!/bin/sh
osascript -e 'on run argv' -e 'display notification (item 2 of argv) with title "muster" subtitle (item 1 of argv)' -e 'end run' \
	"$MUSTER_RUN_ID" "$MUSTER_TITLE" >/dev/null
