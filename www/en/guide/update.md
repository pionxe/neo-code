---
title: Update & Version Check
description: Check the installed version and upgrade NeoCode.
---

# Update & Version Check

NeoCode checks for newer versions at startup. In normal use, no extra configuration is required; if an update is available, follow the upgrade command shown by NeoCode.

## Check your version

```bash
neocode version
```

Include pre-release versions:

```bash
neocode version --prerelease
```

## Upgrade

Upgrade to the latest stable version:

```bash
neocode update
```

Install a pre-release version:

```bash
neocode update --prerelease
```

## Common issues

### Version shows `dev`

If you run NeoCode from source, the version may show `dev`. That usually means you are using a local development build instead of a release package.

For normal use, rerun the installer from [Install & First Run](./install).

### Version is still old after upgrading

1. Close and reopen your terminal.
2. Run `neocode version`.
3. If the version is still old, rerun the installer and confirm the install directory is in `PATH`.

## Next steps

- Install issues: [Troubleshooting](./troubleshooting)
- Configure models or providers: [Configuration](./configuration)
