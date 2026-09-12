package kernel

import "strings"

// Compose 自上而下拼装主布局：按 regionOrder 顺序向各区域所有者索取渲染，
// 返回空串的区域被折叠（ADR-003 的最小机制：无"临时认领"，零状态）。
// Render 为纯读，本函数不修改任何状态，重复调用输出一致（I6 幂等不变量）。
func Compose(h Host, renderers map[RegionID]RegionRenderer, width int) string {
	lines := make([]string, 0, len(regionOrder))
	for _, region := range regionOrder {
		r, ok := renderers[region]
		if !ok {
			continue
		}
		out := r.Render(h, width)
		if out == "" {
			continue
		}
		lines = append(lines, out)
	}
	return strings.Join(lines, "\n")
}
