package layout

import (
	"testing"
)

// TestComputeStreamWidth 验证 StreamWidth：S7 内恒等于终端宽度
// （窄带公式与 100 断点不激活——登记 S7b 横向拼接后续项）。
func TestComputeStreamWidth(t *testing.T) {
	for _, w := range []int{0, 1, 20, 39, 80, 99, 100, 101, 200, 500} {
		if got := Compute(w, 40).StreamWidth; got != w {
			t.Fatalf("width=%d StreamWidth = %d, want %d", w, got, w)
		}
	}
}

// TestComputeVisibleLines 验证 VisibleLines 全语义：
// Height<=0→8 哨兵；小高度→4 下限；正常=Height-8。
func TestComputeVisibleLines(t *testing.T) {
	cases := []struct {
		height int
		want   int
	}{
		{-5, DefaultVisibleLines},
		{0, DefaultVisibleLines},
		{1, MinVisibleLines},
		{5, MinVisibleLines},
		{12, MinVisibleLines},
		{13, 5},
		{20, 12},
		{40, 32},
	}
	for _, tc := range cases {
		if got := Compute(80, tc.height).VisibleLines; got != tc.want {
			t.Fatalf("height=%d VisibleLines = %d, want %d", tc.height, got, tc.want)
		}
	}
}

// TestComputeMinSizeBreached 验证最小尺寸哨兵语义：
// 0/负=未知不报；正尺寸低于阈值才报；边界值 min-1/min/min+1。
func TestComputeMinSizeBreached(t *testing.T) {
	cases := []struct {
		width, height int
		want          bool
	}{
		{0, 0, false},   // 未知不报（首帧零值）
		{0, 40, false},  // 宽未知
		{80, 0, false},  // 高未知
		{-5, -5, false}, // 负值未知
		{19, 20, true},  // 宽 min-1
		{20, 20, false}, // 宽 min
		{21, 20, false}, // 宽 min+1
		{80, 4, true},   // 高 min-1
		{80, 5, false},  // 高 min
		{80, 6, false},  // 高 min+1
		{19, 4, true},   // 双低
	}
	for _, tc := range cases {
		if got := Compute(tc.width, tc.height).MinSizeBreached; got != tc.want {
			t.Fatalf("width=%d height=%d MinSizeBreached = %v, want %v", tc.width, tc.height, got, tc.want)
		}
	}
}

// TestComputePureFunction 验证纯函数性：相同输入重复调用结果一致，
// 且不修改调用方状态（无法直接断言，但锁定输出稳定性防隐含状态）。
func TestComputePureFunction(t *testing.T) {
	a := Compute(80, 24)
	b := Compute(80, 24)
	if a != b {
		t.Fatalf("pure function violated: %+v != %+v", a, b)
	}
}
