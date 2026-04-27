---
title: Capability Guide
description: Choose between memory, AGENTS.md, Skills, and MCP.
---

# Capability Guide

NeoCode has several related capabilities: memory, `AGENTS.md`, Skills, and MCP. They solve different problems.

## Quick choice

| What you need | Use |
|---|---|
| Personal long-term preference | Memory |
| Project-level rules | `AGENTS.md` |
| Workflow for the current task | Skills |
| Real external tools | MCP |

## Memory

Memory stores personal preferences or stable facts across sessions.

```text
/remember I prefer powershell
/remember The project test command is go test ./...
```

Do not store secrets or one-off instructions.

## AGENTS.md

`AGENTS.md` stores project rules that belong with the repository.

```md
# Project Rules

- Keep Chinese docs in Chinese
- Run `go test ./...` after Go changes
- Do not write API keys to config files
```

## Skills

Skills shape how the current task should be handled.

```text
/skills
/skill use go-review
/skill active
/skill off go-review
```

They do not add tools or bypass approvals.

## MCP

MCP connects real external tools, such as internal document search, issue lookup, read-only database queries, or team automation.

If you only need a workflow instruction, use Skills instead. Use MCP when the agent must call an external capability.

## Next steps

- Daily workflow: [Daily Use](./daily-use)
- Project rules: [AGENTS.md Project Rules](./agents-md)
- Skills: [Skills](./skills)
- External tools: [MCP Tools](./mcp)
