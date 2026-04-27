---
title: Tools & Permissions
description: What the agent can do and how to choose Allow, Ask, or Deny.
---

# Tools & Permissions

NeoCode uses tools to interact with your project. Read-only actions usually run automatically. File writes, edits, and risky commands ask for approval.

## What the agent can do

| Capability | Tool | Usually asks? |
|---|---|---|
| Read files | `filesystem_read_file` | No |
| Search file content | `filesystem_grep` | No |
| Search file paths | `filesystem_glob` | No |
| Write files | `filesystem_write_file` | Yes |
| Edit files | `filesystem_edit` | Yes |
| Run commands | `bash` | Depends on risk |
| Fetch web pages | `webfetch` | No |
| Manage task list | `todo_write` | No |
| Manage memory | `memo_*` | No |
| Start subagents | `spawn_subagent` | No |

MCP tools use names like `mcp.<server-id>.<tool>`. See [MCP Tools](./mcp).

## Approval choices

```text
◆ NEO wants to run: filesystem_write_file
  path: src/main.go
  content: (428 bytes)

  [Allow] [Ask] [Deny]
```

| Choice | Meaning | Best for |
|---|---|---|
| Allow | Approve and remember the same decision | Confirmed safe repeated operations |
| Ask | Keep asking next time | Default for most tasks |
| Deny | Block this action | Wrong path, risky command, or uncontrolled scope |

## How to decide

| Scenario | Recommendation |
|---|---|
| Reading and searching files | Usually allow |
| Small code or test edits | Ask, then allow after checking paths |
| Existing test command | Usually allow |
| Deletes, Git reset, broad rewrites | Ask for explanation first |
| Secrets or local config | Deny |

## Full Access

`Ctrl+F` opens the Full Access risk prompt. When enabled, tool approvals are auto-approved.

::: warning
Use Full Access only when you understand the task risk, trust the workspace, and accept file or command side effects.
:::

## Command risk

| Category | Examples | Handling |
|---|---|---|
| Read-only | `git status`, `git log`, `ls` | Auto-allow |
| Local changes | `git commit`, `go build` | Ask |
| Remote interaction | `git push`, `git fetch` | Ask |
| Destructive | `git reset --hard`, `rm` | Ask |
| Unknown | Compound commands, parse failures | Ask |

## File scope

File operations are limited to the current workspace by default.

```text
/cwd
```

## Next steps

- Daily workflow: [Daily Use](./daily-use)
- Slash commands: [Slash Commands](./slash-commands)
- External tools: [MCP Tools](./mcp)
