// Package layout 是 TUI v2 的布局计算纯函数包（ADR-003：布局计算唯一出处，
// 断点逻辑不得散落组件——S7 全量扩展，issue #50）。
//
// 读时直调形态：消费方（components）渲染期以当前 Width/Height 直调
// Compute，无槽写、无 kernel 触碰（S7 审计裁定：Inspector 隐藏后派生
// 仅依赖已存宽高，写时派生收益消失）。
package layout

// 断点常量（收编自 components/stream.go 旧散点，具名留档）。
const (
	// DefaultVisibleLines 是高度未知（<=0）时的哨兵行数（逐字保留旧语义）。
	DefaultVisibleLines = 8
	// MinVisibleLines 是可见流行数的下限（避免极小终端下空流区）。
	MinVisibleLines = 4
	// StreamReservedRows 是流区域为状态栏/输入区等保留的行数（旧常量随迁）。
	StreamReservedRows = 7
	// StreamHeaderRows 是流区域标题行数（旧常量随迁）。
	StreamHeaderRows = 1
	// MinWidth 是最小可用宽度阈值（低于即 MinSizeBreached；20x5 实测可用为下限参照）。
	MinWidth = 20
	// MinHeight 是最小可用高度阈值。
	MinHeight = 5
)

// Derived 是一次布局计算的派生值集合（字段集=消费方需求反推：
// stream 消费 StreamWidth/VisibleLines；AmbientStatus 消费 MinSizeBreached）。
type Derived struct {
	// StreamWidth 是行为流可用宽度。S7 内恒等于终端宽度——宽屏 Inspector
	// 侧栏公式（width>=100 收窄 33 列）不激活：compose 无横向拼接机制，
	// 激活将产生"stream 窄+inspector 隐藏"形变（横向拼接+恢复登记 S7b）。
	StreamWidth int
	// VisibleLines 是视口可展示的流行数。
	VisibleLines int
	// MinSizeBreached 表示终端尺寸已确认低于最小可用阈值。
	// 0/负宽高=未知，不报（防首帧误报——kernel 首帧前 Width/Height 为零值）。
	MinSizeBreached bool
}

// Compute 是布局计算唯一出处（ADR-003）：纯函数，无副作用，
// 相同输入恒产生相同输出。
func Compute(width, height int) Derived {
	return Derived{
		StreamWidth:  streamWidth(width),
		VisibleLines: visibleLines(height),
		// 哨兵语义：0/负尺寸=未知（kernel 首帧前零值），不判 breach；
		// 正尺寸且低于阈值才报。
		MinSizeBreached: width > 0 && width < MinWidth ||
			height > 0 && height < MinHeight,
	}
}

// streamWidth 计算行为流可用宽度（收编自 components 旧断点：
// width>=100 && ShowInspector 收窄分支不激活——Inspector 隐藏，
// 窄带公式与 100 断点登记 S7b 横向拼接后续项）。
func streamWidth(width int) int {
	return width
}

// visibleLines 计算视口可展示的流行数（收编自 components 旧断点：
// Height<=0→8 哨兵逐字保留；可用行数下限 4）。
func visibleLines(height int) int {
	if height <= 0 {
		return DefaultVisibleLines
	}
	limit := height - StreamReservedRows - StreamHeaderRows
	if limit < MinVisibleLines {
		return MinVisibleLines
	}
	return limit
}
