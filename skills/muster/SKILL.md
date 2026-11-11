---
name: muster
description: Hand tasks to other coding agents and check on them with the muster CLI. Use when the user asks to start, queue or delegate a task to an agent, to list or check on agent runs ("what are my agents doing", "which runs need me"), to read what an agent wrote, to answer or nudge a running agent, to give a finished run a follow-up, or to close a run.
---

# muster

muster starts coding agents in terminal panes, one run per task, and tracks each run until it needs a human. You drive it through the `muster` command. Never edit `state.json` or anything else in the muster directory (`$MUSTER_DIR`, default `~/.muster`) by hand.

## Look before acting

```sh
muster status -o json            # runs that are not done, after asking each agent
muster status RUN -o json        # one run in full: prompt, follow-ups, error, workspace
muster read RUN --lines 80       # the agent's recent screen output
```

`RUN` is a run id or a unique prefix of one. A run's `status` is one of:

| status | meaning | what the user usually wants |
|---|---|---|
| `queued` | waiting for a free slot | nothing |
| `starting`, `running` | the agent is working | leave it alone |
| `waiting` | the agent stopped; `agent_state` says why (`done`, `blocked`, `idle`, `error`, `gone`) | a human look, a follow-up or `done` |
| `failed` | muster could not start the agent; `error` says why | fix the cause, then `retry` |
| `done` | closed by a human | nothing |

`agent_state: blocked` means the agent is waiting on a question or a permission prompt. Read its screen and tell the user what it is asking; do not answer for them unless they told you to.

## Act

```sh
muster start "TASK TEXT" --title "Short title"   # new run, dispatched now
muster start -f task.md --workspace ~/src/repo   # prompt from a file, existing directory
muster send RUN "MESSAGE"                         # type into the live agent and press enter
muster follow RUN "MESSAGE"                       # new agent session in the same workspace, with context
muster done RUN                                   # close the run
muster retry RUN                                  # requeue a failed run
```

- Write a prompt that stands on its own: the agent sees only the prompt text and its workspace.
- Use `send` for a short answer to a live agent. It refuses when the agent is `blocked`; pass `--force` only when the user wants that prompt answered with your text.
- Use `follow` when the run is `waiting` or `done` and needs more work. The new session gets the original prompt, earlier follow-ups and the previous agent's last answer.
- `done` does not stop the agent or delete the workspace; it only closes the run.
- Add `-o json` to any of these to get the run record back.

## When something is wrong

`muster doctor` checks the configuration and hooks and exits non-zero on a problem. Hook failures and every transition are logged in `$MUSTER_DIR/muster.log`.
