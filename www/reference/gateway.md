---
title: Gateway 集成参考
description: 面向外部集成和开发者的 Gateway 资料入口。
---

# Gateway 集成参考

普通使用 NeoCode 不需要单独配置 Gateway。直接运行：

```bash
neocode
```

只有在你要开发外部集成、调试本地服务边界或修改相关实现时，才需要阅读这里的资料。

## 适合阅读的场景

| 场景 | 建议 |
|---|---|
| 把 NeoCode 接入外部脚本或应用 | 阅读 Gateway 详细设计 |
| 调试本地集成连接问题 | 阅读 Gateway 详细设计 |
| 修改协议、鉴权或事件中继实现 | 阅读 Gateway 详细设计和相关源码 |
| 只是日常使用 NeoCode | 回到 [开始使用](/guide/) |

## 设计文档

- [Gateway 详细设计](https://github.com/1024XEngineer/neo-code/blob/main/docs/gateway-detailed-design.md)
- [Tools 与 TUI 集成](https://github.com/1024XEngineer/neo-code/blob/main/docs/tools-and-tui-integration.md)
