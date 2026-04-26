---
title: 升级与版本检查
description: 查看当前版本、检查新版本，并升级 NeoCode。
---

# 升级与版本检查

NeoCode 会在启动时检查是否有新版本。普通使用时你不需要额外配置；如果检测到可升级版本，按提示执行升级命令即可。

## 查看当前版本

```bash
neocode version
```

如果你也想检查预发布版本：

```bash
neocode version --prerelease
```

## 升级到最新版

升级到最新稳定版：

```bash
neocode update
```

如果你希望安装预发布版本：

```bash
neocode update --prerelease
```

## 常见问题

### 看到 `dev` 版本

如果你是从源码运行 NeoCode，版本可能显示为 `dev`。这通常表示你运行的是本地开发构建，而不是发布包。

如果你想使用正式发布版本，建议重新执行 [安装与首次运行](./install) 中的一键安装命令。

### 升级后命令仍是旧版本

1. 关闭并重新打开终端。
2. 执行 `neocode version` 确认当前版本。
3. 如果仍未变化，重新执行安装命令，确认安装目录已经加入 `PATH`。

## 下一步

- 安装遇到问题：[排障与常见问题](./troubleshooting)
- 想切换模型或 Provider：[配置指南](./configuration)
