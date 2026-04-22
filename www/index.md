---
layout: home

hero:
  name: NeoCode
  text: 基于 Go + Bubble Tea 的本地 Coding Agent
  tagline: 在终端中完成推理、调用工具、获取结果并持续推进任务。
  image:
    src: /brand/neocode-mark.svg
    alt: NeoCode
  actions:
    - theme: brand
      text: 文档入口
      link: /docs/
    - theme: alt
      text: GitHub
      link: https://github.com/1024XEngineer/neo-code
    - theme: alt
      text: English
      link: /en/

features:
  - title: 面向终端的交互主场
    details: 使用 Bubble Tea 构建 TUI，让会话、命令、工具结果和运行状态都留在本地工作流里。
  - title: 围绕 ReAct 的运行闭环
    details: 从用户输入到继续推理的链路由 runtime 统一编排，保证工具调用、结果回灌和 UI 展示能持续衔接。
  - title: Provider 差异收敛
    details: 模型厂商差异限制在 provider 层，避免泄漏到 runtime、TUI 或更上层的调用方。
  - title: 可控的上下文与会话状态
    details: 支持 context compact、会话持久化与工作目录隔离，降低长会话和多项目切换时的失控风险。
---

<section class="home-section">
  <p class="eyebrow">Core Loop</p>
  <h2>主链路先行，不把关键状态散落到 UI</h2>
  <p>
    NeoCode 的 MVP 不是堆功能，而是始终围绕一条可验证的执行闭环构建：
    <code>用户输入 -> Agent 推理 -> 调用工具 -> 获取结果 -> 继续推理 -> UI 展示</code>。
  </p>
  <div class="loop-flow" role="list" aria-label="NeoCode 主链路">
    <div class="loop-step" role="listitem">用户输入</div>
    <div class="loop-step" role="listitem">Agent 推理</div>
    <div class="loop-step" role="listitem">调用工具</div>
    <div class="loop-step" role="listitem">获取结果</div>
    <div class="loop-step" role="listitem">继续推理</div>
    <div class="loop-step" role="listitem">UI 展示</div>
  </div>
</section>

<section class="home-section">
  <p class="eyebrow">Architecture</p>
  <h2>边界清楚，扩展点集中</h2>
  <ArchitectureGrid locale="zh" />
</section>

<section class="home-section quickstart">
  <p class="eyebrow">Quick Start</p>
  <h2>先选入口，再决定怎么接入你的工作流</h2>

  <p>如果你只是想尽快开始使用，优先走安装脚本；如果你准备直接参与开发，再从源码运行。</p>
  <QuickStartCards locale="zh" />
</section>

<section class="home-section">
  <p class="eyebrow">Docs</p>
  <h2>先给入口，不急着迁移现有实现文档</h2>
  <div class="doc-grid">
    <a class="doc-card" href="/neo-code/docs/">
      <strong>文档入口页</strong>
      <span>按主题整理 README、配置、架构、工具与 Gateway 相关文档。</span>
    </a>
    <a class="doc-card" href="https://github.com/1024XEngineer/neo-code/blob/main/README.md">
      <strong>README</strong>
      <span>快速开始、命令示例、配置入口和项目结构总览。</span>
    </a>
    <a class="doc-card" href="https://github.com/1024XEngineer/neo-code/issues">
      <strong>Issues / PRs</strong>
      <span>通过 Issue 与 Pull Request 参与讨论、反馈问题和提交改动。</span>
    </a>
  </div>
</section>
