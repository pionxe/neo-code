---
layout: home

hero:
  name: NeoCode
  text: A local Go + Bubble Tea coding agent
  tagline: Reason, call tools, observe results, and keep the task moving inside the terminal.
  image:
    src: /brand/neocode-mark.svg
    alt: NeoCode
  actions:
    - theme: brand
      text: Docs
      link: /en/docs/
    - theme: alt
      text: GitHub
      link: https://github.com/1024XEngineer/neo-code
    - theme: alt
      text: 中文
      link: /

features:
  - title: Terminal-native interaction
    details: Bubble Tea keeps the conversation, command flow, and runtime feedback inside a local TUI instead of bouncing across browser tabs.
  - title: ReAct loop as the main path
    details: Runtime orchestration keeps user input, tool calls, tool results, and final output on the same verifiable path.
  - title: Provider differences stay isolated
    details: Vendor-specific protocol details are contained inside the provider layer instead of leaking into runtime or TUI code.
  - title: Stateful without turning opaque
    details: Context compaction, session persistence, and workdir isolation help long-running conversations stay useful and inspectable.
---

<section class="home-section">
  <p class="eyebrow">Core Loop</p>
  <h2>Built around a verifiable execution path</h2>
  <p>
    NeoCode's MVP keeps one path working end-to-end instead of scattering behavior across the UI:
    <code>User Input -> Agent Reasoning -> Tool Call -> Result -> Continue Reasoning -> UI Output</code>.
  </p>
  <div class="loop-flow" role="list" aria-label="NeoCode core loop">
    <div class="loop-step" role="listitem">User Input</div>
    <div class="loop-step" role="listitem">Reasoning</div>
    <div class="loop-step" role="listitem">Tool Call</div>
    <div class="loop-step" role="listitem">Result</div>
    <div class="loop-step" role="listitem">Continue</div>
    <div class="loop-step" role="listitem">UI Output</div>
  </div>
</section>

<section class="home-section">
  <p class="eyebrow">Architecture</p>
  <h2>Clear boundaries, deliberate extension points</h2>
  <ArchitectureGrid locale="en" />
</section>

<section class="home-section quickstart">
  <p class="eyebrow">Quick Start</p>
  <h2>Pick an entry point, then fit it into your workflow</h2>

  <p>If you want to try NeoCode quickly, use the install script first. If you want to work on the codebase itself, run it from source.</p>
  <QuickStartCards locale="en" />
</section>

<section class="home-section">
  <p class="eyebrow">Docs</p>
  <h2>Use the site as an entry point, not a duplicate docs tree</h2>
  <div class="doc-grid">
    <a class="doc-card" href="/neo-code/en/docs/">
      <strong>Docs Index</strong>
      <span>Entry points to the README, configuration guides, architecture notes, and gateway design docs.</span>
    </a>
    <a class="doc-card" href="https://github.com/1024XEngineer/neo-code/blob/main/README.md">
      <strong>README</strong>
      <span>Quick start, commands, configuration entry points, and a high-level project map.</span>
    </a>
    <a class="doc-card" href="https://github.com/1024XEngineer/neo-code/issues">
      <strong>Issues / PRs</strong>
      <span>Track ongoing work, report defects, and contribute implementation changes.</span>
    </a>
  </div>
</section>
