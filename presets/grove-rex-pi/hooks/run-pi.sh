#!/bin/sh
prompt=$(cat "$1")
if [ -n "${2:-}" ]; then
	exec pi --model "$2" "$prompt"
fi
exec pi "$prompt"
