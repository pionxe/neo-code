---
title: Slash Commands
description: Local NeoCode commands for sessions, context, memory, skills, providers, and models.
---

# Slash Commands

Slash commands are local NeoCode control commands. They start with `/` and are handled by the terminal UI before normal chat input reaches the agent.

For example, `please explain /compact` is a normal prompt. `/compact` runs the local context compaction command.

## How to use them

Type `/` in the input box to see command suggestions. Continue typing to filter the list.

```text
/help
/cwd
/cwd /path/to/project
/remember I prefer reading tests before editing code
```

## Help and exit

| Command | Purpose |
|---|---|
| `/help` | Show available slash commands |
| `/clear` | Clear the current draft input |
| `/exit` | Exit NeoCode |

## Workspace, sessions, and context

| Command | Purpose |
|---|---|
| `/cwd` | Show the current workspace |
| `/cwd <path>` | Switch workspace |
| `/session` | Open the session picker |
| `/compact` | Compact a long session context |

Use `/compact` when an old conversation starts to distract the current task. After compacting, restate the current goal briefly.

## Memory

| Command | Purpose |
|---|---|
| `/memo` | Show saved memories |
| `/remember <text>` | Save a long-term preference or stable fact |
| `/forget <keyword>` | Delete matching memories |

Do not store secrets, tokens, passwords, or one-off task instructions in memory.

## Skills

| Command | Purpose |
|---|---|
| `/skills` | List available Skills and active marks |
| `/skill use <id>` | Activate a Skill in the current session |
| `/skill off <id>` | Deactivate a Skill in the current session |
| `/skill active` | Show active Skills |

Skills shape the current task workflow. They do not add tools or bypass approvals.

## Providers and models

| Command | Purpose |
|---|---|
| `/provider` | Open the provider picker |
| `/provider add` | Add a custom OpenAI-compatible provider |
| `/model` | Open the model picker |

Provider means the model service source. Model means the actual model used under that provider. API keys are read from environment variables, not stored in config files.

## Next steps

- Workspace and context: [Sessions, Context, and Workspace](./context-session-workspace)
- Project rules: [AGENTS.md Project Rules](./agents-md)
- Choosing between capabilities: [Capability Guide](./capability-choice)
