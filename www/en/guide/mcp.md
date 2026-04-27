---
title: MCP Tools
description: Connect external MCP servers so NeoCode can safely call your tools.
---

# MCP Tools

MCP connects existing external tools to NeoCode: internal document search, issue lookup, read-only database queries, or team automation.

Use [Skills](./skills) for workflow instructions, [AGENTS.md](./agents-md) for project rules, and memory for personal preferences.

## When to use MCP

| Need | Use |
|---|---|
| Call a real external tool | MCP |
| Query company docs or private systems | MCP |
| Expose read-only database queries | MCP with a narrow allowlist |
| Shape the current workflow | Skills |
| Save project or personal facts | `AGENTS.md` or memory |

## Minimal config

Add a server to:

```text
~/.neocode/config.yaml
```

```yaml
tools:
  mcp:
    servers:
      - id: docs
        enabled: true
        source: stdio
        stdio:
          command: node
          args:
            - ./mcp-server.js
          workdir: ./mcp
        env:
          - name: MCP_TOKEN
            value_env: MCP_TOKEN
```

Store secrets in environment variables and reference them with `value_env`.

## Limit visible tools

```yaml
tools:
  mcp:
    exposure:
      allowlist:
        - mcp.docs.search
      denylist:
        - mcp.docs.delete*
```

`denylist` hides matching tools and has higher priority.

## Verify

After starting NeoCode, ask:

```text
Please list your currently available tools.
```

Then run one real query:

```text
Please use mcp.docs.search to search "release process" and summarize the results.
```

Tool parameters depend on your MCP server.

## Troubleshooting

If tools are missing, check:

- `enabled` is `true`
- `stdio.command` is executable
- `stdio.workdir` is correct
- `env.value_env` points to an environment variable set in the same terminal
- `allowlist` or `denylist` did not filter the tool out

## Next steps

- Unsure what to use: [Capability Guide](./capability-choice)
- Workflow instructions: [Skills](./skills)
- Approvals: [Tools & Permissions](./tools-permissions)
