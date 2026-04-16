package types

const (
	// MaxSessionAssetBytes 定义 session_asset 在读写链路中的统一大小上限（20 MiB）。
	MaxSessionAssetBytes int64 = 20 * 1024 * 1024
)
