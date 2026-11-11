#!/bin/sh
set -u
set -- $MUSTER_AGENT

output=$(rex kill "$1" 2>&1) && exit 0
case $output in
*"not found"*) exit 0 ;;
esac
printf '%s\n' "$output" >&2
exit 1
