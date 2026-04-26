---
title: MCP Tools
description: Connect external MCP servers to NeoCode so the agent can safely call your tools.
---

# MCP Tools

MCP is useful when you want NeoCode to call existing tools, such as internal documentation search, issue lookup, read-only database queries, or team automation scripts.

## When to use MCP

| Goal | Recommendation |
|---|---|
| Let the agent call a real external tool | Use MCP |
| Query internal docs, issue systems, or private platforms | Use MCP |
| Make the agent follow a fixed workflow | Use [Skills](./skills) |
| Save personal preferences or project facts | Use `/remember` memory |

## Minimal config

Add the MCP server to:

```text
~/.neocode/config.yaml
```

Example:

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

Common fields:

| Field | Description |
|---|---|
| `id` | MCP server name, used in tool names |
| `enabled` | Whether this server is enabled |
| `source` | Use `stdio` |
| `stdio.command` | Command that starts the MCP server |
| `stdio.args` | Command arguments |
| `stdio.workdir` | MCP server working directory |
| `env` | Environment variables passed to the MCP server |

::: tip
Put secrets in system environment variables and reference them with `value_env`. Do not write tokens, API keys, or passwords directly into `config.yaml`.
:::

## Control visible tools

If a server exposes many tools, expose only the ones the agent needs:

```yaml
tools:
  mcp:
    exposure:
      allowlist:
        - mcp.docs.search
      denylist:
        - mcp.docs.delete*
```

Common controls:

| Config | Effect |
|---|---|
| `allowlist` | Only matching tools are visible to the agent |
| `denylist` | Hide matching tools; deny wins |

## Verify availability

After starting NeoCode, ask:

```text
List the tools currently available to you.
```

After you see a tool such as `mcp.docs.search`, try a real query:

```text
Use mcp.docs.search to search for "release process" and summarize the result.
```

Tool arguments depend on your MCP server documentation.

## Common issues

### MCP tool does not appear

Check in order:

- `enabled` is `true`
- `stdio.command` is executable in the current terminal
- `stdio.workdir` points to the right directory
- system variables referenced by `env.value_env` are set
- `allowlist` or `denylist` did not filter the tool out

### Startup says an environment variable is empty

If your config contains:

```yaml
env:
  - name: MCP_TOKEN
    value_env: MCP_TOKEN
```

set `MCP_TOKEN` in the same terminal that starts NeoCode.

### Tool appears, but calls fail

Check the MCP server logs and tool arguments first. Most call failures come from invalid parameters or external services the MCP server depends on.

## Next steps

- Control agent workflow: [Skills](./skills)
- Understand approvals: [Tools & Permissions](./tools-permissions)
- Review common config: [Configuration](./configuration)
