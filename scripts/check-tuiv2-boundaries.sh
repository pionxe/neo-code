#!/usr/bin/env bash
# TUI v2 架构边界检查（规范 ADR-007 lint 三禁）：
#   禁 1：kernel 包不得出现业务词汇（内核只有机制，业务在插件）；
#   禁 2：plugins/* 之间禁止互相 import（插件互不认识）；
#   禁 3：kernel 禁止 import plugins/*（机制不依赖功能）；
#   禁 4：tuiv2 禁止 import 后端模块与 TUI v1（Phase 0 冻结边界）。
# 任何一条命中即退出码 1（CI fail-fast）。用法：bash scripts/check-tuiv2-boundaries.sh

set -u
cd "$(dirname "$0")/.."

fail=0

# 禁 1：业务词表。注意：inspector/prompt/cmdline 等是内核拥有的区域词汇
# （RegionID 常量），不在禁列表；词表只收真正属于业务域的词汇。
if grep -riE '(session|permission|palette|assistant|checkpoint|skill|agent_chunk)' \
    internal/tuiv2/kernel --include='*.go'; then
    echo "[边界-1] kernel 出现业务词汇（见上方命中行）" >&2
    fail=1
fi

# 禁 2：插件互不 import。plugins 目录尚不存在时视为通过（守卫未来）。
if [ -d internal/tuiv2/plugins ]; then
    for plugin_dir in internal/tuiv2/plugins/*/; do
        [ -d "$plugin_dir" ] || continue
        plugin_name=$(basename "$plugin_dir")
        if grep -rE "neo-code/internal/tuiv2/plugins/" "$plugin_dir" --include='*.go' \
            | grep -v "plugins/${plugin_name}/" >/dev/null 2>&1; then
            echo "[边界-2] 插件 ${plugin_name} import 了其他插件（见上方命中行）" >&2
            fail=1
        fi
    done
fi

# 禁 3：kernel 不 import plugins。
if grep -rn 'neo-code/internal/tuiv2/plugins' internal/tuiv2/kernel --include='*.go'; then
    echo "[边界-3] kernel import 了 plugins（见上方命中行）" >&2
    fail=1
fi

# 禁 4：后端模块与 TUI v1 冻结边界。
# 锚定形式含 "/" 后继或行尾引号，防子包绕过（如 internal/gateway/client 之外
# 的 internal/gateway 子包）。
# S5 演进（issue #46，ADR-004）：放行 neo-code/internal/gateway/client——
# RealClient 只读复用 v1 RPC 客户端（认证/心跳/重试/通知分发内建），
# 依赖闭包已经审计核实干净（.s5-realclient-audit/plan-r1-claude2.md）；
# 服务端（internal/gateway 自身）与其余后端模块仍冻结。
if grep -rnE 'neo-code/internal/(gateway|runtime|session|repository|tui|config|context|provider|tools)("|[[:space:]]*$|/)' \
    internal/tuiv2 cmd/neocode-tuiv2 --include='*.go' \
    | grep -v 'neo-code/internal/tuiv2/gateway' \
    | grep -v 'neo-code/internal/gateway/client' ; then
    echo "[边界-4] tuiv2 import 了后端模块或 TUI v1（见上方命中行）" >&2
    fail=1
fi

if [ "$fail" -ne 0 ]; then
    echo "TUI v2 边界检查：未通过" >&2
    exit 1
fi
echo "TUI v2 边界检查：通过"
