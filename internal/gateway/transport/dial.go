package transport

import (
	"net"
	"time"
)

const defaultIPCDialTimeout = 3 * time.Second

// Dial 连接到本地网关 IPC 地址，按平台选择 UDS 或 Named Pipe。
func Dial(address string) (net.Conn, error) {
	return dial(address)
}
