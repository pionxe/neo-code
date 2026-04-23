package session

import (
	"crypto/rand"
	"encoding/hex"
)

// NewID 生成带前缀的随机 ID，格式为 "<prefix>_<16hex>"。
func NewID(prefix string) string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return prefix + "_" + hex.EncodeToString(buf)
}
