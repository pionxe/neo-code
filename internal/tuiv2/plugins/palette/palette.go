// Package palette 是 TUI v2 的命令面板插件（issue #27 S3-3）：
// Overlay 型插件——渲染统一命令注册表快照（Host.Commands），
// 提供过滤/选择/执行；数据源唯一（kernel 注册表），自身无命令定义。
// 浮层状态（过滤词/选中项）由 overlay 对象自持（ADR-011）。
package palette

import (
	"context"
	"strings"

	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// Plugin 是命令面板插件（状态在浮层对象上，插件本体无交互状态）。
type Plugin struct{}

// New 创建命令面板插件实例。
func New() *Plugin { return &Plugin{} }

// ID 返回插件标识。
func (p *Plugin) ID() string { return "palette" }

// Init 生命周期占位（打开动作在键位 OnKey 内完成）。
func (p *Plugin) Init(ctx context.Context, h kernel.Host) {
	_ = ctx
}

// Close 释放资源（无外部资源，生命周期对称性）。
func (p *Plugin) Close(ctx context.Context) {
	_ = ctx
}

// open 构造并压入面板浮层：快照命令表（Category→Name 排序 + 快捷键列
// 内核预派生），重置过滤与选中。
func (p *Plugin) open(h kernel.Host) {
	h.PushOverlay(&overlay{cmds: h.Commands()})
}

// overlay 是命令面板浮层：全部交互状态自持（ADR-011）。
type overlay struct {
	cmds     []kernel.Command // 打开时的命令快照
	query    string           // 过滤输入
	selected int              // 当前选中索引
}

// ID 返回浮层标识。
func (o *overlay) ID() string { return "palette" }

// filtered 按查询过滤命令（确定性分桶：精确 > 前缀 > 描述包含，无评分——
// 沿用 issue #23 裁决，避免 mode 误命中 model 类问题）。
func (o *overlay) filtered() []kernel.Command {
	if o.query == "" {
		return o.cmds
	}
	lq := strings.ToLower(o.query)
	var exact, prefix, contain []kernel.Command
	for _, c := range o.cmds {
		name := strings.ToLower(strings.TrimPrefix(c.Name, "/"))
		switch {
		case name == lq:
			exact = append(exact, c)
		case strings.HasPrefix(name, lq) || strings.HasPrefix(strings.ToLower(c.Name), lq):
			prefix = append(prefix, c)
		case strings.Contains(strings.ToLower(c.Description), lq):
			contain = append(contain, c)
		}
	}
	return append(append(exact, prefix...), contain...)
}

// HandleKey 处理面板按键：up/down 选择、enter 执行、esc 关闭、
// 可打印字符追加过滤、backspace 回删；未识别键一律消费（模态）。
func (o *overlay) HandleKey(h kernel.Host, msg tea.KeyMsg) (consumed bool) {
	matched := o.filtered()
	switch msg.String() {
	case "esc":
		h.PopOverlay()
		return true
	case "up":
		if o.selected > 0 {
			o.selected--
		}
		return true
	case "down":
		if o.selected < len(matched)-1 {
			o.selected++
		}
		return true
	case "enter":
		if o.selected < len(matched) {
			c := matched[o.selected]
			h.PopOverlay()
			if c.Run != nil { // 脏数据防御：无动作命令不 panic
				c.Run(h, nil)
			}
		}
		return true
	case "backspace":
		if r := []rune(o.query); len(r) > 0 {
			o.query = string(r[:len(r)-1])
			o.selected = 0
		}
		return true
	}
	if len(msg.Runes) > 0 {
		o.query += string(msg.Runes)
		o.selected = 0
	}
	return true
}

// HandleMouse 消费面板区域鼠标事件（S8，issue #52；实现
// kernel.OverlayMouseHandler——kernel 栈顶独占语义自动路由）：
//   - 滚轮上/下 → selected 滚动（与 up/down 键一致，不含 backspace）
//   - 左键点击 → 选择并执行（Y-2 锚定：面板从屏第 0 行渲染，
//     header 行占 2 行——Y-2 即列表首行索引）
//   - Motion/右键等丢弃
func (o *overlay) HandleMouse(h kernel.Host, msg tea.MouseMsg) (consumed bool) {
	matched := o.filtered()
	switch {
	case msg.Button == tea.MouseButtonWheelUp:
		if o.selected > 0 {
			o.selected--
		}
		return true
	case msg.Button == tea.MouseButtonWheelDown:
		if o.selected < len(matched)-1 {
			o.selected++
		}
		return true
	case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
		// Y-2 = 列表首行（屏第 0 行起渲染：header 占 2 行）。
		idx := msg.Y - 2
		if idx >= 0 && idx < len(matched) {
			o.selected = idx
			h.PopOverlay()
			if c := matched[idx]; c.Run != nil {
				c.Run(h, nil)
			}
		}
		return true
	default:
		return true // 模态消费：面板开启期间所有鼠标事件不穿透
	}
}

// View 渲染面板（┌ 边框为命令面板允许的少数场景之一）。
func (o *overlay) View(h kernel.Host, width int) string {
	matched := o.filtered()
	var b strings.Builder
	b.WriteString("┌─ ⚡ 命令面板 ─────────────────────────────┐\n")
	b.WriteString("│ › " + o.query + "\n")
	for i, c := range matched {
		marker := "  "
		if i == o.selected {
			marker = "▎ "
		}
		line := marker + c.Name + "  " + c.Description
		if c.Shortcut != "" {
			line += "  (" + c.Shortcut + ")"
		}
		b.WriteString("│ " + line + "\n")
	}
	if len(matched) == 0 {
		b.WriteString("│  无匹配命令\n")
	}
	b.WriteString("└──────────────────────────────────────────┘")
	return b.String()
}

// Bindings 声明面板打开键：Leader p（打开动作经 Host.PushOverlay）。
func (p *Plugin) Bindings() []kernel.Binding {
	return []kernel.Binding{
		{
			Mode:        state.LeaderMode,
			Key:         "p",
			Description: "打开命令面板",
			OnKey: func(h kernel.Host) {
				p.open(h)
			},
		},
	}
}
