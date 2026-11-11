# muster: handoff and todo

Working notes for whoever picks this up next. They describe one developer's machine and an unfinished build, so delete this file before the repository is made public.

## What this is

muster is a Go CLI that hands tasks to coding agents running in terminal panes, tracks each run until it needs a human, and lets the human follow up. It owns run state; everything specific to a terminal multiplexer, workspace tool, agent or task source lives in hooks and plugins.

`docs/design.md` is the contract. Read it in full before changing code. Where code and design disagree, fix one of them in the same change.

## Decisions already made by the owner

- Go, single binary, extended through shell hooks and `muster-<name>` plugin executables, modelled on grove (`nicksenap/grove`).
- Agents run in terminal panes the owner can attach to, not as headless processes.
- The tool owns run state in its own store. Tasks come from sources (first: an Obsidian folder) through plugins; the core has no task format.
- The owner's own setup is grove for workspaces, Rex for panes, pi as the agent.
- The name is `muster`.

## Current state

Paused mid-way through slice 1. Nothing below has been tested.

| Area | State |
|---|---|
| `docs/design.md` | written, reviewed by the owner only as a plan summary |
| `internal/layout`, `config`, `state`, `hook`, `engine`, `plugin` | written; each passes `go vet`; no tests |
| `internal/cli/cli.go` | does not compile: it calls nine command constructors that do not exist |
| `cmd/muster/main.go` | missing |
| Tests | none |
| Presets, README, license | missing |

The repository does not build as a whole. `go vet ./internal/<pkg>/` passes for the six packages listed.

## Todo

Work through these in order. Each slice should end with the checks in "Definition of done" passing and a commit.

### 1. Finish slice 1: core run manager

- [ ] Review the existing `internal/` code against `docs/design.md` before building on it; it was written by a subagent and never reviewed.
- [ ] Resolve the design questions in "Open design questions" and update `docs/design.md`.
- [ ] Add the command constructors called from `internal/cli/cli.go`: `start`, `status`, `tick`, `watch`, `done`, `retry`, `rm`, `doctor`, `version`.
- [ ] Add output: `-o table|json`, the run table, the full run view, the transition list, and `"created"` in `start -o json`.
- [ ] Add `cmd/muster/main.go`.
- [ ] Write the tests listed in "Tests wanted".
- [ ] Drive the built binary by hand against a temporary `MUSTER_DIR` with fake hooks: a run should go queued, running, waiting, done.

### 2. Slice 2: worker access, follow-ups, Rex preset

- [ ] `muster read`, `muster send` (refuses when the agent is `blocked` unless `--force`), `muster follow`.
- [ ] Dispatch of a later session: `last_report`, `stop_agent`, the follow-up prompt.
- [ ] `presets/grove-rex-pi/`: `config.toml` plus hook scripts, using the Rex commands in "Rex contract".
- [ ] Verify the preset live with a real pi agent and a trivial prompt, using a temporary `MUSTER_DIR` and a temporary directory as the workspace (not `gw create`, which runs a slow setup hook). Record which states pi reports at startup and after finishing, and whether `start_grace` is needed.
- [ ] Confirm how to submit a message to pi's input through Rex: `rex send` with a trailing carriage return, or `rex send` followed by `rex send-key`.

### 3. Slice 3: Obsidian source plugin

- [ ] `cmd/muster-obsidian`: scan a folder of task notes with a `status` property; a note set to `now` becomes `muster start --source obsidian:<path>`; run status is written back to the note.
- [ ] Decide where a follow-up instruction lives in a note, and map it to `muster follow`.
- [ ] Test on fixture notes only.
- [ ] Try the Obsidian Bases Kanban view (native since Obsidian 1.14) on a copy of real notes: group by `status`, drag a card, confirm the frontmatter changes. This has only been read about, not tried.
- [ ] Write a one-time converter from the owner's current Kanban board file to task notes. Run it only with the owner's approval.

### 4. Slice 4: release

- [ ] Second preset: git worktree + tmux + Claude Code. Check whether Claude Code and Codex report OSC 7501 status; if not, the preset needs another way to detect idle.
- [ ] README, an agent skill, a license, goreleaser and a Homebrew tap.
- [ ] Consider `muster init --preset <name>` with presets embedded in the binary.

## Open design questions

Raised by the subagent that wrote the code. Decide, then make code and design agree.

1. **`pick_model` cannot see the prompt.** The design runs it at dispatch step 2, before the prompt file is written at step 5. A model classifier needs the prompt. Likely fix: write the prompt file first.
2. **Empty `pick_model` output.** The code leaves the model empty; the design reads as "fall back to `default_model`".
3. **`on_change` scope.** The code fires it on every transition including into `starting` and on creation. The design lists event hooks for `queued`, `running`, `waiting`, `done`, `failed` and is silent on `starting`.
4. **Stale `starting` runs** are failed only during dispatch, so `status` and `doctor` show them without changing them. Confirm this is wanted.
5. **Interrupts.** Ctrl-C during a start marks the run `failed` with "interrupted". Confirm, or leave it `queued`.

Smaller choices the code made, to confirm during review: a failed start keeps the model and workspace it obtained, so `retry` does not create the workspace again; `finished_at` is set for `failed` as well as `done`; `updated_at` changes on every `agent_status` check; `unknown` is never stored as `agent_state`; `max_running = 0` is allowed.

## Tests wanted

Every test must protect a real failure mode. Pin the clock wherever time matters.

- Placeholder expansion: a value with a single quote, spaces and `;` reaches the hook intact and cannot inject a command; unknown `{braces}` are left alone.
- State: two concurrent writers do not lose an update; an interrupted save cannot leave a truncated file.
- Lifecycle with fake hook scripts: first start reaches `running`; `idle` inside `start_grace` without `seen_working` stays `running`; `idle` after `working` becomes `waiting`; `working` again returns it to `running`; `blocked` and `gone` become `waiting`; hook failure and `unknown` change nothing; `max_running` holds extra runs in `queued` and releases them in `queued_at` order; a failing `start_agent` yields `failed` with the stderr tail; `retry` requeues; a stale `starting` run becomes `failed`; event hooks fire once per transition in order; the same `--source` twice creates one run.
- Two overlapping passes do not start the same run twice.
- One end-to-end test through the real binary: `start`, `status -o json`, `tick`, `done`, and a plugin in `MUSTER_DIR/plugins`.

## Definition of done for a slice

```sh
export GOMODCACHE=$HOME/go/pkg/mod GOPROXY=off
gofmt -l .                      # prints nothing
go vet ./...
go build ./...
go test ./... -count=1
go test ./... -race -count=1
```

## Code rules

- No code comments, including doc comments. Names and structure carry the meaning.
- Typed constants for statuses, agent states and hook names.
- Standard library plus cobra and BurntSushi/toml only. No new dependencies without the owner's agreement.
- Abstractions proportional to need; nothing beyond the design.
- macOS and Linux only.

## Local environment

- **Go cannot download modules on this machine.** The `go` on `PATH` is a Socket Firewall shim. It assigns the Go binary's path to `GOPATH`, which breaks the module cache location, and Go does not trust the firewall's TLS certificate. Do not bypass the shim.
- **Workaround in use:** `export GOMODCACHE=$HOME/go/pkg/mod GOPROXY=off` before every go command. The two dependencies were already in the local module cache. `go.sum` was resolved offline, and the four module hashes were checked against `sum.golang.org` by hand.
- **Never create or edit `~/.muster` in tests or manual runs.** Set `MUSTER_DIR` to a temporary directory.

## Rex contract

Verified live against a throwaway session. Rex calls a session what muster calls a pane's container, and a block what muster calls a pane.

| Need | Command | Observed |
|---|---|---|
| Start | `rex new LABEL -c DIR --json -- COMMAND...` | JSON with `.session_id` and `.initial_windows[0].block_ids[0]`; the session is not shown in the app automatically |
| Status | `rex block call -s SESSION -b BLOCK program_status` | `{"records":[{"state":"working","app":"pi",...}]}`; the root record has `"id": null` |
| Read | `rex capture -s SESSION -b BLOCK --trim` | screen and scrollback as text |
| Send | `rex send -s SESSION -b BLOCK TEXT` | raw input; a trailing newline submitted a line to a shell `read` |
| Stop | `rex kill SESSION` | exit 0 |
| Gone | any of the above after the kill | exit 3, `... not found` |

States come from the OSC 7501 program status protocol: `idle`, `working`, `done`, `blocked`, `error`. pi reports them under Rex without extra setup. Not yet observed: what pi reports right after launch and after finishing a task, and what an empty `records` list should map to (probably `unknown`).

Rex runs commands through the login shell but not interactive startup files unless `--interactive` is passed. The owner starts pi through a zsh function `pia` defined in `~/.zshrc`, so the owner's personal start command needs `--interactive` or an explicit `zsh -ic`.

## The system muster replaces

Read these for behaviour to preserve; do not modify them without asking.

| Path | What it is |
|---|---|
| `~/.local/bin/todo-dispatch` | Python dispatcher: starts an agent per card moved to "Now" on a Kanban markdown board, tracks idle, handles follow-ups, picks a model tier |
| `~/Library/LaunchAgents/com.gabriel.todo-dispatch.plist` | runs the dispatcher on board change and every 120s |
| `~/.pi/agent/extensions/desk.ts` | pi tools for listing, adding and moving cards and for reading or prompting a worker |
| `~/.pi/agent/prompts/task.md`, `morning.md` | prompt templates that add cards |
| `~/Documents/Obsidian Vault/Agent board.md`, `Tasks/` | the board and its task notes |

The dispatcher still calls herdr and has failed on every run since the owner moved to Rex, so moving a card to "Now" currently does nothing.

## Ask the owner before

- Creating or changing `~/.muster`, the Obsidian vault, the launchd job, or the files in the table above.
- Making the repository public, adding a license, or publishing a release.
- Adding dependencies or changing the module path (`github.com/glundgren93/muster`).

## Prior art

`sortie-ai/sortie` is an open-source Go orchestrator that polls issue trackers and runs agents unattended. muster's distinguishing choices are attended agents in panes, shell hooks and external plugins, and local task sources. `nicksenap/gw-dispatch` creates a grove workspace and starts an agent in the current terminal, with no queue or tracking.
