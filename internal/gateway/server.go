package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultServerAddress         = "127.0.0.1:0"
	defaultServerReadTimeout     = 15 * time.Second
	defaultServerWriteTimeout    = 15 * time.Second
	defaultServerIdleTimeout     = 60 * time.Second
	defaultServerShutdownTimeout = 5 * time.Second
	defaultServerMaxFrameBytes   = int64(1 << 20)
)

// ServerConfig 定义网关服务的运行配置。
type ServerConfig struct {
	// Address 是网关监听地址，默认 127.0.0.1:0。
	Address string
	// ReadTimeout 是 HTTP 请求读取超时。
	ReadTimeout time.Duration
	// WriteTimeout 是 HTTP 响应写入超时。
	WriteTimeout time.Duration
	// IdleTimeout 是 HTTP Keep-Alive 空闲超时。
	IdleTimeout time.Duration
	// ShutdownTimeout 是优雅关闭最大等待时间。
	ShutdownTimeout time.Duration
	// MaxFrameBytes 是单帧最大字节数限制。
	MaxFrameBytes int64
}

// Server 是 Gateway 契约的标准 HTTP/WS 实现。
type Server struct {
	config ServerConfig

	upgrader websocket.Upgrader
	hub      *connectionHub

	mu          sync.RWMutex
	runtimePort RuntimePort
	httpServer  *http.Server
	listenAddr  string
	serveCtx    context.Context
	serveCancel context.CancelFunc
	eventCancel context.CancelFunc

	closeOnce sync.Once
	closeErr  error
}

// wsClient 表示单个 websocket 客户端连接。
type wsClient struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

// connectionHub 负责管理并发 websocket 连接与广播。
type connectionHub struct {
	mu      sync.RWMutex
	clients map[*wsClient]struct{}
}

// NewServer 创建一个网关服务实例。
func NewServer(cfg ServerConfig) Gateway {
	normalized := normalizeServerConfig(cfg)
	return &Server{
		config: normalized,
		hub:    newConnectionHub(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool {
				return true
			},
		},
	}
}

// normalizeServerConfig 归一化并补齐网关服务配置默认值。
func normalizeServerConfig(cfg ServerConfig) ServerConfig {
	cfg.Address = strings.TrimSpace(cfg.Address)
	if cfg.Address == "" {
		cfg.Address = defaultServerAddress
	}
	if cfg.ReadTimeout <= 0 {
		cfg.ReadTimeout = defaultServerReadTimeout
	}
	if cfg.WriteTimeout <= 0 {
		cfg.WriteTimeout = defaultServerWriteTimeout
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = defaultServerIdleTimeout
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = defaultServerShutdownTimeout
	}
	if cfg.MaxFrameBytes <= 0 {
		cfg.MaxFrameBytes = defaultServerMaxFrameBytes
	}
	return cfg
}

// Address 返回当前网关监听地址。
func (s *Server) Address() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listenAddr
}

// Serve 启动网关 HTTP/WS 服务并绑定运行端口。
func (s *Server) Serve(ctx context.Context, runtimePort RuntimePort) error {
	serveCtx, serveCancel := context.WithCancel(ctx)
	eventCtx, eventCancel := context.WithCancel(serveCtx)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/v1/gateway/frame", s.handleFrame)
	mux.HandleFunc("/v1/gateway/ws", s.handleWebSocket)

	httpServer := &http.Server{
		Addr:         s.config.Address,
		Handler:      mux,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
	}

	listener, err := net.Listen("tcp", s.config.Address)
	if err != nil {
		serveCancel()
		eventCancel()
		return err
	}

	s.mu.Lock()
	if s.httpServer != nil {
		s.mu.Unlock()
		_ = listener.Close()
		serveCancel()
		eventCancel()
		return errors.New("gateway: server already started")
	}
	s.runtimePort = runtimePort
	s.httpServer = httpServer
	s.listenAddr = listener.Addr().String()
	s.serveCtx = serveCtx
	s.serveCancel = serveCancel
	s.eventCancel = eventCancel
	s.mu.Unlock()

	if runtimePort != nil {
		go s.forwardRuntimeEvents(eventCtx, runtimePort)
	}

	go func() {
		<-serveCtx.Done()
		_ = s.Close(context.Background())
	}()

	err = httpServer.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Close 优雅关闭网关服务并回收连接资源。
func (s *Server) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	s.closeOnce.Do(func() {
		s.mu.RLock()
		httpServer := s.httpServer
		serveCancel := s.serveCancel
		eventCancel := s.eventCancel
		s.mu.RUnlock()

		if serveCancel != nil {
			serveCancel()
		}
		if eventCancel != nil {
			eventCancel()
		}
		s.hub.closeAll()

		if httpServer == nil {
			return
		}

		shutdownCtx, cancel := context.WithTimeout(ctx, s.config.ShutdownTimeout)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.closeErr = err
		}
	})

	return s.closeErr
}

// handleHealthz 提供网关健康检查接口。
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleFrame 处理 HTTP 帧请求入口。
func (s *Server) handleFrame(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	frame, frameErr := decodeFrameFromReader(r.Body, s.config.MaxFrameBytes)
	if frameErr != nil {
		s.writeFrame(w, statusForFrameError(frameErr.Code), errorFrameFor(MessageFrame{}, frameErr))
		return
	}

	if frameErr := s.validateInboundRequestFrame(frame); frameErr != nil {
		s.writeFrame(w, statusForFrameError(frameErr.Code), errorFrameFor(frame, frameErr))
		return
	}

	payload, frameErr := s.executeFrame(r.Context(), frame)
	if frameErr != nil {
		s.writeFrame(w, statusForFrameError(frameErr.Code), errorFrameFor(frame, frameErr))
		return
	}

	s.writeFrame(w, http.StatusOK, ackFrameFor(frame, payload))
}

// handleWebSocket 处理 websocket 双向帧通道。
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(s.config.MaxFrameBytes)

	client := &wsClient{conn: conn}
	s.hub.add(client)
	defer func() {
		s.hub.remove(client)
		_ = conn.Close()
	}()

	for {
		var frame MessageFrame
		if err := conn.ReadJSON(&frame); err != nil {
			return
		}

		if frameErr := s.validateInboundRequestFrame(frame); frameErr != nil {
			_ = client.writeFrame(errorFrameFor(frame, frameErr))
			continue
		}

		if err := client.writeFrame(ackFrameFor(frame, nil)); err != nil {
			return
		}

		payload, frameErr := s.executeFrame(r.Context(), frame)
		if frameErr != nil {
			_ = client.writeFrame(errorFrameFor(frame, frameErr))
			continue
		}
		if payload == nil {
			continue
		}

		eventFrame := MessageFrame{
			Type:      FrameTypeEvent,
			Action:    frame.Action,
			RequestID: frame.RequestID,
			RunID:     frame.RunID,
			SessionID: frame.SessionID,
			Payload:   payload,
		}
		_ = client.writeFrame(eventFrame)
	}
}

// decodeFrameFromReader 从 reader 中读取并解析单个网关协议帧。
func decodeFrameFromReader(reader io.Reader, maxBytes int64) (MessageFrame, *FrameError) {
	limited := io.LimitReader(reader, maxBytes)
	decoder := json.NewDecoder(limited)

	var frame MessageFrame
	if err := decoder.Decode(&frame); err != nil {
		return MessageFrame{}, NewFrameError(ErrorCodeInvalidFrame, "invalid frame payload")
	}

	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return MessageFrame{}, NewFrameError(ErrorCodeInvalidFrame, "invalid frame payload")
	}

	return frame, nil
}

// validateInboundRequestFrame 校验网关入口仅接受 request 帧。
func (s *Server) validateInboundRequestFrame(frame MessageFrame) *FrameError {
	if frame.Type != FrameTypeRequest {
		return NewFrameError(ErrorCodeInvalidFrame, "gateway endpoint only accepts request frame")
	}
	return ValidateFrame(frame)
}

// executeFrame 执行动作并返回可选同步结果负载。
func (s *Server) executeFrame(ctx context.Context, frame MessageFrame) (any, *FrameError) {
	runtimePort := s.getRuntimePort()
	if runtimePort == nil {
		return nil, NewFrameError(ErrorCodeRuntimeUnavailable, "runtime port is unavailable")
	}

	switch frame.Action {
	case FrameActionRun:
		go s.runAsync(frame, runtimePort)
		return nil, nil
	case FrameActionCompact:
		result, err := runtimePort.Compact(ctx, toCompactInput(frame))
		if err != nil {
			return nil, mapRuntimeError(err)
		}
		return result, nil
	case FrameActionCancel:
		return map[string]bool{"canceled": runtimePort.CancelActiveRun()}, nil
	case FrameActionListSessions:
		summaries, err := runtimePort.ListSessions(ctx)
		if err != nil {
			return nil, mapRuntimeError(err)
		}
		return summaries, nil
	case FrameActionLoadSession:
		session, err := runtimePort.LoadSession(ctx, frame.SessionID)
		if err != nil {
			return nil, mapRuntimeError(err)
		}
		return session, nil
	case FrameActionSetSessionWorkdir:
		session, err := runtimePort.SetSessionWorkdir(ctx, frame.SessionID, frame.Workdir)
		if err != nil {
			return nil, mapRuntimeError(err)
		}
		return session, nil
	default:
		return nil, NewFrameError(ErrorCodeUnsupportedAction, "unsupported action")
	}
}

// runAsync 异步执行 run 动作，并在失败时向 websocket 客户端广播错误帧。
func (s *Server) runAsync(frame MessageFrame, runtimePort RuntimePort) {
	s.mu.RLock()
	ctx := s.serveCtx
	s.mu.RUnlock()
	if ctx == nil {
		ctx = context.Background()
	}

	if err := runtimePort.Run(ctx, toRunInput(frame)); err != nil {
		s.hub.broadcast(errorFrameFor(frame, mapRuntimeError(err)))
	}
}

// forwardRuntimeEvents 将 runtime 事件转发为网关事件帧并广播到 websocket 客户端。
func (s *Server) forwardRuntimeEvents(ctx context.Context, runtimePort RuntimePort) {
	events := runtimePort.Events()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			s.hub.broadcast(MessageFrame{
				Type:      FrameTypeEvent,
				RunID:     event.RunID,
				SessionID: event.SessionID,
				Payload:   event,
			})
		}
	}
}

// getRuntimePort 读取当前绑定的 runtime 端口。
func (s *Server) getRuntimePort() RuntimePort {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.runtimePort
}

// toRunInput 将网关请求帧转换为 RunInput。
func toRunInput(frame MessageFrame) RunInput {
	return RunInput{
		RequestID:  frame.RequestID,
		SessionID:  frame.SessionID,
		RunID:      frame.RunID,
		InputText:  frame.InputText,
		InputParts: append([]InputPart(nil), frame.InputParts...),
		Workdir:    frame.Workdir,
	}
}

// toCompactInput 将网关请求帧转换为 CompactInput。
func toCompactInput(frame MessageFrame) CompactInput {
	return CompactInput{
		RequestID: frame.RequestID,
		SessionID: frame.SessionID,
		RunID:     frame.RunID,
	}
}

// mapRuntimeError 将下游运行错误映射为稳定网关错误对象。
func mapRuntimeError(err error) *FrameError {
	if err == nil {
		return nil
	}
	return NewFrameError(ErrorCodeInternalError, err.Error())
}

// statusForFrameError 根据网关错误码映射 HTTP 状态码。
func statusForFrameError(code string) int {
	switch code {
	case ErrorCodeInvalidFrame.String(),
		ErrorCodeInvalidAction.String(),
		ErrorCodeInvalidMultimodalPayload.String(),
		ErrorCodeMissingRequiredField.String(),
		ErrorCodeUnsupportedAction.String():
		return http.StatusBadRequest
	case ErrorCodeRuntimeUnavailable.String():
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// ackFrameFor 构造请求对应的确认帧。
func ackFrameFor(frame MessageFrame, payload any) MessageFrame {
	return MessageFrame{
		Type:      FrameTypeAck,
		Action:    frame.Action,
		RequestID: frame.RequestID,
		RunID:     frame.RunID,
		SessionID: frame.SessionID,
		Payload:   payload,
	}
}

// errorFrameFor 构造请求对应的错误帧。
func errorFrameFor(frame MessageFrame, frameErr *FrameError) MessageFrame {
	return MessageFrame{
		Type:      FrameTypeError,
		Action:    frame.Action,
		RequestID: frame.RequestID,
		RunID:     frame.RunID,
		SessionID: frame.SessionID,
		Error:     frameErr,
	}
}

// writeFrame 输出标准网关协议帧到 HTTP 响应。
func (s *Server) writeFrame(w http.ResponseWriter, statusCode int, frame MessageFrame) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(frame)
}

// writeFrame 向 websocket 客户端写入单帧，内部保证并发写串行化。
func (c *wsClient) writeFrame(frame MessageFrame) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.conn.WriteJSON(frame)
}

// newConnectionHub 创建 websocket 连接管理器。
func newConnectionHub() *connectionHub {
	return &connectionHub{clients: make(map[*wsClient]struct{})}
}

// add 注册 websocket 客户端连接。
func (h *connectionHub) add(client *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[client] = struct{}{}
}

// remove 注销 websocket 客户端连接。
func (h *connectionHub) remove(client *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, client)
}

// broadcast 向所有 websocket 客户端广播同一协议帧。
func (h *connectionHub) broadcast(frame MessageFrame) {
	h.mu.RLock()
	clients := make([]*wsClient, 0, len(h.clients))
	for client := range h.clients {
		clients = append(clients, client)
	}
	h.mu.RUnlock()

	for _, client := range clients {
		if err := client.writeFrame(frame); err != nil {
			h.remove(client)
			_ = client.conn.Close()
		}
	}
}

// closeAll 关闭并清空所有 websocket 客户端连接。
func (h *connectionHub) closeAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for client := range h.clients {
		_ = client.conn.Close()
		delete(h.clients, client)
	}
}
