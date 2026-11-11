# muster

muster hands tasks to coding agents running in terminal panes, tracks each run until it needs you, and lets you follow up.

You can attach to every agent: it runs in a pane of your terminal multiplexer, not as a hidden background process. muster keeps the run state (queued, running, waiting for you, done) and leaves everything specific to your tools to shell hooks: how a workspace is created, how an agent is started, how its state is read. Tasks can come from the command line or from a source plugin, such as a folder of Obsidian notes.

```text
$ muster start "Fix the flaky TestCheckout and explain the cause"
ID                                      STATUS   AGENT  UPDATED  TITLE
fix-the-flaky-testcheckout-and-explain  running  -      0s       Fix the flaky TestCheckout and explain the cause

$ muster status
ID                                      STATUS   AGENT    UPDATED  TITLE
fix-the-flaky-testcheckout-and-explain  waiting  done     2m       Fix the flaky TestCheckout and explain the cause
add-rate-limiting-to-the-api            running  working  5m       Add rate limiting to the API

$ muster follow fix-the "Also add a regression test"
```

## Install

With Go 1.25 or later:

```sh
go install github.com/glundgren93/muster/cmd/muster@latest
go install github.com/glundgren93/muster/cmd/muster-obsidian@latest   # optional Obsidian source
```

muster runs on macOS and Linux.

## Set up

muster keeps its configuration and state in `~/.muster`, or in `$MUSTER_DIR` when set. Start from a preset:

```sh
muster init                        # list the presets
muster init grove-rex-pi           # copy one into ~/.muster
muster doctor                      # check the configuration and hooks
```

| Preset | Workspace | Panes | Agent |
|---|---|---|---|
| [`grove-rex-pi`](presets/grove-rex-pi) | grove workspace | Rex | pi |
| [`worktree-tmux-claude`](presets/worktree-tmux-claude) | git worktree | tmux | Claude Code |

A preset is a `config.toml` plus the hook scripts it calls. Edit both to fit your setup; the preset READMEs say what to change.

## Use

```sh
muster start "TASK"              # create a run and start an agent for it
muster start -f task.md --workspace ~/src/app --model opus
muster status                    # runs that need attention first; --all includes done ones
muster status RUN                # one run in full
muster read RUN                  # the agent's recent output
muster send RUN "yes, go ahead"  # type into the live agent
muster follow RUN "now add tests"  # a new agent session in the same workspace, with context
muster done RUN                  # close the run
muster retry RUN                 # requeue a run whose agent failed to start
muster rm RUN                    # forget a run
muster watch                     # keep checking agents and starting queued runs
```

`RUN` is a run id or a unique prefix of one. Every command that prints runs takes `-o json`.

At most `max_running` agents run at once; the rest wait in the queue. `muster watch`, or `muster tick` from a scheduler, checks each agent's state and starts queued runs as slots free up. A run moves to `waiting` when its agent stops working, and an `on_waiting` hook can notify you.

## Hooks

Hooks are shell commands in `config.toml`. muster fills in placeholders such as `{id}`, `{workspace}` and `{agent}` with shell-quoted values and also exports them as `MUSTER_*` environment variables.

```toml
max_running = 4

[hooks]
create_workspace = "git -C ~/src/app worktree add -b muster/{id} ~/.muster/worktrees/{id} >&2 && echo ~/.muster/worktrees/{id}"
start_agent = "~/.muster/hooks/start-agent.sh"
agent_status = "~/.muster/hooks/agent-status.sh"
on_waiting = "notify-send muster {title}"
```

Only `start_agent` and `agent_status` are required. `agent_status` prints one of `working`, `idle`, `done`, `blocked`, `error`, `gone` or `unknown`; terminals that support the OSC 7501 program status protocol, such as Rex, report these directly.

[docs/design.md](docs/design.md) is the full contract: the run lifecycle, every hook and placeholder, concurrency, and the commands.

## Plugins and sources

`muster NAME` runs an executable `muster-NAME` from `~/.muster/plugins` or `PATH`. Plugins read state through `muster status -o json` and change it through muster commands.

`muster-obsidian` uses a folder of Obsidian notes as a task source: set a note's `status` to `now` and the next sync starts a run for it, then writes the run's status back to the note, so an Obsidian Bases board grouped by `status` shows your agents. See [docs/obsidian.md](docs/obsidian.md).

## For agents

[skills/muster/SKILL.md](skills/muster/SKILL.md) teaches a coding agent to drive muster: start and check runs, read and answer agents, follow up and close runs. Install it wherever your agent loads skills, for example `~/.claude/skills/muster/` for Claude Code.

## License

MIT; see [LICENSE](LICENSE).
