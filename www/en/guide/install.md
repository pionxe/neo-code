---
title: Install & First Run
description: From installation to your first conversation in 3 minutes.
---

# Install & First Run

## 1. Requirements

- At least one API key (OpenAI, Gemini, OpenLL, Qiniu, or ModelScope)
- Go 1.25+ if running from source

## 2. Install

### One-line install (recommended)

macOS / Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/neo-code/main/scripts/install.sh | bash
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/1024XEngineer/neo-code/main/scripts/install.ps1 | iex
```

### Run from source

```bash
git clone https://github.com/1024XEngineer/neo-code.git
cd neo-code
go run ./cmd/neocode
```

## 3. Set API key

NeoCode reads API keys from environment variables and never writes them to config files.

macOS / Linux:

```bash
export OPENAI_API_KEY="your_key_here"
```

Windows PowerShell:

```powershell
$env:OPENAI_API_KEY = "your_key_here"
```

Other providers:

| Provider | Environment variable |
|---|---|
| OpenAI | `OPENAI_API_KEY` |
| Gemini | `GEMINI_API_KEY` |
| OpenLL | `AI_API_KEY` |
| Qiniu | `QINIU_API_KEY` |
| ModelScope | `MODELSCOPE_API_KEY` |

## 4. Launch

```bash
neocode
```

You will enter the terminal interface. Type in the input box at the bottom to start a conversation.
To open a specific workspace:

```bash
neocode --workdir /path/to/your/project
```

## 5. First conversation

Need a starting prompt? Try:

```text
Read the current project directory structure and give a module summary
```

```text
Based on that structure, find the test entry point and the main business entry point.
```

The agent will use file reading and search tools automatically. When it needs to write files or run commands, NeoCode will ask for approval.

## 6. Common `/` commands

Inside NeoCode, use `/` commands for common actions:

| Command | Action |
|---|---|
| `/help` | Show all commands |
| `/provider` | Switch provider |
| `/provider add` | Add a custom provider |
| `/model` | Switch model |
| `/compact` | Compress long session context |
| `/clear` | Clear the current draft |
| `/cwd [path]` | View/switch workspace |
| `/session` | Switch session |
| `/memo` | View memory index |
| `/remember <text>` | Save memory |
| `/forget <keyword>` | Delete memory |
| `/skills` | View available skills |
| `/exit` | Exit NeoCode |

## Installation issues?

See [Troubleshooting](./troubleshooting)

## Next steps

- More usage scenarios: [Usage examples](./examples)
- Switch models or add custom providers: [Configuration](./configuration)
- Daily operations: [Daily use](./daily-use)
