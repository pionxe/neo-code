---
title: Daily Use
description: Workspace, sessions, memory, Skills, and subagents for everyday NeoCode use.
---

# Daily Use

## Workspace

The workspace decides which project NeoCode can read and edit.

```text
/cwd                     # Show current workspace
/cwd /path/to/project    # Switch to another project
```

You can also set it at launch:

```bash
neocode --workdir /path/to/project
```

## Sessions

### Switch sessions

```text
/session                 # Open the session picker
```

### Compress long sessions

When a conversation gets long, old context can interfere with the current task. Run:

```text
/compact
```

### New session or continue?

| Scenario | Recommendation |
|---|---|
| Starting an unrelated task | New session |
| Continuing the same feature | Continue current session |
| Switching projects | New session and switch workspace |
| Responses repeat or drift | Try `/compact`, then new session if needed |

## Memory

Memory is for preferences or project facts that should carry across sessions.

```text
/memo                              # View all memories
/remember I prefer powershell      # Save a memory
/forget powershell                 # Delete matching memories
```

Good uses:

- Your usual shell, test command, or code style preference
- Stable project facts, such as language, package manager, or test entry point
- Personal preferences you do not want to repeat

Avoid storing:

- Temporary task instructions
- Secrets, tokens, or passwords
- Context that only matters in the current session

## Skills

Skills make the agent follow a workflow in the current session, such as "read tests before editing" or "list review risks first".

Common commands:

```text
/skills                  # View available Skills
/skill use go-review     # Activate a Skill
/skill off go-review     # Deactivate a Skill
/skill active            # View active Skills
```

Quick rule:

- Long-term preference or project fact: use memory
- Current task workflow: use a Skill
- Real external tool capability: use [MCP](./mcp)

## Subagents

For complex tasks, NeoCode may use subagents for search, review, or checks. You usually do not need to manage them.

If you want it to split the task, say:

```text
Please have a researcher review the related code first, then have a reviewer check the plan for risks.
```

## Common commands

| Command | Purpose |
|---|---|
| `/help` | Show all commands |
| `/provider` | Switch provider |
| `/model` | Switch model |
| `/cwd` | View or switch workspace |
| `/session` | Switch session |
| `/compact` | Compress a long session |
| `/memo` | View memory |
| `/remember <text>` | Save memory |
| `/forget <keyword>` | Delete memory |
| `/skills` | View Skills |
| `/exit` | Exit |

## Next steps

- Configure models and providers: [Configuration](./configuration)
- Understand approvals: [Tools & Permissions](./tools-permissions)
- Write or activate a Skill: [Skills](./skills)
- Something wrong: [Troubleshooting](./troubleshooting)
