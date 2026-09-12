// Package kernel 是 TUI v2 的内核：只提供机制（事件循环、模式机、浮层栈、
// 命令注册表、区域拼装、确认/提示服务），不包含任何业务词汇。
//
// 架构纪律（规范 ADR-007，CI 脚本强制）：
//  1. kernel 禁止 import 插件包（插件单向依赖内核，经 Host 接口）；
//  2. kernel 禁止 import 任何后端模块与 TUI v1（冻结边界词表见
//     scripts/check-tuiv2-boundaries.sh）；
//  3. kernel 源码中禁止出现业务词汇（同上词表）。
package kernel

import (
	"context"
	"errors"

	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// Plugin 是功能插件的唯一必需契约（最小核心，ADR-008）：
// 其余能力经可选接口表达，内核在 Register 时以类型断言发现，
// 未实现的能力不参与对应机制。
type Plugin interface {
	// ID 返回全局唯一插件标识：用于注册冲突检测、日志与归属标注。
	ID() string
	// Init 在内核启动阶段被顺序调用：注册期已收集的键位/命令/区域在此生效，
	// 插件可经 Host 发起初始异步任务（GoCmd）。
	Init(ctx context.Context, h Host)
	// Close 在程序退出阶段被逆序调用（后注册先关闭），插件在此释放资源。
	Close(ctx context.Context)
}

// Reactor 是可选能力：订阅广播。
// 广播内容 = Gateway 事件 + 插件间消息（键与内核私有消息永不入广播，见 4.1 契约）。
// React 必须只做两件事：就地迁移自己拥有的状态槽、经 Host 发起动作；
// 禁止直写他人槽位、禁止二次分发按键（KeyMsg 不会出现在广播中）。
type Reactor interface {
	React(h Host, msg tea.Msg)
}

// KeyBinder 是可选能力：以数据声明键位绑定（取代 switch 路由）。
// 绑定冲突（同 mode 同 key）在注册期 fail-fast，由内核裁定归属。
type KeyBinder interface {
	Bindings() []Binding
}

// Binding 描述一条键位绑定：某键位模式下某键触发某动作。
//
// 两种形态（互斥，注册期校验）：
//   - 精确绑定：Key 非空 + OnKey——匹配 String() 精确相等的按键；
//   - 通配绑定：Key 为空 + OnKeyMsg——匹配该模式下所有未被精确绑定
//     命中的按键（如 Input 模式的任意字符编辑），每 mode 仅限一条
//     （注册期 fail-fast）。帮助派生时显示通配占位，Description 必填。
type Binding struct {
	// Mode 是绑定生效的键位模式（Input/Normal/Leader）。
	Mode state.InputMode
	// Key 是按键的 bubbletea 规范形态（tea.KeyMsg.String()），如 "a"、"enter"、"ctrl+p"、" "。
	// 通配绑定留空。
	Key string
	// Description 是动作的中文一句话说明，供帮助面板自动生成。
	Description string
	// Command 是关联的命令注册表名（可选）：命令面板的快捷键列据此自动派生。
	Command string
	// Group 是帮助面板的分组标题（可选；空 = 按 Mode 平铺）。
	Group string
	// When 是可选守卫：返回 false 时本绑定被跳过（如搜索激活时才捕获按键）。
	// 守卫只读状态，不得写。
	When func(s *state.ViewState) bool
	// OnKey 是精确绑定的动作（与 OnKeyMsg 互斥）：就地迁移自己的槽 / GoCmd / Send / SetMode。
	OnKey func(h Host)
	// OnKeyMsg 是通配绑定的动作（与 OnKey 互斥）：收到原始 KeyMsg，
	// 供输入类插件透传给组件状态机（如字符插入需要按键形状）。
	OnKeyMsg func(h Host, msg tea.KeyMsg)
}

// isWildcard 报告该绑定是否为通配形态。
func (b Binding) isWildcard() bool { return b.Key == "" && b.OnKeyMsg != nil }

// invoke 按绑定形态分派动作。
func (b Binding) invoke(h Host, msg tea.KeyMsg) {
	if b.isWildcard() {
		if b.OnKeyMsg != nil {
			b.OnKeyMsg(h, msg)
		}
		return
	}
	if b.OnKey != nil {
		b.OnKey(h)
	}
}

// CommandProvider 是可选能力：向统一命令注册表贡献命令。
// 同一份数据供三个入口共用：命令面板（搜索列表）、`:` Ex 行（别名解析）、`/` slash（名字路由）。
type CommandProvider interface {
	Commands() []Command
}

// Command 描述一条用户可调用命令。
type Command struct {
	// Name 是用户可见的规范名（带 slash 前缀），如 "/new"。
	Name string
	// Aliases 是 `:` Ex 行接受的别名（不含冒号），如 "new"。
	Aliases []string
	// Description 是中文一句话说明，供命令面板与帮助展示。
	Description string
	// Category 是面板排序分组名（按 Category 稳定排序后按 Name）。
	Category string
	// Shortcut 是快捷键展示列（内核在快照时从绑定注册表预派生，插件留空）。
	Shortcut string
	// Run 是命令体：args 为命令行入口传入的参数（面板入口恒为空）。
	Run func(h Host, args []string)
}

// RegionID 标识主布局的一个区域；每区域唯一所有者（一个 RegionRenderer）。
type RegionID string

const (
	// RegionStatusBar 顶部状态栏。
	RegionStatusBar RegionID = "statusbar"
	// RegionStream 中部行为流。
	RegionStream RegionID = "stream"
	// RegionInspector 右侧软检查器。
	RegionInspector RegionID = "inspector"
	// RegionPrompt 底部输入区。
	RegionPrompt RegionID = "prompt"
	// RegionCmdLine 临时命令/搜索行（渲染空串时该行折叠）。
	RegionCmdLine RegionID = "cmdline"
	// RegionDebug 调试行（--debug 时由内核提供）。
	RegionDebug RegionID = "debug"
)

// regionOrder 是区域的自上而下拼装顺序（compose 的稳定输出依赖它）。
var regionOrder = []RegionID{
	RegionStatusBar,
	RegionStream,
	RegionInspector,
	RegionPrompt,
	RegionCmdLine,
	RegionDebug,
}

// RegionRenderer 是可选能力：渲染主布局的一个区域。
// Render 返回空串表示该区域当前不展示，compose 将折叠该行；
// Render 必须是纯读（同一状态连续调用输出一致），禁止在此变更状态。
type RegionRenderer interface {
	Region() RegionID
	// Render 以给定宽度渲染区域内容（高度由区域状态自持）。
	Render(h Host, width int) string
}

// Overlay 是可选能力：一个浮层对象。
// 浮层的全部交互状态（输入焦点、选中项等）由对象自持，不进入全局 ViewState（ADR-011）。
// 浮层在栈中独占键盘：栈非空时所有按键先交栈顶。
type Overlay interface {
	// ID 返回浮层标识（日志与归属）。
	ID() string
	// HandleKey 处理一个按键（透传原始 KeyMsg，浮层可读按键形状）；
	// 返回 consumed=true 表示已消费（内核停止派发该键）。
	// 约定：栈顶对 "esc" 返回 consumed=false 时，内核弹栈（浮层主动放弃）。
	HandleKey(h Host, msg tea.KeyMsg) (consumed bool)
	// View 以给定宽度渲染浮层（覆盖式，由内核负责居中等外观）。
	View(h Host, width int) string
}

// Host 是插件唯一可见的内核面：插件由此获得全部机制能力（ADR-008）。
// 预期第二实现 = 插件单元测试用的 fake Host（不依赖真终端）。
type Host interface {
	// State 返回全局唯一状态：读任意槽合法；只写自己拥有的槽（ADR-009）。
	State() *state.ViewState
	// Gateway 返回 Gateway 客户端契约（fake/real 对插件透明，ErrUnsupported 语义见 ADR-002）。
	Gateway() gateway.Client
	// GoCmd 发起异步任务（RPC、定时器）：返回值经内核 Update 回流为广播或内核私有消息。
	GoCmd(cmd tea.Cmd)
	// BindEventStream 重绑 Gateway 事件流（S3-2 重绑定协议）：代际号递增使
	// 旧泵产物失效；旧通道由调用方关闭。仅会话切换等换代场景使用。
	BindEventStream(ch <-chan gateway.GatewayEvent)
	// Send 广播一条消息给所有 Reactor：同步、按注册顺序、保序（ADR-009）。
	// 派发期间调用会入队尾延后派发（禁嵌套，见 4.1 契约修订）。
	Send(msg tea.Msg)
	// Mode 返回当前键位模式。
	Mode() state.InputMode
	// SetMode 切换键位模式（内核槽；进入 Leader 模式时内核自动武装超时）。
	SetMode(m state.InputMode)
	// PushOverlay 压入一个浮层：新浮层成为栈顶并独占键盘。
	PushOverlay(o Overlay)
	// PopOverlay 弹出栈顶浮层（栈空时为空操作）。
	PopOverlay()
	// Confirm 发起一次危险操作确认（ADR-012 内核服务）：
	// 内核生成请求 ID 并弹出确认浮层，应答以 state.ConfirmResult 广播回传。
	Confirm(req state.ConfirmRequest)
	// Notify 在状态栏写一条弱提示（内核拥有 Notify 槽），到期由内核定时器清除。
	Notify(text string)
	// Quit 请求退出程序。
	Quit()
	// Commands 返回统一命令注册表快照（Shortcut 已由内核预派生；
	// 列表渲染与别名解析均以此为唯一数据源，ADR-010）。
	Commands() []Command
	// RunCommand 按规范名或别名执行命令（Ex/slash 入口的单一出处）；
	// 未知命令返回错误。
	RunCommand(nameOrAlias string, args []string) error
	// Bindings 返回键位绑定注册表快照（help 自动生成的数据源）。
	Bindings() []Binding
}

// ErrDuplicatePlugin 表示注册了重复的插件 ID。
var ErrDuplicatePlugin = errors.New("kernel: duplicate plugin id")

// ErrDuplicateRegion 表示注册了重复的区域所有者。
var ErrDuplicateRegion = errors.New("kernel: duplicate region renderer")
