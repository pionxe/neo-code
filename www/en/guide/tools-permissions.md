---
title: Tools & Permissions
description: What the agent can do, when it needs your approval, and how to choose.
---

# Tools & Permissions

## What the agent can do

The agent interacts with your project through tools. Read-only operations run automatically; writes and commands need your approval.

| Capability | Tool | Needs approval |
|---|---|---|
| Read files | `filesystem_read_file` | No |
| Search file contents | `filesystem_grep` | No |
| Search file paths | `filesystem_glob` | No |
| Write files | `filesystem_write_file` | Yes |
| Edit files | `filesystem_edit` | Yes |
| Run commands | `bash` | Depends on risk |
| Fetch web pages | `webfetch` | No |
| Manage task list | `todo_write` | No |
| Save/read/delete memories | `memo_*` | No |
| Launch subagent | `spawn_subagent` | No |

External tools registered via MCP also appear in the list, namespaced as `mcp.<server-id>.<tool>`. See [MCP Tools](./mcp) for setup.

## Permission approval

When the agent requests a file write or command execution, NeoCode shows a confirmation prompt:

```text
◆ NEO wants to run: filesystem_write_file
  path: src/main.go
  content: (428 bytes)

  [Allow] [Ask] [Deny]
```

- **Allow**: Approve and remember — future identical operations won't ask again
- **Ask**: Ask every time (default)
- **Deny**: Reject this request

### How to choose

| Your situation | Recommendation |
|---|---|
| Continuous refactoring, workspace is safe | Allow — fewer interruptions |
| Reading an unknown repo, observing first | Ask — confirm each time |
| Involves directories you don't want changed or high-risk commands | Deny — block immediately |

### Full Access mode

Press `!` to enable Full Access, which skips all permission approvals.

::: warning
Full Access skips all approvals, including destructive operations. Make sure you understand the risks before enabling it.
:::

## Command risk classification

Bash commands aren't all treated the same — NeoCode classifies them by risk:

| Category | Examples | Handling |
|---|---|---|
| Read-only | `git status`, `git log`, `ls` | Auto-approved |
| Local mutation | `git commit`, `go build` | Needs approval |
| Remote interaction | `git push`, `git fetch` | Needs approval |
| Destructive | `git reset --hard`, `rm` | Needs approval |
| Unknown | Compound commands, parse failures | Needs approval |

## File operation scope

All file operations are restricted to the current workspace. Path traversal and symlink escapes are blocked.

## Next steps

- Configure tool parameters: [Configuration](./configuration)
- Daily operations: [Daily use](./daily-use)
- Connect external tools: [MCP Tools](./mcp)
