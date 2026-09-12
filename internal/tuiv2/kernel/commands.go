package kernel

import (
	"fmt"
	"sort"

	"neo-code/internal/tuiv2/state"
)

// commandRegistry 是统一命令表：面板 / `:` Ex 行 / `/` slash 三个入口共用
// 同一份数据（ADR-010），消灭"同一命令多处登记"的人肉同步。
type commandRegistry struct {
	byName map[string]*Command // 规范名与别名 → 命令（别名不含冒号）
	list   []Command           // 注册序（供面板空查询时的基础顺序）
}

// add 登记一条命令；规范名或任一别名与既有条目冲突即报错。
func (r *commandRegistry) add(pluginID string, c Command) error {
	if r.byName == nil {
		r.byName = make(map[string]*Command)
	}
	if c.Name == "" {
		return fmt.Errorf("kernel: command from %s has empty name", pluginID)
	}
	if c.Run == nil {
		return fmt.Errorf("kernel: command %s from %s has no Run", c.Name, pluginID)
	}
	names := append([]string{c.Name}, c.Aliases...)
	for _, name := range names {
		if prev, dup := r.byName[name]; dup {
			return fmt.Errorf("kernel: command name conflict on %q: %s conflicts with %s", name, pluginID, prev.Name)
		}
	}
	for i := range names {
		r.byName[names[i]] = &c
	}
	r.list = append(r.list, c)
	return nil
}

// lookup 按 slash 规范名或 Ex 别名解析命令（`/` 与 `:` 两入口共用此解析）。
func (r *commandRegistry) lookup(nameOrAlias string) (*Command, bool) {
	c, ok := r.byName[nameOrAlias]
	return c, ok
}

// sorted 返回按 Category 再 Name 稳定排序的命令列表（面板展示顺序）。
func (r *commandRegistry) sorted() []Command {
	out := append([]Command(nil), r.list...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// 模式前缀是快捷键派生的展示词汇：帮助与命令面板右侧列共用。
var modeShortcutPrefix = map[state.InputMode]string{
	state.InputModeInput: "i ",
	state.NormalMode:     "",
	state.LeaderMode:     "Space ",
}

// deriveShortcut 从绑定注册表为命令派生快捷键展示列（ADR-010 自动派生，
// 消灭 // keep in sync with keymap 类注释）。多个绑定时按注册序以 ", " 连接；
// 无绑定时返回空串（面板不展示快捷键列）。
func deriveShortcut(commandName string, bindings *bindingRegistry) string {
	out := ""
	for _, b := range bindings.all() {
		if b.Command != commandName {
			continue
		}
		if out != "" {
			out += ", "
		}
		out += modeShortcutPrefix[b.Mode] + b.Key
	}
	return out
}
