# grove + Rex + pi

Workspaces from [grove](https://github.com/nicksenap/grove) (`gw`), agent panes in Rex, and [pi](https://github.com/badlogic/pi-mono) as the agent.

Requires `gw`, `rex`, `pi` and `jq` on `PATH`. The notification hook uses `osascript`, so it is macOS only; replace `hooks/notify.sh` elsewhere.

## Install

```sh
mkdir -p ~/.muster
cp -R presets/grove-rex-pi/. ~/.muster/
muster doctor
```

Then edit `~/.muster/config.toml`:

- `create_workspace` passes `backend` as the grove preset. Change it to one of yours (`gw preset list`), or remove the hook to work in `~/.muster/workspaces/<id>`. `muster start --workspace DIR` skips it for one run.
- `hooks/run-pi.sh` starts `pi`. Rex runs it through your login shell without interactive startup files, so a shell function from `~/.zshrc` is not available there. To use one, replace the `exec pi ...` lines with something like `exec zsh -ic 'pia "$@"' pia "$prompt"`.

## How it maps

| Hook | Script | Rex or pi call |
|---|---|---|
| `create_workspace` | `create-workspace.sh PRESET` | `gw create ID -b ID -p PRESET`, directory read from `GROVE_CD_FILE` |
| `start_agent` | `start-agent.sh` | `rex new muster-ID -c WORKSPACE --json -- run-pi.sh PROMPT_FILE [MODEL]`; the handle is `SESSION BLOCK` |
| `agent_status` | `agent-status.sh` | `rex block call program_status`; the root record's state, `unknown` when there is none yet, `gone` when Rex reports not found |
| `read_agent` | `read-agent.sh` | `rex capture --trim`, last `N` lines |
| `send_agent` | `send-agent.sh` | `rex send -- TEXT`, then `rex send-key enter` |
| `stop_agent` | `stop-agent.sh` | `rex kill SESSION`; an already gone session is fine |
| `last_report` | `last-report.sh` | the last assistant text in pi's newest session file for the workspace, else the last 40 lines of the pane |
| `on_waiting` | `notify.sh` | a macOS notification |

Rex does not show a new session in the app on its own; open it from the session list (`rex ls`).

## Observed behaviour

Checked live with pi 1.5 and Rex on macOS:

- pi publishes no program status for about two seconds after launch (`unknown`, ignored), then `working` while it processes the prompt, then `done` when it has answered. It does not report `idle` at startup when started with a prompt, so `start_grace` only matters if pi is started without one.
- A message is submitted with `rex send TEXT` followed by `rex send-key enter`. A trailing carriage return in `rex send` also submits; the two-step form keeps multi-line messages as one message.
- After `rex kill`, every Rex call exits 3 with `... not found`, which `agent-status.sh` reports as `gone`.
