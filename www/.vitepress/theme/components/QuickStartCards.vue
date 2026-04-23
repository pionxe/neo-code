<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
	locale?: 'zh' | 'en'
}>()

const isEnglish = computed(() => props.locale === 'en')

const installUnix = 'curl -fsSL https://raw.githubusercontent.com/1024XEngineer/neo-code/main/scripts/install.sh | bash'
const installWindows = 'irm https://raw.githubusercontent.com/1024XEngineer/neo-code/main/scripts/install.ps1 | iex'
const runBinary = `neocode`
const fromSource = `git clone https://github.com/1024XEngineer/neo-code.git
cd neo-code
go run ./cmd/neocode`
const envUnix = `export OPENAI_API_KEY="your_key_here"
export GEMINI_API_KEY="your_key_here"
export AI_API_KEY="your_key_here"
export QINIU_API_KEY="your_key_here"`
const envWindows = `$env:OPENAI_API_KEY = "your_key_here"
$env:GEMINI_API_KEY = "your_key_here"
$env:AI_API_KEY = "your_key_here"
$env:QINIU_API_KEY = "your_key_here"`
</script>

<template>
  <div class="quickstart-grid" v-if="isEnglish">
    <article class="quickstart-card">
      <p class="quickstart-kicker">Install</p>
      <h3>Install with one command</h3>
      <p>Use the same install scripts maintained in the repository README.</p>
      <CodePanel language="bash" label="macOS / Linux" :code="installUnix" />
      <CodePanel language="powershell" label="Windows PowerShell" :code="installWindows" />
      <div class="quickstart-links">
        <p>Then run: <code>neocode</code></p>
      </div>
    </article>

    <article class="quickstart-card">
      <p class="quickstart-kicker">Source</p>
      <h3>Run the current codebase</h3>
      <p>Best when you want to inspect behavior, debug issues, or contribute changes.</p>
      <CodePanel language="bash" label="Clone and run" :code="fromSource" />
      <div class="quickstart-links">
        <p>Gateway command: <code>go run ./cmd/neocode gateway</code></p>
        <p>Workspace isolation: <code>--workdir /path/to/workspace</code></p>
      </div>
    </article>
  </div>

  <div class="quickstart-grid" v-else>
    <article class="quickstart-card">
      <p class="quickstart-kicker">安装脚本</p>
      <h3>安装 NeoCode</h3>
      <p>直接使用仓库里维护的安装脚本，适合先把本地环境跑起来。</p>
      <CodePanel language="bash" label="macOS / Linux" :code="installUnix" />
      <CodePanel language="powershell" label="Windows PowerShell" :code="installWindows" />
      <div class="quickstart-links">
        <p>安装完成后运行：<code>neocode</code></p>
      </div>
    </article>

    <article class="quickstart-card">
      <p class="quickstart-kicker">源码运行</p>
      <h3>从源码运行</h3>
      <p>准备调试、看源码或参与开发时，直接运行当前仓库即可。</p>
      <CodePanel language="bash" label="Clone and run" :code="fromSource" />
      <div class="quickstart-links">
        <p>网关进程：<code>go run ./cmd/neocode gateway</code></p>
        <p>会话工作区：<code>--workdir /path/to/workspace</code></p>
      </div>
    </article>
  </div>
</template>
