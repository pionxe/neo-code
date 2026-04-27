---
title: AGENTS.md Project Rules
description: Use AGENTS.md to give NeoCode stable project-level rules.
---

# AGENTS.md Project Rules

`AGENTS.md` is a project rules file for the agent. NeoCode starts at the current workspace directory, searches upward for files named exactly `AGENTS.md`, and includes them as project rules.

Use it for stable rules the agent should follow across tasks: code style, test expectations, safety boundaries, documentation language, and module responsibilities.

## Good content

| Good use | Example |
|---|---|
| Project goal | This repository implements a terminal AI coding assistant |
| Code style | Format Go code with `gofmt` |
| Tests | Run `go test ./...` after Go changes |
| Safety | Do not write API keys to config files |
| Docs language | Keep Chinese docs in Chinese |

## Avoid

| Avoid | Why |
|---|---|
| Secrets or tokens | They should not enter the repository or model context |
| One-off task requirements | Put them in the current conversation |
| Long design docs | They consume context; link or reference them when needed |
| Rules that conflict with code | They mislead the agent |

## Minimal template

Create `AGENTS.md` at the repository root:

```md
# Project Rules

- Run `go test ./...` after changing Go code
- Keep Chinese documentation in Chinese
- Do not write API keys to config files
- Put model-callable capabilities in the tools layer first
```

## Where to put it

Put general project rules at the repository root. When the workspace is the repository root or one of its child directories, NeoCode can discover that root file by searching upward.

If a subdirectory needs more specific rules, add another `AGENTS.md` there. NeoCode will see it when the workspace is that subdirectory or one of its child directories.

Keep rules short, concrete, and actionable.

## Next steps

- Slash commands: [Slash Commands](./slash-commands)
- Sessions and context: [Sessions, Context, and Workspace](./context-session-workspace)
- Choosing capabilities: [Capability Guide](./capability-choice)
