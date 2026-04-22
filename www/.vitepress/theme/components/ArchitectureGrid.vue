<script setup lang="ts">
defineProps<{
  locale?: 'zh' | 'en'
}>()
</script>

<template>
  <div class="info-grid">
    <div class="info-card architecture-block">
      <h3>{{ locale === 'en' ? 'Core layering' : '核心分层' }}</h3>
      <pre><code>TUI -&gt; Runtime -&gt; Provider / Tool Manager</code></pre>
      <p v-if="locale === 'en'">
        New capabilities are expected to join the main path, not bypass it with ad-hoc UI logic or direct tool execution.
      </p>
      <p v-else>
        新能力默认沿主链路接入，不在 TUI 或 runtime 里直接写工具执行逻辑。
      </p>
    </div>

    <div class="info-card">
      <h3>{{ locale === 'en' ? 'Key modules' : '关键模块' }}</h3>
      <div class="info-lines" v-if="locale === 'en'">
        <p><code>internal/runtime</code>: ReAct loop, event stream, tool-call orchestration</p>
        <p><code>internal/tools</code>: tool contracts, validation, execution, and result normalization</p>
        <p><code>internal/provider</code>: model protocol differences, request shaping, response parsing</p>
        <p><code>internal/tui</code>: Bubble Tea state machine and runtime event rendering</p>
      </div>
      <div class="info-lines" v-else>
        <p><code>internal/runtime</code>：ReAct 主循环、事件流、tool call 编排</p>
        <p><code>internal/tools</code>：工具契约、参数校验、执行与结果收敛</p>
        <p><code>internal/provider</code>：模型协议差异、请求组装与响应解析</p>
        <p><code>internal/tui</code>：Bubble Tea 状态机与 runtime 事件展示</p>
      </div>
    </div>
  </div>
</template>
