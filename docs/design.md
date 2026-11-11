# muster design

muster hands tasks to coding agents that run in terminal panes, tracks each run until it needs a human, and lets the human follow up. It owns run state. Everything specific to a terminal multiplexer, a workspace tool, an agent, or a task source lives outside the core, in hooks and plugins.

This document is the contract for the implementation. Where the code and this document disagree, fix one of them in the same change.

## Concepts

- **Run**: one task handed to an agent, with its workspace, agent session, status and follow-ups.
- **Agent session**: one agent process started for a run. A run has one live session at a time; a follow-up starts a new one in the same workspace.
- **Hook**: a shell command configured by the user that muster calls at a defined point. Some hooks return a value on stdout.
- **Plugin**: an external executable named `muster-<name>`, run as `muster <name>`.
- **Source**: where tasks come from (an Obsidian folder, an issue tracker). Sources are plugins that call `muster start` and read run state back. The core has no task format.

## Directory

`MUSTER_DIR` selects the directory; the default is `~/.muster`.

| Path | Purpose |
|---|---|
| `config.toml` | settings and hooks |
| `state.json` | run records |
| `state.lock` | lock file for state mutations |
| `muster.log` | transitions and hook failures, one timestamped line each |
| `runs/<id>/prompt.md` | the prompt for the run's current agent session |
| `workspaces/<id>/` | default workspace when no `create_workspace` hook is configured |
| `plugins/` | plugin executables, searched before `PATH` |

## Configuration

```toml
max_running = 5
interval = "10s"
start_grace = "60s"
default_model = ""

[hooks]
create_workspace = "gw create -b {id} -p backend >&2 && echo ~/.grove/workspaces/{id}"
on_waiting = "terminal-notifier -title muster -message {title}"

[hooks.start_agent]
command = "~/.muster/hooks/start-agent.sh"
timeout = "2m"
```

| Setting | Default | Meaning |
|---|---|---|
| `max_running` | `5` | cap on runs in `starting` or `running` |
| `interval` | `10s` | time between passes in `muster watch` |
| `start_grace` | `60s` | how long a new session may report `idle` before the run counts as waiting |
| `default_model` | empty | model used when neither `--model` nor `pick_model` supplies one |

A hook is a command string, or a table with `command` and an optional `timeout` (a Go duration). A missing `config.toml` means defaults and no hooks. Unknown settings and unknown hook names are errors, so typos surface.

## Run record

`state.json` holds `{"version": 1, "runs": [...]}`. Field names are the JSON keys.

| Field | Meaning |
|---|---|
| `id` | unique slug, `[a-z0-9][a-z0-9-]*` |
| `title` | one line |
| `prompt` | the original task text |
| `source` | opaque reference set by a source plugin, optional |
| `model` | model passed to the agent, may be empty |
| `workspace` | absolute directory, set at first start |
| `agent` | opaque handle returned by `start_agent` |
| `status` | see Lifecycle |
| `agent_state` | last value from `agent_status` |
| `agent_state_at` | when `agent_state` was last observed |
| `seen_working` | whether the current session has reported `working` |
| `error` | why the run is `failed` |
| `follow_ups` | list of `{text, created_at, sent_at}`; `sent_at` is absent until a session received it |
| `sessions` | number of agent sessions started |
| `created_at`, `queued_at`, `starting_at`, `started_at`, `updated_at`, `finished_at` | RFC 3339 UTC, omitted when unset |

The title defaults to the first non-empty line of the prompt, cut to 80 characters. The id defaults to a slug of the title: lower case, runs of other characters become `-`, first six words, at most 48 characters, with `-2`, `-3`, ... appended until unique.

## Lifecycle

| Status | Meaning |
|---|---|
| `queued` | wants an agent session and is waiting for a slot |
| `starting` | a muster process has claimed it and is running start hooks |
| `running` | the agent is working |
| `waiting` | the agent stopped and a human should look; `agent_state` says why |
| `done` | the human closed the run |
| `failed` | muster could not start an agent session; `error` says why |

Transitions:

- `muster start` creates a run as `queued`.
- Dispatch moves `queued` to `starting`, then to `running` on success or `failed` on any start-hook error.
- Reconcile moves a run between `running` and `waiting` from the agent's reported state.
- `muster follow` moves `waiting`, `done` or `failed` to `queued`.
- `muster retry` moves `failed` to `queued`.
- `muster done` moves any status except `starting` to `done`.
- A run left in `starting` for longer than the sum of the start-hook timeouts plus one minute becomes `failed` with the error `start interrupted`.

### Reconcile

For every run in `running` or `waiting` with an `agent` handle, call `agent_status` and apply:

| Agent state | Result |
|---|---|
| `working` | `running`; set `seen_working` |
| `blocked`, `done`, `error`, `gone` | `waiting` |
| `idle` | `waiting` if `seen_working` or the session is older than `start_grace`; otherwise unchanged |
| `unknown`, or the hook fails | unchanged; log the failure |

`agent_state` and `agent_state_at` are stored on every successful observation. The state names are those of the OSC 7501 program status protocol plus `gone` (the pane no longer exists) and `unknown`.

### Dispatch

While fewer than `max_running` runs are `starting` or `running`, take the `queued` run with the oldest `queued_at` and start a session:

1. Claim it: set `starting` and `starting_at`.
2. If `sessions` is 0 and `model` is empty, set the model from `pick_model`, else `default_model`.
3. If `workspace` is empty, set it from `create_workspace`, or create `workspaces/<id>` when the hook is not configured. A workspace that is set but no longer exists is an error.
4. If `sessions` is above 0: call `last_report` when configured, then `stop_agent` when configured and `agent` is set. A `stop_agent` failure is logged and ignored.
5. Write `runs/<id>/prompt.md`. For a first session this is the prompt. For a later one it is the follow-up prompt below.
6. Call `start_agent` and store the handle.
7. Set `running`, `started_at`, `seen_working = false`, increment `sessions`, and mark the included follow-ups sent.

Any error in steps 2 to 6 sets `failed` with the hook name and the last lines of its stderr.

Follow-up prompt:

```
<original prompt>

---

An earlier agent session already worked on this task in this workspace.

Earlier follow-ups:
- <each follow-up already sent>

Its last report:
<output of last_report>

Follow-up:
<each pending follow-up, one per line>
```

The "Earlier follow-ups" and "Its last report" blocks are omitted when empty.

### Concurrency

State changes are read-modify-write under an exclusive lock on `state.lock`, saved by writing a temporary sibling and renaming it. The lock is never held while a hook runs. A process claims a run before running its hooks, and applies a result only if the run is still in the status, and has the same `agent` handle or `starting_at`, that it had when the hook was started. Two processes running passes at once therefore never start a run twice or apply a transition twice.

## Hooks

Hooks run through `sh -c` with the working directory set to the run's workspace when it exists, otherwise to `MUSTER_DIR`.

Placeholders are replaced with a single-quoted shell-safe value. A placeholder with no value expands to `''`. Text in braces that is not a placeholder name is left alone. Every value is also exported as an environment variable.

| Placeholder | Environment | Value |
|---|---|---|
| `{id}` | `MUSTER_RUN_ID` | run id |
| `{title}` | `MUSTER_TITLE` | run title |
| `{workspace}` | `MUSTER_WORKSPACE` | workspace directory |
| `{agent}` | `MUSTER_AGENT` | agent handle |
| `{model}` | `MUSTER_MODEL` | model |
| `{source}` | `MUSTER_SOURCE` | source reference |
| `{prompt_file}` | `MUSTER_PROMPT_FILE` | path to the session prompt |
| `{status}` | `MUSTER_STATUS` | run status |
| `{previous}` | `MUSTER_PREVIOUS_STATUS` | status before the transition |
| `{message}` | `MUSTER_MESSAGE` | text for `send_agent` |
| `{lines}` | `MUSTER_LINES` | line count for `read_agent` |

`MUSTER_DIR` and `MUSTER_BIN` (the muster executable) are always set.

Hooks that return a value. The value is the last non-empty line of stdout, trimmed, unless stated otherwise. A non-zero exit or a timeout is a failure.

| Hook | Required | Called | Returns | Default timeout |
|---|---|---|---|---|
| `pick_model` | no | first dispatch, when no model is set | model name | 2m |
| `create_workspace` | no | first dispatch, when no workspace is set | absolute path of an existing directory | 10m |
| `start_agent` | yes | every dispatch | agent handle, non-empty | 2m |
| `agent_status` | yes | reconcile, `send`, `status` | `working`, `blocked`, `idle`, `done`, `error`, `gone` or `unknown` | 15s |
| `read_agent` | for `read` | `muster read` | all of stdout, passed through | 30s |
| `send_agent` | for `send` | `muster send` | nothing | 30s |
| `stop_agent` | no | dispatch of a later session | nothing | 30s |
| `last_report` | no | dispatch of a later session | all of stdout | 30s |

Event hooks fire after a transition is saved: first `on_change`, then `on_<status>` for `queued`, `running`, `waiting`, `done` or `failed`. Their output is ignored and a failure is logged without affecting the run. Default timeout 30s.

## Commands

Every command that prints runs accepts `-o table` (default) or `-o json`. A `RUN` argument is a run id or a unique prefix of one.

| Command | Behaviour |
|---|---|
| `muster start [TEXT]` | Create a run and dispatch. Flags: `-f FILE` reads the prompt from a file (`-` for stdin), `--title`, `--id`, `--source`, `--model`, `--workspace DIR` (an existing directory, skips `create_workspace`), `--queue` (create only, leave dispatch to a later pass). If a run with the same `--source` exists, print it and change nothing. |
| `muster status [RUN]` | Reconcile, then list runs that are not `done`, ordered `waiting`, `failed`, `running`, `starting`, `queued`. `--all` includes `done`. `--cached` skips the reconcile. With `RUN`, show that run in full. |
| `muster tick` | One reconcile and dispatch pass. Prints each transition. |
| `muster watch` | Run passes every `interval` until interrupted. `--interval` overrides the setting. |
| `muster read RUN` | Print the agent's recent output through `read_agent`. `--lines N`, default 80. |
| `muster send RUN MESSAGE` | Send text to the live agent through `send_agent`. Refuses when the agent reports `blocked`, unless `--force`. |
| `muster follow RUN MESSAGE` | Add a follow-up, queue the run and dispatch. Requires at least one earlier session and a status of `waiting`, `done`, `failed` or `queued`. `--queue` as for `start`. |
| `muster done RUN` | Mark the run done. |
| `muster retry RUN` | Queue a failed run and dispatch. |
| `muster rm RUN` | Delete the record and `runs/<id>`. Refuses for `starting` or `running` unless `--force`. Leaves the workspace and the agent alone. |
| `muster doctor` | Check the configuration, required hooks and state file, and report runs stuck in `starting`. Exits non-zero on a problem. |
| `muster version` | Print the version. |

JSON output for a run is its record. `start`, `follow`, `retry` and `done` print the affected run; `start -o json` adds `"created": true` or `false`. `tick -o json` prints a list of `{id, from, to}`.

## Plugins

When the first argument is not a built-in command, muster looks for an executable `muster-<name>` in `MUSTER_DIR/plugins`, then on `PATH`, and replaces itself with it, passing the remaining arguments. The plugin's environment gains `MUSTER_DIR`, `MUSTER_CONFIG`, `MUSTER_STATE` and `MUSTER_BIN`. Plugins read state through `muster status -o json` and change it through muster commands, never by editing `state.json`.

## Presets

A preset is a `config.toml` plus the scripts its hooks call, kept under `presets/<name>/`. A user copies one into `MUSTER_DIR` and edits it. Presets are examples, not part of the core.

## Out of scope for v1

Issue-tracker sources, a built-in model classifier, a TUI, Windows, and plugin installation commands.
