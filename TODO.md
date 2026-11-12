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

All four slices are done; the open items left are marked `[ ]` below and in "Ask the owner before".

| Area | State |
|---|---|
| `docs/design.md` | open questions settled and recorded (see "Settled design questions") |
| `internal/layout`, `config`, `state`, `hook`, `engine`, `plugin` | reviewed, fixed, tested |
| `internal/cli`, `cmd/muster` | slice 1 commands: `start`, `status`, `tick`, `watch`, `done`, `retry`, `rm`, `doctor`, `version` |
| Tests | unit tests per package plus an end-to-end test through the built binary |
| Presets | `grove-rex-pi` and `worktree-tmux-claude`, both verified live |
| Obsidian | `cmd/muster-obsidian`: `sync`, `watch`, `import-board` |
| README, license, skill, release config | written; snapshot release built and the cask installed locally; never published |

## Todo

Work through these in order. Each slice should end with the checks in "Definition of done" passing and a commit.

### 1. Finish slice 1: core run manager

- [x] Review the existing `internal/` code against `docs/design.md` before building on it; it was written by a subagent and never reviewed.
- [x] Resolve the design questions in "Open design questions" and update `docs/design.md`.
- [x] Add the command constructors called from `internal/cli/cli.go`: `start`, `status`, `tick`, `watch`, `done`, `retry`, `rm`, `doctor`, `version`.
- [x] Add output: `-o table|json`, the run table, the full run view, the transition list, and `"created"` in `start -o json`.
- [x] Add `cmd/muster/main.go`.
- [x] Write the tests listed in "Tests wanted".
- [x] Drive the built binary by hand against a temporary `MUSTER_DIR` with fake hooks: a run should go queued, running, waiting, done.

### 2. Slice 2: worker access, follow-ups, Rex preset

- [x] `muster read`, `muster send` (refuses when the agent is `blocked` unless `--force`), `muster follow`.
- [x] Dispatch of a later session: `last_report`, `stop_agent`, the follow-up prompt.
- [x] `presets/grove-rex-pi/`: `config.toml` plus hook scripts, using the Rex commands in "Rex contract".
- [x] Verify the preset live with a real pi agent and a trivial prompt, using a temporary `MUSTER_DIR` and a temporary directory as the workspace (not `gw create`, which runs a slow setup hook). Record which states pi reports at startup and after finishing, and whether `start_grace` is needed. Recorded under "Rex contract".
- [x] Confirm how to submit a message to pi's input through Rex: `rex send` with a trailing carriage return, or `rex send` followed by `rex send-key`. Both work; the preset uses `rex send -- TEXT` then `rex send-key enter`, which keeps a multi-line message as one.
- [x] `create-workspace.sh` ran against real `gw create`, with `HOME` pointed at a scratch directory so `~/.grove` was untouched: it created the workspace and printed its directory. `notify.sh` ran without error; whether the notification appeared on screen was not seen. pi was never seen `blocked`; the `blocked` state was verified with Claude Code instead (slice 4).

### 3. Slice 3: Obsidian source plugin

- [x] `cmd/muster-obsidian`: scan a folder of task notes with a `status` property; a note set to `now` becomes `muster start --source obsidian:<path>`; run status is written back to the note. Documented in `docs/obsidian.md`.
- [x] Decide where a follow-up instruction lives in a note, and map it to `muster follow`: a `follow_up` property, sent when `status` is `now` and the run has stopped, then removed.
- [x] Test on fixture notes only. Unit tests on fixtures plus an end-to-end test through both binaries; also driven by hand against a live pi agent with notes in a temporary folder.
- [ ] Try the Obsidian Bases Kanban view (native since Obsidian 1.14) on a copy of real notes: group by `status`, drag a card, confirm the frontmatter changes. Not done: it needs the Obsidian app driven by hand, which this session could not do. `docs/obsidian.md` has a `.base` file to try; the view type `kanban` and the `groupBy: {property, direction}` keys were read from Obsidian 1.15's bundled code.
- [x] Write a one-time converter from the owner's current Kanban board file to task notes: `muster obsidian import-board`. Tested on a fixture board only. Run it only with the owner's approval.
- [x] The converter was run on a copy of the owner's real board (67 cards). That showed dispatcher status lines ending up in prompts, Review cards losing their workspace and linked cards keeping a self-link; all three are fixed, see `docs/obsidian.md`.

### 4. Slice 4: release

- [x] Second preset: git worktree + tmux + Claude Code. Claude Code 2.1 does report OSC 7501 (`app: claude-code`), seen under Rex. tmux cannot report it, so the preset records state through Claude Code `--settings` hooks; those hooks were verified live. Codex is not installed here, so it was not checked.
- [x] Verify `presets/worktree-tmux-claude` with real tmux 3 and Claude Code 2.1: trust prompt reported as `blocked`, `send` refused while blocked, a real permission prompt reported as `blocked` and approved with `send --force`, `send`, `follow` (old session stopped, last report from the transcript), `gone` after killing the session. Trusting `MUSTER_DIR/worktrees` once covers new worktrees below it.
- [x] README, an agent skill (`skills/muster/SKILL.md`), a license (MIT), goreleaser (`.goreleaser.yaml`, `.github/workflows/release.yml`) and a Homebrew tap entry in the goreleaser config. CI in `.github/workflows/ci.yml`.
- [x] `goreleaser check` passes and `goreleaser release --snapshot` builds all four archives. goreleaser deprecated `brews`, so the config publishes a Homebrew cask. A quarantined unsigned binary hangs at launch under Gatekeeper, so the cask removes the quarantine attribute in `postflight`. The generated cask was installed from a throwaway local tap with `file://` URLs; both binaries ran. The tap repository `glundgren93/homebrew-tap` and the `HOMEBREW_TAP_GITHUB_TOKEN` secret do not exist yet.
- [x] `muster init [PRESET]` with presets embedded in the binary (`presets/presets.go`).

## Settled design questions

Decided in slice 1 and written into `docs/design.md`.

1. **`pick_model` sees the prompt.** A first session writes `runs/<id>/prompt.md` before `pick_model` runs.
2. **Empty `pick_model` output** falls back to `default_model`.
3. **`on_change`** fires on every transition, including creation and the move into `starting`. `on_<status>` exists only for `queued`, `running`, `waiting`, `done`, `failed`.
4. **Stale `starting` runs** are failed by every reconcile as well as every dispatch, so `muster status` repairs them; `doctor` only reports.
5. **Interrupts** fail the run with `<hook>: interrupted` and exit 130; `retry` queues it again. Event hooks still run to completion.

Also confirmed or changed during review: a failed start keeps its model and workspace; `finished_at` is set for `done` and `failed` and cleared on requeue; `updated_at` no longer moves on a repeated identical agent observation; `unknown` is never stored; a new session clears `agent_state`; `max_running = 0` pauses dispatch; a `last_report` failure is ignored like a `stop_agent` failure; a hook whose background child keeps stdout open counts as finished (it used to fail with `ErrWaitDelay`).

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

- **Go cannot download modules on the original machine.** The `go` on `PATH` there is a Socket Firewall shim. It assigns the Go binary's path to `GOPATH`, which breaks the module cache location, and Go does not trust the firewall's TLS certificate. Do not bypass the shim.
- **Workaround in use:** `export GOMODCACHE=$HOME/go/pkg/mod GOPROXY=off` before every go command. `go.sum` was resolved offline, and the four module hashes were checked against `sum.golang.org` by hand.
- On the machine slices 1 to 4 were built on (`~/git/muster`), `go` is Homebrew's Go 1.26 with no shim, and `BurntSushi/toml` was missing from the cache; one `go mod download github.com/BurntSushi/toml` through the default proxy fetched it and verified it against `go.sum`. The workaround above still works there.
- **Never create or edit `~/.muster` in tests or manual runs.** Set `MUSTER_DIR` to a temporary directory.

## Rex contract

Verified live against a throwaway session. Rex calls a session what muster calls a pane's container, and a block what muster calls a pane.

| Need | Command | Observed |
|---|---|---|
| Start | `rex new LABEL -c DIR --json -- COMMAND...` | JSON with `.session_id` and `.initial_windows[0].block_ids[0]`; the session is not shown in the app automatically |
| Status | `rex block call -s SESSION -b BLOCK program_status` | `{"records":[{"state":"working","app":"pi",...}]}`; the root record has `"id": null` |
| Read | `rex capture -s SESSION -b BLOCK --trim` | screen and scrollback as text |
| Submit | `rex send -s SESSION -b BLOCK -- TEXT` then `rex send-key -s SESSION -b BLOCK enter` | one message, newlines kept |
| Send | `rex send -s SESSION -b BLOCK TEXT` | raw input; a trailing newline submitted a line to a shell `read` |
| Stop | `rex kill SESSION` | exit 0 |
| Gone | any of the above after the kill | exit 3, `... not found` |

States come from the OSC 7501 program status protocol: `idle`, `working`, `done`, `blocked`, `error`. pi reports them under Rex without extra setup.

Observed in slice 2 with pi 1.5 started with a prompt argument: `records` is empty for about two seconds after launch (mapped to `unknown`, which muster ignores), then `working`, then `done` once pi has answered; `done` persists until the next message, which goes back to `working`. pi never reported `idle` in these runs, so `start_grace` is a safeguard rather than a need for this preset. `rex send TEXT` with a trailing `\r` submits, and so does `rex send TEXT` followed by `rex send-key enter`; `rex send -- TEXT` accepts text starting with `-`. `rex kill` of an already killed session exits 3 with `not found`.

Rex runs commands through the login shell but not interactive startup files unless `--interactive` is passed. The owner starts pi through a zsh function `pia` defined in `~/.zshrc`; plain `pi` has no model credentials on the owner's machine. The preset's `start_agent` takes the agent command as its argument, and `run-pi.sh` runs a name that is not on `PATH` through `$SHELL -ic`, so the owner's setup is `start-agent.sh pia`. Verified live with pi 1.5 under Rex.

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
- Making the repository public or publishing a release. (The owner approved the MIT license.)
- Adding dependencies or changing the module path (`github.com/glundgren93/muster`).

Prepared and waiting for the owner:

1. Install the binaries: `go install ./cmd/muster ./cmd/muster-obsidian` from the repository.
2. Set up `~/.muster`: `muster init grove-rex-pi`, then in `~/.muster/config.toml` change the grove preset name `backend` to `funnel-ai` and the agent command `pi` to `pia`.
3. Convert the board into a new folder and review it before moving anything: `muster obsidian import-board "<vault>/Agent board.md" --out "<vault>/Tasks-muster" --vault "<vault>" --workspaces ~/.grove/workspaces`.
4. Replace the launchd job: unload the old one if it exists (`launchctl unload ~/Library/LaunchAgents/com.gabriel.todo-dispatch.plist`), write the plist from `docs/obsidian.md`, and `launchctl load` it.
5. Try the Bases Kanban view with the `.base` file in `docs/obsidian.md`.
6. Before going public: delete this file, then `gh repo edit glundgren93/muster --visibility public --accept-visibility-change-consequences`.
7. Release: create `glundgren93/homebrew-tap`, add a `HOMEBREW_TAP_GITHUB_TOKEN` secret to this repository, then `git tag v0.1.0 && git push origin v0.1.0`.

## Prior art

`sortie-ai/sortie` is an open-source Go orchestrator that polls issue trackers and runs agents unattended. muster's distinguishing choices are attended agents in panes, shell hooks and external plugins, and local task sources. `nicksenap/gw-dispatch` creates a grove workspace and starts an agent in the current terminal, with no queue or tracking.
