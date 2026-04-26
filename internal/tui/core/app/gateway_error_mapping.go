package tui

import (
	"errors"
	"strings"

	"neo-code/internal/gateway/protocol"
	tuiservices "neo-code/internal/tui/services"
)

// isGatewayUnsupportedActionError 统一判断网关错误是否表示“当前动作不受支持”。
func isGatewayUnsupportedActionError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, tuiservices.ErrUnsupportedActionInGatewayMode) {
		return true
	}

	var rpcErr *tuiservices.GatewayRPCError
	if !errors.As(err, &rpcErr) || rpcErr == nil {
		return false
	}

	if strings.EqualFold(strings.TrimSpace(rpcErr.GatewayCode), protocol.GatewayCodeUnsupportedAction) {
		return true
	}
	return rpcErr.Code == protocol.JSONRPCCodeMethodNotFound
}
