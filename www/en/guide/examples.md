---
title: Usage Examples
description: Four copy-pasteable scenarios to help you get comfortable with NeoCode quickly.
---

# Usage Examples

You can paste these examples directly into a NeoCode session. Replace `{...}` with real paths, logs, or requirements from your project.

---

## Scenario 1: Understand an unfamiliar project

**Goal**: You opened a project and want a quick overview of its structure and main modules.

**Prompt**:

```text
Please read the current project directory structure, explain what each main directory is responsible for, and point out which files I should read first to understand the main flow.
```

**Expected behavior**:

- Scan the project tree and key files
- Summarize module responsibilities and entry points
- If the project is large, give an overview first and ask before going deeper

**Approval tips**:

- File reads and search are usually safe to allow
- If it wants to run a command, check that the command is only inspecting project structure

---

## Scenario 2: Find and fix a bug

**Goal**: You have an error or failing test log and want NeoCode to find the cause and fix it.

**Prompt (step 1)**:

```text
I see the following failure. Please locate the root cause and propose a fix:

{test log}
```

**Prompt (step 2)**:

```text
Please apply the fix you proposed, then explain what changed and how to verify it.
```

**Expected behavior**:

- Read relevant source and tests
- Find the failing logic
- Modify only the needed files
- Suggest or run the relevant tests

**Approval tips**:

- Choose **Ask** or **Allow** for file edits
- Test commands are usually safe to allow
- Reject or ask for explanation before deletes, resets, or broad rewrites

---

## Scenario 3: Add tests for a function

**Goal**: Add tests for a function or module.

**Prompt**:

```text
Please add unit tests for {function name} in {source file}. Cover the happy path, empty input, and error return. Put the tests in {test file}.
```

**Verification**:

```text
Please run the related tests. If any fail, analyze and fix them.
```

**Expected behavior**:

- Read the target function and existing test style
- Generate tests that match the project
- Run the smallest relevant test scope

**Approval tips**:

- Check the file path before allowing test writes
- Allow the project's existing test command

---

## Scenario 4: Add a small feature

**Goal**: Add a clear feature to an existing project.

**Prompt (design phase)**:

```text
I want to add this feature: {feature description}. Please read the related code first, then propose the smallest implementation plan and the files that need changes.
```

**Prompt (implementation phase)**:

```text
Please implement the plan, keep the existing code style, and add necessary tests.
```

**Expected behavior**:

- Identify the feature entry point and impact
- Propose a short implementation plan
- Modify only relevant files
- Add tests and report verification

**Approval tips**:

- Keep **Ask** for multi-file changes
- Allow build or test commands
- If it proposes unrelated refactors, ask it to narrow the change

---

## Next steps

- Permission decisions: [Tools & Permissions](./tools-permissions)
- Configure models and providers: [Configuration](./configuration)
- Daily operations: [Daily Use](./daily-use)
- Something wrong: [Troubleshooting](./troubleshooting)
