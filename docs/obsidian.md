# Obsidian source

`muster-obsidian` turns a folder of Obsidian notes into a task source. It is a plugin: install the binary as `muster-obsidian` on `PATH` or in `MUSTER_DIR/plugins`, and run it as `muster obsidian`.

## Task notes

A task note is any Markdown file in the folder, or below it, with a `status` property. Folders starting with `.` are skipped. Other notes are left alone.

```markdown
---
status: now
model: anthropic/claude-sonnet-5-5
workspace: ~/git/muster
---
The login page returns 500 when the session cookie has expired.
Find out why and fix it.
```

| Property | Set by | Meaning |
|---|---|---|
| `status` | you, then muster | `now` asks for work, `done` closes the run, in any letter case; muster writes the run's status back |
| `run` | muster | the id of the note's run |
| `follow_up` | you | text for the next agent session, sent when `status` is `now` |
| `model` | you, optional | passed as `muster start --model` |
| `workspace` | you, optional | an existing directory, passed as `muster start --workspace`; `~/` is expanded |

Any other `status` value (`backlog`, `next`, `ready`, ...) is yours, and the note is ignored until it has a run.

The run's title is the note's file name, and its prompt is the file name followed by the note's body. Its source is `obsidian:<path relative to the folder>`, so syncing twice never starts two runs; a renamed note keeps its run through the `run` property.

## What a sync does

For each task note:

| Note | Run | Action |
|---|---|---|
| `status: now` | none | `muster start`; the note gets `run` and the run's status. A `follow_up` is added to the end of the prompt and removed |
| `status: now` with `follow_up` | `waiting`, `done`, `failed` or `queued` | `muster follow` with the text; `follow_up` is removed |
| `status: now` with `follow_up` | `running` or `starting` | nothing yet; the note keeps `now` until the run stops |
| `status: now` | `failed` | `muster retry` |
| `status: done` | not `done` | `muster done` |
| anything | any | `status` is set to the run's status |

A note is only rewritten when a property changes, and only if it has not changed since the sync read it; otherwise the next sync tries again. Only the `status`, `run` and `follow_up` lines change; the rest of the file stays byte for byte. If `muster start` fails before a run exists, the note keeps `now` and the error is printed.

So the run's status wins: moving a card with a running agent to another column puts it back on the next sync. To stop tracking a note, `muster rm` its run and remove `run` and `status` from the note.

## Commands

```sh
muster obsidian sync --dir ~/Vault/Tasks      # one pass
muster obsidian watch --dir ~/Vault/Tasks     # muster tick, then sync, every 15s
muster obsidian import-board ~/Vault/Board.md --out ~/Vault/Tasks-new --vault ~/Vault
```

`--dir` defaults to `$MUSTER_OBSIDIAN_DIR`. `watch` replaces `muster watch`; run one or the other.

`import-board` converts a board from the Kanban community plugin into task notes in a new, empty folder, and never changes the board. A card's lane becomes its `status` (lower case, other characters turned into `-`), a checked card becomes `done`, and the archive is skipped. Cards in a lane called Now become `ready`, so the first sync does not start them all; pass `--keep-now` to keep `now`. The original lane is kept in a `lane` property. A card that is only a `[[link]]` gets the linked note's body, looked up in `--vault`, followed by the card's other lines. The new note has the same name as the linked one, so the command lists the linked notes; delete them once the new notes look right.

Cards that were handled by the `todo-dispatch` script carry a status line such as `- 🤖 idle, check · opus · NAME`. These lines are left out, and the `next:` and `sent:` prefixes on those cards are removed so the text stays. With `--workspaces DIR`, a card whose status line names an existing directory `DIR/NAME` gets `workspace: DIR/NAME`, so setting the note to `now` starts an agent where the earlier one worked. Add a `follow_up` first to tell it what is left to do.

## A board in Obsidian

Obsidian's Bases (1.14 or later) can show the notes as a Kanban board grouped by `status`. Save this as `Tasks.base` next to the folder and adjust the folder name:

```yaml
filters:
  and:
    - file.inFolder("Tasks")
views:
  - type: kanban
    name: Board
    groupBy:
      property: note.status
      direction: ASC
```

Dragging a card to another column should set its `status`, which the next sync acts on. This has not been tried yet; the view type and `groupBy` keys were read from Obsidian 1.15's code.

## Running it in the background on macOS

A launchd agent keeps `watch` running. Write it to `~/Library/LaunchAgents/com.muster.obsidian.plist`, then `launchctl load` it:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.muster.obsidian</string>
  <key>ProgramArguments</key>
  <array>
    <string>/bin/zsh</string><string>-lc</string>
    <string>exec muster obsidian watch --dir "$HOME/Vault/Tasks"</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>/tmp/muster-obsidian.log</string>
  <key>StandardErrorPath</key><string>/tmp/muster-obsidian.log</string>
</dict>
</plist>
```

The login shell (`zsh -l`) gives the job the `PATH` that `rex`, `pi`, `gw` and `jq` need.
