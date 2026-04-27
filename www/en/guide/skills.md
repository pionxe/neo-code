---
title: Skills
description: Use SKILL.md to shape the workflow for the current task.
---

# Skills

Skills are reusable workflow prompts. They do not add tools and do not bypass approvals. They tell the agent how to handle a type of task.

If the rule belongs to the project, use [AGENTS.md](./agents-md). If it is a personal long-term preference, use memory. If the agent needs a real external tool, use [MCP](./mcp).

## When to use Skills

| Need | Use |
|---|---|
| Always list review risks first | Skill |
| Read tests before editing | Skill |
| Current task needs a fixed output shape | Skill |
| Save project rules | `AGENTS.md` |
| Save personal preferences | Memory |
| Connect external tools | MCP |

## Location

Local Skills live under:

```text
<workspace>/.neocode/skills/
~/.neocode/skills/
```

If `~/.neocode/skills/` does not exist, NeoCode falls back to `~/.codex/skills/`.

Recommended layout:

```text
<workspace>/.neocode/skills/go-review/SKILL.md
~/.neocode/skills/go-review/SKILL.md
```

## Example Skill

```md
---
id: go-review
name: Go Review
description: Review Go changes for correctness, boundaries, and tests.
---

# Go Review

## Instruction

Read related implementation and tests before reviewing. Prioritize regressions, error handling, edge cases, and missing tests. List risks first, then give a short summary.
```

## Enable and disable

```text
/skills
/skill use go-review
/skill active
/skill off go-review
```

`/skill use <id>` affects the current session only.

## Writing good instructions

Weak:

```md
## Instruction

Review more carefully.
```

Better:

```md
## Instruction

Read related implementation and tests first. Output high-risk findings, then test gaps, then a short summary. Do not request unrelated refactors.
```

## Common issues

### `/skills` cannot see my Skill

Check:

- It is under `~/.neocode/skills/`
- The file name is `SKILL.md`
- Frontmatter is valid YAML
- `id` is not duplicated

### Can a Skill grant tool access?

No. File writes and commands still follow normal approval flow.

## Next steps

- Unsure what to use: [Capability Guide](./capability-choice)
- External tools: [MCP Tools](./mcp)
- Project rules: [AGENTS.md Project Rules](./agents-md)
