package urlscheme

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"neo-code/internal/gateway"
	"neo-code/internal/gateway/protocol"
	"neo-code/internal/gateway/transport"
)

const (
	// ErrorCodeGatewayUnavailable 表示无法连接本地网关进程。
	ErrorCodeGatewayUnavailable = "gateway_unreachable"
	// ErrorCodeUnexpectedResponse 表示网关返回了不符合预期的帧结构。
	ErrorCodeUnexpectedResponse = "unexpected_response"
	// ErrorCodeNotSupported 表示当前平台暂未实现目标能力。
	ErrorCodeNotSupported = "not_supported"
	// ErrorCodeInternal 表示调度器内部错误。
	ErrorCodeInternal = "internal_error"
	// defaultDispatchIOTimeout 表示单次调度读写超时时间。
	defaultDispatchIOTimeout = 10 * time.Second
)

var dispatchRequestCounter uint64

// DispatchRequest 表示 URL Scheme 调度输入参数。
type DispatchRequest struct {
	RawURL        string
	ListenAddress string
	AuthToken     string
}

// DispatchResult 表示 URL Scheme 调度输出。
type DispatchResult struct {
	ListenAddress string               `json:"listen_address"`
	Request       gateway.MessageFrame `json:"request"`
	Response      gateway.MessageFrame `json:"response"`
}

// DispatchError 表示调度过程中的结构化错误。
type DispatchError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error 返回调度错误文本。
func (e *DispatchError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Dispatcher 负责将 neocode:// URL 转发到网关本地控制面。
type Dispatcher struct {
	resolveListenAddressFn func(string) (string, error)
	dialFn                 func(address string) (net.Conn, error)
	requestIDFn            func() string
}

// NewDispatcher 创建默认 URL Scheme 调度器。
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		resolveListenAddressFn: transport.ResolveListenAddress,
		dialFn:                 transport.Dial,
		requestIDFn:            nextDispatchRequestID,
	}
}

// Dispatch 将 URL 映射为 wake.openUrl 请求，并通过 IPC 转发到网关。
func (d *Dispatcher) Dispatch(ctx context.Context, request DispatchRequest) (DispatchResult, error) {
	intent, err := protocol.ParseNeoCodeURL(request.RawURL)
	if err != nil {
		return DispatchResult{}, toDispatchError(err)
	}

	listenAddress, err := d.resolveListenAddressFn(request.ListenAddress)
	if err != nil {
		return DispatchResult{}, newDispatchError(ErrorCodeInternal, fmt.Sprintf("resolve listen address: %v", err))
	}

	conn, err := d.dialFn(listenAddress)
	if err != nil {
		return DispatchResult{}, newDispatchError(ErrorCodeGatewayUnavailable, fmt.Sprintf("dial gateway failed: %v", err))
	}
	defer func() {
		_ = conn.Close()
	}()

	if err := applyDispatchDeadline(conn, ctx); err != nil {
		return DispatchResult{}, newDispatchError(ErrorCodeInternal, fmt.Sprintf("set connection deadline: %v", err))
	}
	if err := ensureDispatchContextActive(ctx); err != nil {
		return DispatchResult{}, toDispatchError(err)
	}
	stopCancelWatcher := watchDispatchCancellation(ctx, conn)
	defer stopCancelWatcher()

	authToken := strings.TrimSpace(request.AuthToken)
	if authToken != "" {
		if err := d.authenticate(ctx, conn, authToken); err != nil {
			return DispatchResult{}, err
		}
	}

	requestFrame := gateway.MessageFrame{
		Type:      gateway.FrameTypeRequest,
		Action:    gateway.FrameActionWakeOpenURL,
		RequestID: d.requestIDFn(),
		SessionID: intent.SessionID,
		Workdir:   intent.Workdir,
		Payload:   intent,
	}

	requestIDRaw, err := marshalJSONRawMessage(requestFrame.RequestID)
	if err != nil {
		return DispatchResult{}, newDispatchError(ErrorCodeInternal, fmt.Sprintf("encode request id: %v", err))
	}
	requestParamsRaw, err := marshalJSONRawMessage(intent)
	if err != nil {
		return DispatchResult{}, newDispatchError(ErrorCodeInternal, fmt.Sprintf("encode request params: %v", err))
	}
	rpcRequest := protocol.JSONRPCRequest{
		JSONRPC: protocol.JSONRPCVersion,
		ID:      requestIDRaw,
		Method:  protocol.MethodWakeOpenURL,
		Params:  requestParamsRaw,
	}

	if err := ensureDispatchContextActive(ctx); err != nil {
		return DispatchResult{}, toDispatchError(err)
	}
	rpcResponse, err := d.callRPC(ctx, conn, rpcRequest)
	if err != nil {
		return DispatchResult{}, err
	}
	if strings.TrimSpace(rpcResponse.JSONRPC) != protocol.JSONRPCVersion {
		return DispatchResult{}, newDispatchError(
			ErrorCodeUnexpectedResponse,
			"unexpected response jsonrpc version",
		)
	}
	if !rawJSONMessageEqual(rpcResponse.ID, rpcRequest.ID) {
		return DispatchResult{}, newDispatchError(ErrorCodeUnexpectedResponse, "rpc correlation failed: id mismatch")
	}
	if rpcResponse.Error != nil && rpcResponse.Result != nil {
		return DispatchResult{}, newDispatchError(
			ErrorCodeUnexpectedResponse,
			"unexpected response payload: both result and error are present",
		)
	}
	if rpcResponse.Error != nil {
		return DispatchResult{}, toDispatchErrorFromJSONRPC(rpcResponse.Error)
	}
	if rpcResponse.Result == nil {
		return DispatchResult{}, newDispatchError(ErrorCodeUnexpectedResponse, "gateway response missing result payload")
	}

	responseFrame, err := decodeResponseFrameResult(rpcResponse.Result)
	if err != nil {
		return DispatchResult{}, newDispatchError(ErrorCodeUnexpectedResponse, fmt.Sprintf("decode response frame: %v", err))
	}
	if responseFrame.Action != requestFrame.Action || responseFrame.RequestID != requestFrame.RequestID {
		return DispatchResult{}, newDispatchError(
			ErrorCodeUnexpectedResponse,
			"frame correlation failed: action or request_id mismatch",
		)
	}

	switch responseFrame.Type {
	case gateway.FrameTypeAck:
		return DispatchResult{
			ListenAddress: listenAddress,
			Request:       requestFrame,
			Response:      responseFrame,
		}, nil
	case gateway.FrameTypeError:
		if responseFrame.Error == nil {
			return DispatchResult{}, newDispatchError(ErrorCodeUnexpectedResponse, "gateway error frame missing error payload")
		}
		return DispatchResult{}, newDispatchError(responseFrame.Error.Code, responseFrame.Error.Message)
	default:
		return DispatchResult{}, newDispatchError(ErrorCodeUnexpectedResponse, fmt.Sprintf("unsupported response frame type: %s", responseFrame.Type))
	}
}

// authenticate 在同一连接上发送 gateway.authenticate，建立连接级认证态。
func (d *Dispatcher) authenticate(ctx context.Context, conn net.Conn, token string) error {
	authRequestID := d.requestIDFn() + "-auth"
	authRequestIDRaw, err := marshalJSONRawMessage(authRequestID)
	if err != nil {
		return newDispatchError(ErrorCodeInternal, fmt.Sprintf("encode authenticate id: %v", err))
	}
	authParamsRaw, err := marshalJSONRawMessage(protocol.AuthenticateParams{Token: token})
	if err != nil {
		return newDispatchError(ErrorCodeInternal, fmt.Sprintf("encode authenticate params: %v", err))
	}

	authRequest := protocol.JSONRPCRequest{
		JSONRPC: protocol.JSONRPCVersion,
		ID:      authRequestIDRaw,
		Method:  protocol.MethodGatewayAuthenticate,
		Params:  authParamsRaw,
	}
	authResponse, err := d.callRPC(ctx, conn, authRequest)
	if err != nil {
		return err
	}
	if strings.TrimSpace(authResponse.JSONRPC) != protocol.JSONRPCVersion {
		return newDispatchError(ErrorCodeUnexpectedResponse, "unexpected auth response jsonrpc version")
	}
	if !rawJSONMessageEqual(authResponse.ID, authRequest.ID) {
		return newDispatchError(ErrorCodeUnexpectedResponse, "rpc correlation failed: auth id mismatch")
	}
	if authResponse.Error != nil {
		return toDispatchErrorFromJSONRPC(authResponse.Error)
	}
	if authResponse.Result == nil {
		return newDispatchError(ErrorCodeUnexpectedResponse, "gateway auth response missing result payload")
	}
	frame, err := decodeResponseFrameResult(authResponse.Result)
	if err != nil {
		return newDispatchError(ErrorCodeUnexpectedResponse, fmt.Sprintf("decode auth response frame: %v", err))
	}
	if frame.Type != gateway.FrameTypeAck || frame.Action != gateway.FrameActionAuthenticate || frame.RequestID != authRequestID {
		return newDispatchError(ErrorCodeUnexpectedResponse, "unexpected auth response frame")
	}
	return nil
}

// callRPC 在已建立连接上执行一次 JSON-RPC 调用，统一处理上下文取消与编解码错误映射。
func (d *Dispatcher) callRPC(ctx context.Context, conn net.Conn, request protocol.JSONRPCRequest) (protocol.JSONRPCResponse, error) {
	if err := ensureDispatchContextActive(ctx); err != nil {
		return protocol.JSONRPCResponse{}, toDispatchError(err)
	}
	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(request); err != nil {
		if ctx != nil && ctx.Err() != nil {
			return protocol.JSONRPCResponse{}, toDispatchError(ctx.Err())
		}
		return protocol.JSONRPCResponse{}, newDispatchError(ErrorCodeInternal, fmt.Sprintf("write request rpc: %v", err))
	}

	if err := ensureDispatchContextActive(ctx); err != nil {
		return protocol.JSONRPCResponse{}, toDispatchError(err)
	}
	var response protocol.JSONRPCResponse
	decoder := json.NewDecoder(conn)
	if err := decoder.Decode(&response); err != nil {
		if ctx != nil && ctx.Err() != nil {
			return protocol.JSONRPCResponse{}, toDispatchError(ctx.Err())
		}
		return protocol.JSONRPCResponse{}, newDispatchError(ErrorCodeUnexpectedResponse, fmt.Sprintf("decode response rpc: %v", err))
	}
	return response, nil
}

// Dispatch 使用默认调度器执行 URL 转发。
func Dispatch(ctx context.Context, request DispatchRequest) (DispatchResult, error) {
	return NewDispatcher().Dispatch(ctx, request)
}

// applyDispatchDeadline 为调度连接设置统一超时控制。
func applyDispatchDeadline(conn net.Conn, ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if deadline, ok := ctx.Deadline(); ok {
		return conn.SetDeadline(deadline)
	}
	return conn.SetDeadline(time.Now().Add(defaultDispatchIOTimeout))
}

// ensureDispatchContextActive 在网络读写前检查上下文是否已取消，避免进入无意义阻塞 I/O。
func ensureDispatchContextActive(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

// watchDispatchCancellation 监听上下文取消信号，并通过收紧连接 deadline 立刻中断阻塞 I/O。
func watchDispatchCancellation(ctx context.Context, conn net.Conn) func() {
	if ctx == nil {
		return func() {}
	}

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.SetDeadline(time.Now())
		case <-done:
		}
	}()
	return func() {
		close(done)
	}
}

// toDispatchErrorFromJSONRPC 将 JSON-RPC 错误对象映射为 url-dispatch 稳定错误。
func toDispatchErrorFromJSONRPC(rpcError *protocol.JSONRPCError) error {
	if rpcError == nil {
		return newDispatchError(ErrorCodeUnexpectedResponse, "gateway returned empty rpc error payload")
	}

	code := strings.TrimSpace(protocol.GatewayCodeFromJSONRPCError(rpcError))
	if code == "" {
		code = mapJSONRPCCodeToDispatchCode(rpcError.Code)
	}
	message := strings.TrimSpace(rpcError.Message)
	if message == "" {
		message = "gateway returned empty rpc error message"
	}
	return newDispatchError(code, message)
}

// mapJSONRPCCodeToDispatchCode 为缺少 gateway_code 的响应提供兜底错误码映射。
func mapJSONRPCCodeToDispatchCode(code int) string {
	switch code {
	case protocol.JSONRPCCodeMethodNotFound:
		return gateway.ErrorCodeUnsupportedAction.String()
	case protocol.JSONRPCCodeInvalidRequest, protocol.JSONRPCCodeInvalidParams, protocol.JSONRPCCodeParseError:
		return gateway.ErrorCodeInvalidFrame.String()
	case protocol.JSONRPCCodeInternalError:
		return gateway.ErrorCodeInternalError.String()
	default:
		return ErrorCodeInternal
	}
}

// decodeResponseFrameResult 将 JSON-RPC result 安全解码回 MessageFrame。
func decodeResponseFrameResult(result json.RawMessage) (gateway.MessageFrame, error) {
	var frame gateway.MessageFrame
	if err := json.Unmarshal(result, &frame); err != nil {
		return gateway.MessageFrame{}, err
	}
	return frame, nil
}

// rawJSONMessageEqual 比较两段 JSON 原文在去除首尾空白后的字节是否一致。
func rawJSONMessageEqual(left, right json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(left), bytes.TrimSpace(right))
}

// marshalJSONRawMessage 将任意对象编码为 json.RawMessage，便于构造 JSON-RPC 请求字段。
func marshalJSONRawMessage(payload any) (json.RawMessage, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

// toDispatchError 将不同来源错误转换为统一结构化错误。
func toDispatchError(err error) error {
	if err == nil {
		return nil
	}

	var parseErr *protocol.ParseError
	if errors.As(err, &parseErr) {
		return newDispatchError(parseErr.Code, parseErr.Message)
	}

	var dispatchErr *DispatchError
	if errors.As(err, &dispatchErr) {
		return dispatchErr
	}

	return newDispatchError(ErrorCodeInternal, err.Error())
}

// nextDispatchRequestID 生成 url-dispatch 请求唯一标识。
func nextDispatchRequestID() string {
	sequence := atomic.AddUint64(&dispatchRequestCounter, 1)
	return fmt.Sprintf("wake-%d", sequence)
}

// newDispatchError 创建结构化调度错误。
func newDispatchError(code, message string) *DispatchError {
	return &DispatchError{
		Code:    code,
		Message: message,
	}
}
