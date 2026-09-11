package components

import "sort"

// PaletteAction 是命令面板动作的具名常量，消除 handlePaletteCommand 的硬编码字符串。
type PaletteAction string

const (
	PaletteActionNewSession    PaletteAction = "new_session"
	PaletteActionSwitchSession PaletteAction = "switch_session"
	PaletteActionDeleteSession PaletteAction = "delete_session"
	PaletteActionModel         PaletteAction = "model"
	PaletteActionRetry         PaletteAction = "retry"
	PaletteActionCancel        PaletteAction = "cancel"
	PaletteActionHelp          PaletteAction = "help"
	PaletteActionClear         PaletteAction = "clear"
	PaletteActionDebug         PaletteAction = "debug"
	PaletteActionSessionInfo   PaletteAction = "session_info"
	PaletteActionCompact       PaletteAction = "compact"
	PaletteActionMode          PaletteAction = "mode"
	PaletteActionExit          PaletteAction = "exit"
	PaletteActionCheckpoint    PaletteAction = "checkpoint" // 未实现
	PaletteActionSkills        PaletteAction = "skills"     // 未实现
)

// Category 描述命令分类，其整数值即空查询时的排序优先级（越小越靠前）。
type Category int

const (
	CategorySession Category = iota // 会话相关优先展示
	CategoryModel
	CategoryRun
	CategoryHelp
	CategorySystem
)

// CommandDef 描述一个命令面板项。
type CommandDef struct {
	Name        string // "/model" 等用户可见标识
	Description string // 中文描述（未实现命令加 [未实现] 后缀）
	Category    Category
	Shortcut    string // "Space m" 等；空表示无快捷键
	Action      PaletteAction
}

// paletteCommands 是命令面板的命令定义列表。
//
// Shortcut 字段需与 keymap.MatchLeaderKey 保持一致（修改键位时同步更新）。
var paletteCommands = []CommandDef{
	{Name: "/new", Description: "新建会话", Category: CategorySession, Shortcut: "Space n", Action: PaletteActionNewSession},
	{Name: "/session", Description: "切换会话", Category: CategorySession, Shortcut: "Space s", Action: PaletteActionSwitchSession},
	{Name: "/delete", Description: "删除当前会话", Category: CategorySession, Action: PaletteActionDeleteSession},
	{Name: "/model", Description: "切换模型", Category: CategoryModel, Shortcut: "Space m", Action: PaletteActionModel},
	{Name: "/retry", Description: "重试上次运行", Category: CategoryRun, Shortcut: "Space r", Action: PaletteActionRetry},
	{Name: "/cancel", Description: "取消当前运行", Category: CategoryRun, Shortcut: "Space c", Action: PaletteActionCancel},
	{Name: "/help", Description: "查看帮助", Category: CategoryHelp, Shortcut: "Space h", Action: PaletteActionHelp},
	{Name: "/clear", Description: "清空会话", Category: CategorySystem, Action: PaletteActionClear},
	{Name: "/debug", Description: "切换调试模式", Category: CategorySystem, Shortcut: ":debug", Action: PaletteActionDebug},
	{Name: "/info", Description: "查看会话信息", Category: CategorySystem, Action: PaletteActionSessionInfo},
	{Name: "/compact", Description: "手动 compact", Category: CategorySystem, Action: PaletteActionCompact},
	{Name: "/mode", Description: "切换 Agent 模式", Category: CategorySystem, Action: PaletteActionMode},
	{Name: "/exit", Description: "退出", Category: CategorySystem, Shortcut: "Space q", Action: PaletteActionExit},
	{Name: "/checkpoint", Description: "管理检查点 [未实现]", Category: CategorySystem, Action: PaletteActionCheckpoint},
	{Name: "/skills", Description: "管理会话技能 [未实现]", Category: CategorySystem, Action: PaletteActionSkills},
}

// PaletteCommands 返回按 Category 优先级排序的命令列表（空查询时使用）。
func PaletteCommands() []CommandDef {
	out := append([]CommandDef(nil), paletteCommands...)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Category < out[j].Category
	})
	return out
}
