#!/bin/sh
set -eu

repo=$1
base=${2:-HEAD}
worktree="$MUSTER_DIR/worktrees/$MUSTER_RUN_ID"

mkdir -p "$MUSTER_DIR/worktrees"
git -C "$repo" worktree add -b "muster/$MUSTER_RUN_ID" "$worktree" "$base" >&2
printf '%s\n' "$worktree"
