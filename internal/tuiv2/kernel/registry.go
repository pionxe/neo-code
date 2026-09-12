package kernel

import (
	"fmt"

	"neo-code/internal/tuiv2/state"
)

// bindingRegistry 汇总所有插件贡献的键位绑定，供模式机路由与帮助/快捷键列派生。
// 零值即可用；add 在注册期做重复 (mode, key) 检测（fail-fast，ADR-009 仲裁规则）。
type bindingRegistry struct {
	byModeKey map[state.InputMode]map[string]Binding
	count     int
}

// add 登记一条绑定；同 mode 同 key 重复即报错（含冲突双方描述）。
func (r *bindingRegistry) add(pluginID string, b Binding) error {
	if r.byModeKey == nil {
		r.byModeKey = make(map[state.InputMode]map[string]Binding)
	}
	if b.OnKey == nil {
		return fmt.Errorf("kernel: binding %q (%s) from %s has no OnKey", b.Key, b.Mode, pluginID)
	}
	keys, ok := r.byModeKey[b.Mode]
	if !ok {
		keys = make(map[string]Binding)
		r.byModeKey[b.Mode] = keys
	}
	if prev, dup := keys[b.Key]; dup {
		return fmt.Errorf("kernel: binding conflict on (%s, %q): %s conflicts with %s", b.Mode, b.Key, pluginID, prev.Description)
	}
	keys[b.Key] = b
	r.count++
	return nil
}

// lookup 查询某模式下某键的绑定；未命中返回 false（调用方按独占语义丢弃）。
func (r *bindingRegistry) lookup(mode state.InputMode, key string) (Binding, bool) {
	b, ok := r.byModeKey[mode][key]
	return b, ok
}

// all 返回全部绑定（注册序），供帮助面板自动生成与快捷键派生。
func (r *bindingRegistry) all() []Binding {
	out := make([]Binding, 0, r.count)
	for _, mode := range []state.InputMode{state.InputModeInput, state.NormalMode, state.LeaderMode} {
		for _, b := range r.byModeKey[mode] {
			out = append(out, b)
		}
	}
	return out
}
