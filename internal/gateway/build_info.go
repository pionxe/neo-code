package gateway

import "strings"

var (
	// GatewayVersion 表示网关构建版本，可通过 -ldflags 覆盖。
	GatewayVersion = "dev"
	// GatewayCommit 表示网关构建提交号，可通过 -ldflags 覆盖。
	GatewayCommit = "unknown"
	// GatewayBuildTime 表示网关构建时间，可通过 -ldflags 覆盖。
	GatewayBuildTime = ""
)

// ResolvedBuildInfo 返回归一化后的网关构建信息。
func ResolvedBuildInfo() map[string]string {
	buildTime := strings.TrimSpace(GatewayBuildTime)
	return map[string]string{
		"version":    strings.TrimSpace(GatewayVersion),
		"commit":     strings.TrimSpace(GatewayCommit),
		"build_time": buildTime,
	}
}
