#!/bin/sh
prompt=$(cat "$1")
shift
set -- "$@" "$prompt"

if command -v "$1" >/dev/null 2>&1; then
	exec "$@"
fi
exec "${SHELL:-/bin/sh}" -ic '"$@"' "$1" "$@"
