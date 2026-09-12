package kernel

// overlayStack 是浮层栈（ADR-011）：内核只管栈与焦点，浮层的交互状态由
// Overlay 对象自持（不进入全局 ViewState）。
//
// 焦点规则（§4.1 键路由第 2 级）：栈非空时所有按键先交栈顶——
//   - "esc"：栈顶返回 consumed=false 时弹栈（浮层主动放弃），否则消费；
//   - 其余键：栈顶未消费即丢弃（独占，不穿透到键位绑定）。
type overlayStack struct {
	items []Overlay
}

// push 压入浮层并使其成为新栈顶；Host 无需存储——接口方法按调用注入。
func (s *overlayStack) push(o Overlay) {
	s.items = append(s.items, o)
}

// pop 弹出栈顶浮层；栈空为空操作。
func (s *overlayStack) pop() {
	if len(s.items) == 0 {
		return
	}
	s.items = s.items[:len(s.items)-1]
}

// top 返回栈顶浮层；栈空返回 nil。
func (s *overlayStack) top() Overlay {
	if len(s.items) == 0 {
		return nil
	}
	return s.items[len(s.items)-1]
}

// depth 返回当前栈深（调试行展示用）。
func (s *overlayStack) depth() int {
	return len(s.items)
}
