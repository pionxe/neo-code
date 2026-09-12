package kernel

import (
	"fmt"

	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// bindingRegistry 汇总所有插件贡献的键位绑定，供模式机路由与帮助/快捷键列派生。
// 零值即可用；add 在注册期做重复 (mode, key) 检测与通配唯一性检测
// （fail-fast，ADR-009 仲裁规则）。
type bindingRegistry struct {
	byModeKey map[state.InputMode]map[string]Binding
	wildcards map[state.InputMode]Binding // 每 mode 至多一条通配（add 保证）
	order     []Binding                   // 登记序快照：all() 的确定性来源（map 遍历序随机）
	count     int
}

// add 登记一条绑定。规则：
//   - OnKey 与 OnKeyMsg 互斥（二选一，否则报错）；
//   - 精确绑定：同 mode 同 key 重复即报错（含冲突双方描述）；
//   - 通配绑定（Key 空 + OnKeyMsg）：同 mode 第二条即报错。
func (r *bindingRegistry) add(pluginID string, b Binding) error {
	if r.byModeKey == nil {
		r.byModeKey = make(map[state.InputMode]map[string]Binding)
	}
	if (b.Key == "") == (b.OnKeyMsg == nil) {
		return fmt.Errorf("kernel: binding from %s must be exactly one of Key+OnKey or wildcard OnKeyMsg", pluginID)
	}
	if b.isWildcard() {
		if b.Description == "" {
			return fmt.Errorf("kernel: wildcard binding from %s requires Description (help derivation)", pluginID)
		}
		if prev, dup := r.wildcards[b.Mode]; dup {
			return fmt.Errorf("kernel: wildcard binding conflict on mode %v: %s conflicts with %s", b.Mode, pluginID, prev.Description)
		}
		if r.wildcards == nil {
			r.wildcards = make(map[state.InputMode]Binding)
		}
		r.wildcards[b.Mode] = b
		r.order = append(r.order, b)
		r.count++
		return nil
	}
	if b.OnKey == nil {
		return fmt.Errorf("kernel: binding %q (%v) from %s has no OnKey", b.Key, b.Mode, pluginID)
	}
	keys, ok := r.byModeKey[b.Mode]
	if !ok {
		keys = make(map[string]Binding)
		r.byModeKey[b.Mode] = keys
	}
	if prev, dup := keys[b.Key]; dup {
		return fmt.Errorf("kernel: binding conflict on (%v, %q): %s conflicts with %s", b.Mode, b.Key, pluginID, prev.Description)
	}
	keys[b.Key] = b
	r.order = append(r.order, b)
	r.count++
	return nil
}

// lookup 查询某模式下的绑定：先精确匹配，未命中再查通配；
// 守卫（When）不通过视为未命中；两者均未命中返回 false
// （调用方按独占语义丢弃）。
func (r *bindingRegistry) lookup(mode state.InputMode, msg tea.KeyMsg, st *state.ViewState) (Binding, bool) {
	if b, ok := r.byModeKey[mode][msg.String()]; ok {
		if b.When == nil || b.When(st) {
			return b, true
		}
	}
	if w, ok := r.wildcards[mode]; ok {
		if w.When == nil || w.When(st) {
			return w, true
		}
	}
	return Binding{}, false
}

// all 返回全部绑定（登记序，确定性；P2 修复：此前 map 遍历序随机，
// "注册序"注释名不副实），供帮助面板自动生成与快捷键派生。
func (r *bindingRegistry) all() []Binding {
	return append([]Binding(nil), r.order...)
}
