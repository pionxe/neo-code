# NeoCode TUI v2 — Ghost Console 视觉与交互规范

> 版本：v3.0 | 日期：2026-05-18 | 父文档：[架构导航](./tui-v2-architecture-hub.md)

---

## What — 本规范解决什么问题

本规范定义 **NeoCode TUI v2 的视觉语言与用户交互体验**。它要解决的问题是：

- **TUI v1 视觉语言不统一**：当前界面混用了面板边框、内联消息、弹窗等多种视觉模式，缺乏一致的设计语言。
- **信息层级不清晰**：所有信息线性排列在一个滚动区，用户在长时间会话中难以快速定位关键信息。
- **缺乏产品气质**："能用的终端工具" vs "为 Agent 编程设计的黑客控制台"。

**边界**：本规范仅涉及 UI 渲染、布局、色彩和键盘/鼠标交互。状态管理、Gateway 通信、代码目录结构分别由其他子文档定义。

## Why — 为什么选择这个方向

### 为什么不继续用 v1 的视觉风格

v1 的设计参考了传统聊天应用和简易 IDE 的混合风格。它在功能上是完整的，但在视觉上有三个根本问题：

1. **线框思维**：大量 `┌ ┐ ├ ┤ │` 字符切割界面，产生"后台管理系统"感。终端不是网页，线框在终端中比在浏览器中压迫感强得多。
2. **容器思维**：把信息放进"面板"、"卡片"、"标签页"里，这是 GUI 时代的思维惯性。终端的美学在于**留白、缩进、文本流**。
3. **椭圆药丸与玻璃拟态**：这些来自现代 Web 设计的元素在终端中不可靠（阴影、渐变、圆角依赖 True Color 和字体），且在等宽字符网格中视觉不协调。

### 为什么选择 Ghost Console 方向

Ghost Console 是一种**终端原生**的设计哲学：

- **不模仿 GUI**。终端有自己的美学传统：状态行、文本流、命令提示符、浮层弹窗。
- **不抢夺注意力**。Agent 是主角，UI 是安静的舞台。用色彩分层和符号节奏传递信息，而非用容器和边框喊叫。
- **不依赖高级终端特性**。Nerd Font、True Color 是渐进增强，不是硬依赖。
- **黑客控制台气质**：不是"帮你写代码的聊天机器人"，而是"你指挥的 Agent 正在屏幕背后运转"。

### 为什么键位必须与 v1 不兼容

v2 引入 Vim 风格的双模式键盘操作（Input Mode / Normal Mode / Leader Key），与 v1 的单一输入模式有根本性冲突：

- v1 中 `Esc` = 聚焦输入框，v2 中 `Esc` = 进入 Normal Mode（失去输入焦点）
- v1 中 `Ctrl+J` = 换行，v2 中 `Shift+Enter` = 换行（更符合直觉）
- v1 中 `Ctrl+W` = 取消运行，v2 中 `Ctrl+C` = 取消/双退（终端惯例）

在同一套键位配置中兼容两套截然不同的语义是不可能的。因此 v2 选择独立实现，而非渐进改造。

---

## How — 具体设计规范

### 1. 视觉设计原则

#### 1.1 方向定义

> **NeoCode Ghost Console（幽灵控制台）**
>
> 极简、冷静、高密度但不拥挤、像黑客终端、像 Agent 正在屏幕背后运转。
> 不是传统 IDE，不是后台管理系统，不是聊天软件。

#### 1.2 设计关键词

```text
terminal-native    终端原生，不是网页移植
hacker console     黑客控制台，不是低配 IDE
command surface    命令界面，不是导航菜单
agent stream       Agent 行为流，不是聊天消息
ambient status     环境状态，不是状态面板
```

#### 1.3 核心视觉原则

**原则一：默认不画框**

线框会切割界面，产生"后台管理系统"感。NeoCode Ghost Console 默认不使用边框。

仅以下场景允许使用边框：

- 命令面板（Telescope 风格）
- 快捷键帮助（分组信息多）
- 会话选择器（搜索 + 列表）
- 危险操作确认（防误操作）
- diff 预览
- 错误详情（完整错误栈）

普通消息、工具调用、文件变更、状态展示、任务进度 **不得使用完整线框**。

**原则二：用语义标签代替标题栏**

不要写：

```
┌─ Runtime Status ─────┐
│ Phase: Execute       │
└──────────────────────┘
```

写成：

```
runtime
  phase     execute
  tools     2 active
  tokens    12.3k
```

**原则三：用状态符号建立节奏**

```text
✓ completed     已完成
◉ active        活跃中
○ idle          空闲
◌ waiting       等待中
× failed        失败
· pending       待处理
→ transitioning 过渡中
```

**原则四：用缩进表达层级**

不使用表格和竖线来组织信息。层级关系通过缩进和弱色文字表达。

```
tool.edit_file
  path      internal/tui/core/app/view.go
  status    applied
  diff      +42 -18
```

**原则五：视觉分层，不分区**

不是把屏幕切为左中右面板，而是通过纵向三层逻辑结构：

```text
ambient layer     顶部环境状态 — 一行，像状态脉冲
stream layer      中央 Agent 行为流 — 事件节奏驱动
command layer     底部命令输入 — 极简提示符
```

#### 1.4 LazyVim 风格约束

**禁止项：**

- 椭圆药丸（oval pills / rounded tags）
- 玻璃拟态卡片（glass-morphism cards）
- 外层圆角终端容器（outer rounded terminal containers）
- 渐变背景色块
- 阴影效果（终端无法可靠渲染）
- 任何模拟浏览器窗口的装饰

**允许项：**

- 扁平状态行（flat segments）
- 色彩分层（LazyVim 风格的多层颜色系统）
- 语义高亮（关键字、类型、字段使用不同颜色）
- accent bar 高亮（命令面板当前项左侧竖线）
- 微妙背景色差（如 Ambient Status 行比 Stream 背景深 1 级）

#### 1.5 终端约束

| 约束      | 最低要求     | 推荐要求     | 理想要求     |
| --------- | ------------ | ------------ | ------------ |
| 终端宽度  | 80 列        | 120 列       | 160 列       |
| 终端高度  | 24 行        | 30 行        | 40 行        |
| 色彩      | 256 色       | True Color   | True Color   |
| Nerd Font | 不可强依赖   | 可选增强     | 可选增强     |
| 鼠标      | **强制支持** | **强制支持** | **强制支持** |
| 键盘      | 完全可操作   | 完全可操作   | 完全可操作   |

---

### 2. Focus-Only 布局

#### 2.1 单布局策略

TUI v2 **不提供** wide/focus/compact 等多布局模式。所有终端尺寸使用同一套 Focus-Only 布局，并按宽度自动进入隐藏、堆叠或右侧栏三档响应式形态：

```text
NEOCODE  ◉ run   build   sonnet-4.6   ~/neo-code                                      12.3k


  mission
  redesign tui into a gateway-only hacker console

  ✓ scanned current document
  ✓ identified visual problem
  ◉ removing box-heavy layout
  · drafting ghost console structure


  you
  我觉得还是不行，特别是 TUI 的实现不应该太多线框，
  会导致有割裂感。TUI 的视觉显示应该是极客风格的、极简又极其炫酷。

  neo
  agreed.
  NeoCode should not look like a boxed IDE.
  It should look like a quiet command surface for controlling an agent.

  tool.read_file
    docs/tui-redesign-gateway-contract.md
    1 file · layout section detected · 31ms

  design.shift
    from  classic three-column ide
    to    ghost console


› 继续设计这个方向
```

#### 2.2 单布局的响应式行为

Focus-Only 是**一种布局**，不是多种模式。它随终端宽度自动调整，无需用户手动切换：

- **< 80 列**：Soft Inspector 自动隐藏。Stream 占满宽度。Ambient Status 省略非关键字段。
- **80–99 列**：Soft Inspector 自动出现，采用堆叠布局（Inspector 位于 Stream 区域之外，占据全宽）。
- **≥ 100 列**：Soft Inspector 自动出现在右侧（约 30 列，无边框，弱色文字）。

Soft Inspector 的显示/隐藏是**自动的、无感的**，不是用户可选的"wide 模式"。它只是单布局在充裕空间下的自然延伸。

#### 2.3 区域职责

| 区域           | 位置                               | 视觉方式                                   | 职责                                             |
| -------------- | ---------------------------------- | ------------------------------------------ | ------------------------------------------------ |
| Ambient Status | 顶部一行                           | 单行状态脉冲，无边框，微弱背景色差         | 展示产品名、连接状态、模式、模型、目录、Token    |
| Agent Stream   | 中央主体                           | 标签 + 缩进 + 流式文本，可鼠标滚轮滚动     | 展示所有 Agent 行为流事件                        |
| Soft Inspector | 80–99 列堆叠显示，≥ 100 列右侧显示 | 无边框弱色文字，宽屏右列约 30 列           | 展示 Runtime、Tools、Files 摘要；< 80 列自动隐藏 |
| Command Prompt | 底部                               | `›` 输入提示符，无边框                     | 用户输入、命令、模式指示                         |
| Modal Overlay  | 居中浮层                           | 仅命令面板、帮助、会话选择、确认时使用边框 | 命令面板、帮助、会话切换、危险操作确认           |

---

### 3. 核心 UI 区域设计

#### 3.1 Ambient Status 顶部环境层

位置：第 1 行。高度：1 行。无边框。微弱背景色差（比 Stream 背景深 1 级）。

```
NEOCODE  ◉ run   build   sonnet-4.6   ~/neo-code                                      12.3k
```

状态变化示例：

```
NEOCODE  ○ idle   build   sonnet-4.6   ~/neo-code                                      12.3k
NEOCODE  ◉ run    build   sonnet-4.6   ~/neo-code                                      12.3k
NEOCODE  ◌ wait   review  sonnet-4.6   ~/neo-code                                      12.3k
NEOCODE  × error  build   sonnet-4.6   ~/neo-code                                      12.3k
```

| 符号        | 含义         | 触发条件                                           |
| ----------- | ------------ | -------------------------------------------------- |
| `○ idle`    | Agent 空闲   | `run_done` / `run_error` / 无活跃运行              |
| `◉ run`     | Agent 运行中 | `phase_changed` 到 execute/plan                    |
| `◌ wait`    | 等待用户输入 | `permission_requested` / `user_question_requested` |
| `× error`   | 发生错误     | `run_error` / `fatal_error`                        |
| `~ compact` | 正在压缩     | `compact_start`                                    |

#### 3.2 Agent Stream 中央行为流

位置：第 2 行到倒数第 2 行。可垂直滚动，支持鼠标滚轮和拖拽滚动。

**消息类型与视觉样式：**

| 类型           | 标签                 | 颜色             | 缩进             | 说明                      |
| -------------- | -------------------- | ---------------- | ---------------- | ------------------------- |
| 用户消息       | `you`                | Cyan             | 2 空格           | 用户输入的内容            |
| Assistant 消息 | `neo`                | Purple           | 2 空格           | Agent 回复，Markdown 渲染 |
| 工具调用开始   | `tool.<name>`        | Yellow           | 2 空格           | 工具名称 + 参数摘要       |
| 工具调用结果   | （缩进在 tool 下方） | Green/Red        | 4 空格           | 结果摘要、耗时、行数      |
| 文件变更       | `file.modified` 等   | Green/Red/Yellow | 2 空格           | 路径 + diff 统计          |
| 系统事件       | `design.shift` 等    | Muted Gray       | 2 空格           | 语义事件标签              |
| 错误           | `error`              | Red              | 2 空格           | 错误码 + 描述             |
| 思考过程       | `thinking`           | Muted Gray       | 2 空格，默认折叠 | 模型思考过程              |

**流式输出处理：**

- `agent_chunk`：逐字符追加，打字机效果
- `thinking_delta`：更新思考折叠区域
- `agent_done`：消息完整，Markdown 最终渲染

**滚动行为：**

- 新消息自动滚动到底部
- 用户手动上滚（键盘或鼠标滚轮）时停止自动滚动
- `g` 跳到顶部，`G` 跳到底部；`Ctrl+D` / `Ctrl+U` 半页下/上翻

#### 3.3 Soft Inspector（响应式右侧弱信息列）

Soft Inspector 是 Focus-Only 单布局的一部分，非独立模式。当终端宽度 ≥ 100 列时自动出现在右侧，宽度不足时自动隐藏。无边框，弱色文字，不抢占中央 Stream 的视觉中心。用户无需手动切换。

```
  mission                                      runtime
  redesign tui into ghost console             phase       execute
                                               turn        3 / 8
  you                                           tools
  这个布局还是不够酷，线框太多。                  grep           running · 1.2s
                                                find_files     done · 230ms
  neo
  agreed.                                       files
                                                M app.go          +12 -5
  tool.read_file                                M handler.go      +3 -1
    docs/tui-redesign-gateway-contract.md
    31ms

› 继续
```

**数据来源：**

| 信息块       | 数据来源                                                                                |
| ------------ | --------------------------------------------------------------------------------------- |
| `runtime` 块 | `gateway.event` → `EventPhaseChanged`、`EventTokenUsage`、`EventRuntimeSnapshotUpdated` |
| `tools` 块   | `gateway.event` → `EventToolStart`、`EventToolResult`                                   |
| `files` 块   | `gateway.event` → `EventBashSideEffect`、`EventRunDiffSummary`                          |

#### 3.4 Command Prompt 底部命令输入

位置：底部 1–10 行（根据输入内容动态扩展）。无边框。支持鼠标点击定位光标。

**各状态示例：**

```text
# 默认 Input Mode
› 继续设计 Ghost Console 布局

# 运行中（只读）
◉ running · ctrl+c cancel
›

# 等待权限（内联，不弹窗）
◌ permission required · allow edit_file on internal/tui/core/app/view.go?
› [a] allow once   [A] allow always   [d] deny

# 等待 ask_user（内联，不弹窗）
◌ question · select the target module
› [1] gateway   [2] runtime   [3] tui   [s] skip

# ask_user 选项文字过长时自动换行，每条选项独占一行
◌ question · which approach do you prefer for error handling?
  [1] Centralized error handler with sentinel types and wrapped errors
  [2] Per-package error types with domain-specific error classification
  [3] Functional error handling with Result<T> monad pattern via generics
  [s] skip
›

# 附件显示
  @ app.go (234 lines)  @ handler.go (156 lines)
› 请重构这两个文件

# Normal Mode
  NORMAL  q:quit  i:input  /:search  ::command  ?:help
```

实现方式：自定义 textarea 组件（非 Bubbles 原生），支持 Input Mode 和 Normal Mode 切换。Prompt 前缀 `›` 颜色随状态变化。支持鼠标点击定位光标。

#### 3.5 弹窗与浮层

弹窗是唯一可以使用边框的区域。

**Telescope 风格命令面板**（`Ctrl+P` 或 `Space p`）：

```
┌─ ── ── ── ── ── ── ── ── ── ── ── ── ── ── ──┐
│ > mod                                            │
│                                                  │
│ ▎  /model       Change the current model        │
│    /mode        Switch between build and plan   │
│    /session     Browse and switch sessions      │
│    /compact     Compact current session         │
│    /checkpoint  Manage checkpoints              │
│    /skills      Manage session skills           │
│    /help        Show keyboard shortcuts         │
│    /exit        Quit NeoCode                    │
│                                                  │
│  ␣ : close   ⏎ : execute   ␛ : dismiss         │
└─ ── ── ── ── ── ── ── ── ── ── ── ── ── ── ──┘
```

Telescope 风格特征：

- `▎` accent bar 标记当前高亮项
- 输入即搜索（模糊匹配），无需先 Tab 到搜索框
- 底部提示行显示键盘操作
- 鼠标点击项可选，滚轮滚动列表
- 边框使用细虚线或暗色，不显眼

**快捷键帮助**（`?` 或 `Space h`，分组展示）：

```
┌─ ── ── ── ── ── ── ── ── ── ── ── ── ── ── ──┐
│                                                  │
│  Input Mode                                      │
│    Enter        Send message                     │
│    Shift+Enter  New line                         │
│    Ctrl+C       Cancel agent (double to quit)    │
│    Ctrl+P       Command palette                  │
│    ?            This help                        │
│    /            Slash command                    │
│    @            Attach file reference            │
│                                                  │
│  Normal Mode (Esc)                               │
│    i            Enter Input Mode                 │
│    /            Search in stream                 │
│    :            Command line                     │
│    q            Quit                             │
│                                                  │
│  Leader (Space)                                  │
│    Space p      Command palette                  │
│    Space n      New session                      │
│    Space s      Switch session                   │
│    Space h      Help                             │
│    Space q      Quit                             │
│                                                  │
│  Navigation                                      │
│    j / k        Scroll down / up                 │
│    Ctrl+D / U   Half-page down / up              │
│    g / G        Jump to top / bottom             │
│    Mouse wheel  Scroll                           │
│                                                  │
│  ␛ : close                                      │
└─ ── ── ── ── ── ── ── ── ── ── ── ── ── ── ──┘
```

**会话选择器**（`Space s`）：

```
┌─ ── Sessions ── ── ── ── ── ── ── ── ── ── ── ──┐
│ > debu                                            │
│                                                   │
│ ▎  ◉ demo-session          2026-05-18 10:30      │
│      api-debugging         2026-05-18 09:15      │
│      refactor-app          2026-05-17 16:00      │
│                                                   │
│  ␣ : switch   Ctrl+D : delete   ␛ : cancel       │
└─ ── ── ── ── ── ── ── ── ── ── ── ── ── ── ── ──┘
```

#### 3.6 弹窗与内联规则

| 交互类型     | 展示方式                   | 原因                                        |
| ------------ | -------------------------- | ------------------------------------------- |
| 权限确认     | **内联**（底部 prompt 区） | 不打断用户注意力；键盘 `a`/`A`/`d` 直接响应 |
| ask_user     | **内联**（底部 prompt 区） | 属于对话流的一部分；选项过多时自动换行      |
| 命令面板     | 弹窗（边框）               | 搜索优先，需要独立交互空间                  |
| 快捷键帮助   | 弹窗（边框）               | 分组信息量大，需要独立展示                  |
| 会话选择     | 弹窗（边框）               | 搜索 + 列表选择，需要独立交互空间           |
| 危险操作确认 | 弹窗（边框）               | 需要明确打断用户，防止误操作                |

---

### 4. 键位系统

#### 4.1 三层键位总览

TUI v2 使用三层键位系统，与 TUI v1 完全不兼容：

| 层              | 进入方式                   | 说明                         |
| --------------- | -------------------------- | ---------------------------- |
| **Input Mode**  | 默认启动 / Normal Mode `i` | 文本输入、发送消息、输入命令 |
| **Normal Mode** | `Esc`                      | 导航、搜索、命令操作         |
| **Leader Key**  | Normal Mode 下按 `Space`   | 快捷操作前缀                 |

模式指示：

```
Input Mode:    › 用户输入...
Normal Mode:   NORMAL  i:input  /:search  ::command  q:quit  ?:help
```

#### 4.2 Input Mode 键位

| 快捷键        | 行为                                                                                                                |
| ------------- | ------------------------------------------------------------------------------------------------------------------- |
| `Enter`       | 发送消息                                                                                                            |
| `Shift+Enter` | 输入换行                                                                                                            |
| `Ctrl+C`      | 如果 Agent 正在运行：取消运行。如果 Agent 空闲：第一次提示 "Press Ctrl+C again to quit"，第二次退出应用（双退保护） |
| `Ctrl+P`      | 打开 Telescope 命令面板                                                                                             |
| `?`           | 打开快捷键帮助                                                                                                      |
| `/`           | 当输入框为空时，进入 Slash 命令模式                                                                                 |
| `@`           | 当输入框为空时，进入文件引用模式                                                                                    |
| `Esc`         | 切换到 Normal Mode（清空输入焦点）                                                                                  |
| `Ctrl+L`      | 日志查看器                                                                                                          |

#### 4.3 Normal Mode 键位（`Esc` 进入）

| 快捷键              | 行为                            |
| ------------------- | ------------------------------- |
| `i`                 | 返回 Input Mode（聚焦输入框）   |
| `j` / `k`           | 向下 / 向上滚动 Stream          |
| `Ctrl+D` / `Ctrl+U` | 半页向下 / 向上滚动             |
| `g`                 | 跳到 Stream 顶部                |
| `G`                 | 跳到 Stream 底部                |
| `/`                 | 在 Stream 中向前搜索            |
| `?`                 | 在 Stream 中向后搜索            |
| `n` / `N`           | 跳转到下一个 / 上一个搜索结果   |
| `:`                 | 打开命令行（Ex 命令）           |
| `q`                 | 退出应用（有双退保护）          |
| `Space`             | Leader Key 前缀（等待后续按键） |

#### 4.4 Leader Key 键位（`Space` 前缀）

Leader Key 超时 1 秒：如果 1 秒内无后续按键，取消 Leader 等待，保持 Normal Mode。

| 序列          | 行为                         |
| ------------- | ---------------------------- |
| `Space` + `p` | 命令面板                     |
| `Space` + `n` | 新建会话                     |
| `Space` + `s` | 会话切换器                   |
| `Space` + `h` | 快捷键帮助                   |
| `Space` + `m` | 切换 Agent 模式 (build/plan) |
| `Space` + `f` | 切换 Full Access             |
| `Space` + `l` | 日志查看器                   |
| `Space` + `c` | 手动 compact                 |
| `Space` + `q` | 退出应用                     |

#### 4.5 鼠标操作（强制支持）

| 鼠标操作  | 上下文                                          | 行为            |
| --------- | ----------------------------------------------- | --------------- |
| 滚轮上/下 | Stream 区域                                     | 滚动 Stream     |
| 滚轮上/下 | 命令面板 / Picker                               | 滚动选项列表    |
| 左键单击  | Stream 中的可点击项（如 `[retry]`、`[expand]`） | 触发对应操作    |
| 左键单击  | 命令面板选项                                    | 选择并执行      |
| 左键单击  | 权限弹窗按钮                                    | 选择 allow/deny |
| 左键单击  | 输入框                                          | 定位光标        |
| 拖拽      | Stream 区域                                     | 滚动 Stream     |

#### 4.6 `Ctrl+C` 双退保护逻辑

```
按下 Ctrl+C:
  if Agent 正在运行:
    → 取消 Agent 运行
    → 提示 "run cancelled"
  else if Agent 空闲:
    if 上次 Ctrl+C 在 2 秒内:
      → 退出应用
    else:
      → 提示 "Press Ctrl+C again to quit"
```

#### 4.7 与 TUI v1 键位对照

| 功能        | TUI v1       | TUI v2                             | 说明       |
| ----------- | ------------ | ---------------------------------- | ---------- |
| 发送        | `Enter`      | `Enter`                            | 一致       |
| 换行        | `Ctrl+J`     | `Shift+Enter`                      | **不兼容** |
| 取消运行    | `Ctrl+W`     | `Ctrl+C` (首次)                    | **不兼容** |
| 退出        | `Ctrl+U`     | `Ctrl+C` (再次) / `Space q` / `:q` | **不兼容** |
| 新建会话    | `Ctrl+N`     | `Space n`                          | **不兼容** |
| 聚焦输入    | `Esc`        | `i` (Normal → Input)               | **不兼容** |
| 帮助        | `Ctrl+Q`     | `?` / `Space h`                    | **不兼容** |
| Full Access | `Ctrl+F`     | `Space f`                          | **不兼容** |
| 日志        | `Ctrl+L`     | `Ctrl+L` / `Space l`               | 兼容       |
| 向上滚动    | `Up` / `k`   | `k` (Normal) / 鼠标滚轮            | 部分兼容   |
| 向下滚动    | `Down` / `j` | `j` (Normal) / 鼠标滚轮            | 部分兼容   |
| 顶部        | `g` / `Home` | `g` (Normal)                       | 兼容       |
| 底部        | `G` / `End`  | `G` (Normal)                       | 兼容       |
| 粘贴图片    | `Ctrl+V`     | 移除                               | **移除**   |

---

### 5. LazyVim 色彩分层与主题

#### 5.1 语义色板（Go 定义）

```go
type ThemeColors struct {
    // 背景层
    Background    lipgloss.Color // 主背景  #1a1b26 (Tokyo Night bg)
    SurfaceAlt    lipgloss.Color // 次级背景 #1f2335 (Ambient Status 行)

    // 前景层（文本）
    TextPrimary   lipgloss.Color // 主要文本 #c0caf5
    TextSecondary lipgloss.Color // 次级文本 #a9b1d6
    TextMuted     lipgloss.Color // 弱化文本 #565f89

    // 语义色（LazyVim 风格）
    Accent        lipgloss.Color // 主强调色 (Neo)       #7aa2f7 (Blue)
    Success       lipgloss.Color // 成功                  #9ece6a (Green)
    Warning       lipgloss.Color // 警告/等待             #e0af68 (Yellow)
    Error         lipgloss.Color // 错误                  #f7768e (Red)
    Info          lipgloss.Color // 信息/用户消息          #7dcfff (Cyan)
    Hint          lipgloss.Color // 提示/辅助             #bb9af7 (Purple)

    // 状态符号特殊色
    GlyphActive   lipgloss.Color // ◉ 活跃               #9ece6a (Green)
    GlyphIdle     lipgloss.Color // ○ 空闲               #565f89 (Muted)
    GlyphWaiting  lipgloss.Color // ◌ 等待               #e0af68 (Yellow)
    GlyphError    lipgloss.Color // × 错误               #f7768e (Red)

    // Telescope 面板
    TelescopeAccent lipgloss.Color // ▎ accent bar      #7aa2f7 (Blue)
    TelescopeMatch  lipgloss.Color // 搜索匹配高亮       #e0af68 (Yellow)

    // 语法高亮（Markdown 代码块内）
    SynKeyword   lipgloss.Color // 关键字                #9d7cd8 (Purple)
    SynString    lipgloss.Color // 字符串                #9ece6a (Green)
    SynType      lipgloss.Color // 类型                  #2ac3de (Cyan)
    SynFunc      lipgloss.Color // 函数                  #7aa2f7 (Blue)
    SynComment   lipgloss.Color // 注释                  #565f89 (Muted)
}
```

#### 5.2 Tokyo Night 色板速查

| 色值      | 角色            | 用途                              |
| --------- | --------------- | --------------------------------- |
| `#1a1b26` | 主背景          | Stream、Command Prompt 背景       |
| `#1f2335` | 次级背景        | Ambient Status 行微弱底色         |
| `#c0caf5` | 主要文本        | Stream 内容、输入文本             |
| `#a9b1d6` | 次级文本        | Soft Inspector 值、时间戳         |
| `#565f89` | 弱化文本        | Soft Inspector 标签、状态符号 `○` |
| `#7aa2f7` | 强调色 (Blue)   | Neo 标签、Telescope accent bar    |
| `#9ece6a` | 成功色 (Green)  | `✓`、`◉`、工具成功、diff added    |
| `#e0af68` | 警告色 (Yellow) | `◌`、工具运行中、Telescope 匹配   |
| `#f7768e` | 错误色 (Red)    | `×`、error 标签、工具失败         |
| `#7dcfff` | 信息色 (Cyan)   | You 标签、用户消息                |
| `#bb9af7` | 提示色 (Purple) | `/` 命令、特殊高亮                |

#### 5.3 低色彩终端 Fallback

| 语义         | True Color | 256 Color | 16 Color    |
| ------------ | ---------- | --------- | ----------- |
| Accent       | `#7aa2f7`  | `111`     | Blue        |
| Success      | `#9ece6a`  | `114`     | Green       |
| Warning      | `#e0af68`  | `179`     | Yellow      |
| Error        | `#f7768e`  | `210`     | Red         |
| Info         | `#7dcfff`  | `117`     | Cyan        |
| Hint         | `#bb9af7`  | `183`     | Magenta     |
| Muted        | `#565f89`  | `60`      | White (dim) |
| Text Primary | `#c0caf5`  | `189`     | White       |
| Background   | `#1a1b26`  | `234`     | Black       |

#### 5.4 Nerd Font Fallback

| 符号 | 含义       | ASCII Fallback |
| ---- | ---------- | -------------- | --- |
| `◉`  | 活跃       | `(*)`          |
| `○`  | 空闲       | `( )`          |
| `◌`  | 等待       | `(?)`          |
| `×`  | 错误       | `(!)`          |
| `✓`  | 完成       | `[+]`          |
| `·`  | 待处理     | `.`            |
| `→`  | 过渡中     | `->`           |
| `›`  | prompt     | `>`            |
| `▎`  | accent bar | ` | `   |

---

_本规范是 TUI v2 文档集的子文档。架构总览见 [架构导航](./tui-v2-architecture-hub.md)，数据契约见 [数据/契约](./tui-v2-data-and-gateway.md)，工程实现见 [工程实现](./tui-v2-implementation-guide.md)。_
