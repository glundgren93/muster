#!/bin/sh
prompt=$(cat "$1")
MUSTER_AGENT_DIR=$(dirname "$1")
export MUSTER_AGENT_DIR

record() {
	printf '{"type": "command", "command": "printf %s > \\"$MUSTER_AGENT_DIR/claude-state\\"%s"}' "$1" "$2"
}
transcript=' && jq -r .transcript_path > \"$MUSTER_AGENT_DIR/claude-transcript\"'
settings=$(cat <<JSON
{"hooks": {
  "UserPromptSubmit": [{"hooks": [$(record working "")]}],
  "PreToolUse": [{"hooks": [$(record working "")]}],
  "PostToolUse": [{"hooks": [$(record working "")]}],
  "Notification": [{"matcher": "permission_prompt|elicitation_dialog", "hooks": [$(record blocked "")]}],
  "Stop": [{"hooks": [$(record done "$transcript")]}]
}}
JSON
)

if [ -n "${2:-}" ]; then
	exec claude --settings "$settings" --model "$2" "$prompt"
fi
exec claude --settings "$settings" "$prompt"
