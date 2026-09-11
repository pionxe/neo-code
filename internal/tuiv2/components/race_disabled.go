//go:build !race

package components

// raceEnabled 标识当前是否以 -race 竞态检测模式构建（非 race 构建恒为 false）。
// 竞态插桩会使执行耗时放大数倍至数十倍，墙钟类性能预算断言在该模式下失真，
// 相关测试应据此跳过（见 TestAgentStreamLargeStreamRenderBudget）。
const raceEnabled = false
