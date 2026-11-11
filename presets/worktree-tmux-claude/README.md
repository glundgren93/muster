# git worktree + tmux + Claude Code

Each run gets a git worktree of one repository on a branch `muster/<id>`, a detached tmux session `muster-<id>`, and Claude Code as the agent.

Requires `git`, `tmux`, `claude` and `jq` on `PATH`.

## Install

```sh
muster init worktree-tmux-claude
muster doctor
```

Then edit `config.toml` in `MUSTER_DIR`:

- `create_workspace` names the repository, `~/git/my-project`. Change it to yours. A second argument picks the base ref (default `HEAD` of that repository). Worktrees go to `MUSTER_DIR/worktrees/<id>`; remove one with `git worktree remove` when you are done with it.
- Claude Code asks whether to trust a folder it has not seen. Until someone answers, the run shows `blocked`; attach with `tmux attach -t muster-<id>` to answer. The prompt's default choice is "No, exit", so do not answer it with a bare Enter. Trusting `MUSTER_DIR/worktrees` once, by starting `claude` there, covers every worktree below it.

## How Claude Code reports its state

tmux has no command that reports the OSC 7501 program status Claude Code prints, so `run-claude.sh` starts Claude Code with `--settings` hooks that write the state to `runs/<id>/claude-state` in `MUSTER_DIR`:

| Claude Code hook | State |
|---|---|
| `UserPromptSubmit`, `PreToolUse`, `PostToolUse` | `working` |
| `Notification` for a permission prompt or a question | `blocked` |
| `Stop` | `done`; the transcript path is saved to `runs/<id>/claude-transcript` |

`agent-status.sh` reports `gone` when the tmux pane no longer exists, `blocked` while the trust prompt is on screen, the recorded state otherwise, and `unknown` before the first hook has run. `last-report.sh` prints the last assistant text from the saved transcript, or the last 40 lines of the pane.

## Verified

With tmux 3 and Claude Code 2.1, in a temporary `MUSTER_DIR`:

- The trust prompt shows as `blocked`, and `muster send` refuses while it is on screen.
- A real permission prompt shows as `blocked`; `muster send --force RUN 1` approves it and the run goes back to `running`, then `waiting` with `done`.
- `muster send` to an idle agent, `muster follow` (the old tmux session is stopped and the new prompt carries the last report from the transcript), and `gone` after the tmux session is killed.
- Claude Code also prints OSC 7501 status (`app: claude-code`) under Rex, so the grove + Rex preset works with Claude Code too: replace `pi` in its `run-pi.sh`.
