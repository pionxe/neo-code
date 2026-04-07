package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultStdioStartTimeout   = 5 * time.Second
	defaultStdioCallTimeout    = 15 * time.Second
	defaultStdioRestartBackoff = 1 * time.Second
	maxStdioRestartBackoff     = 30 * time.Second
	maxStdioFrameBytes         = 8 * 1024 * 1024
	defaultMCPProtocolVersion  = "2024-11-05"
	defaultMCPClientName       = "neocode"
	defaultMCPClientVersion    = "0.1.0"
)

// StdioClientConfig 描述 MCP stdio 客户端的启动与调用参数。
type StdioClientConfig struct {
	Command        string
	Args           []string
	Env            []string
	Workdir        string
	StartTimeout   time.Duration
	CallTimeout    time.Duration
	RestartBackoff time.Duration
}

type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonRPCNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcReply struct {
	result json.RawMessage
	err    error
}

// StdIOClient 通过 stdio 子进程与 MCP server 进行 JSON-RPC 通信。
type StdIOClient struct {
	cfg          StdioClientConfig
	idSeed       uint64
	mu           sync.Mutex
	writeMu      sync.Mutex
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	stdout       io.ReadCloser
	reader       *bufio.Reader
	pending      map[string]chan rpcReply
	exited       chan struct{}
	exitErr      error
	backoff      time.Duration
	retryAt      time.Time
	started      bool
	initialized  bool
	initializing bool
	initDone     chan struct{}
	shutdown     bool
}

// NewStdIOClient 创建 stdio MCP client。
func NewStdIOClient(cfg StdioClientConfig) (*StdIOClient, error) {
	if strings.TrimSpace(cfg.Command) == "" {
		return nil, errors.New("mcp: stdio command is empty")
	}
	if cfg.StartTimeout <= 0 {
		cfg.StartTimeout = defaultStdioStartTimeout
	}
	if cfg.CallTimeout <= 0 {
		cfg.CallTimeout = defaultStdioCallTimeout
	}
	if cfg.RestartBackoff <= 0 {
		cfg.RestartBackoff = defaultStdioRestartBackoff
	}

	return &StdIOClient{
		cfg:     cfg,
		pending: make(map[string]chan rpcReply),
		backoff: cfg.RestartBackoff,
	}, nil
}

// Close 关闭 stdio 子进程并释放资源。
func (c *StdIOClient) Close() error {
	if c == nil {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.shutdown = true
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	c.failAllPendingLocked(errors.New("mcp: stdio client closed"))
	return nil
}

// ListTools 调用 MCP `tools/list` 获取工具清单。
func (c *StdIOClient) ListTools(ctx context.Context) ([]ToolDescriptor, error) {
	callCtx, cancel := c.callContext(ctx)
	defer cancel()

	raw, err := c.call(callCtx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}

	var payload struct {
		Tools []struct {
			Name         string         `json:"name"`
			Description  string         `json:"description"`
			InputSchema  map[string]any `json:"inputSchema"`
			InputSchema2 map[string]any `json:"input_schema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("mcp: decode tools/list result: %w", err)
	}

	result := make([]ToolDescriptor, 0, len(payload.Tools))
	for _, item := range payload.Tools {
		schema := item.InputSchema
		if len(schema) == 0 {
			schema = item.InputSchema2
		}
		result = append(result, ToolDescriptor{
			Name:        strings.TrimSpace(item.Name),
			Description: strings.TrimSpace(item.Description),
			InputSchema: ensureObjectSchema(schema),
		})
	}
	return result, nil
}

// CallTool 调用 MCP `tools/call` 并收敛返回值。
func (c *StdIOClient) CallTool(ctx context.Context, toolName string, arguments []byte) (CallResult, error) {
	trimmedToolName := strings.TrimSpace(toolName)
	if trimmedToolName == "" {
		return CallResult{}, errors.New("mcp: tool name is empty")
	}

	callCtx, cancel := c.callContext(ctx)
	defer cancel()

	var args any = map[string]any{}
	if len(arguments) > 0 {
		if err := json.Unmarshal(arguments, &args); err != nil {
			return CallResult{}, fmt.Errorf("mcp: decode tool arguments: %w", err)
		}
	}

	raw, err := c.call(callCtx, "tools/call", map[string]any{
		"name":      trimmedToolName,
		"arguments": args,
	})
	if err != nil {
		return CallResult{}, err
	}
	return decodeCallResult(raw), nil
}

// HealthCheck 通过一次短超时 `tools/list` 验证连接可用性。
func (c *StdIOClient) HealthCheck(ctx context.Context) error {
	_, err := c.ListTools(ctx)
	return err
}

func (c *StdIOClient) callContext(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := c.cfg.CallTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}
	return context.WithTimeout(ctx, timeout)
}

func (c *StdIOClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	return c.callRequest(ctx, method, params, false)
}

// callRequest 发送带响应的 RPC 请求；skipEnsure=true 用于初始化阶段避免递归。
func (c *StdIOClient) callRequest(ctx context.Context, method string, params any, skipEnsure bool) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !skipEnsure {
		if err := c.ensureStarted(ctx); err != nil {
			return nil, err
		}
		if err := c.ensureInitialized(ctx); err != nil {
			return nil, err
		}
	}

	requestID := "req-" + strconv.FormatUint(atomic.AddUint64(&c.idSeed, 1), 10)
	replyCh := make(chan rpcReply, 1)

	c.mu.Lock()
	if c.shutdown {
		c.mu.Unlock()
		return nil, errors.New("mcp: stdio client closed")
	}
	c.pending[requestID] = replyCh
	stdin := c.stdin
	c.mu.Unlock()
	if stdin == nil {
		c.removePending(requestID)
		return nil, errors.New("mcp: stdio client is not connected")
	}

	requestPayload, err := json.Marshal(jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  method,
		Params:  params,
	})
	if err != nil {
		c.removePending(requestID)
		return nil, fmt.Errorf("mcp: marshal request: %w", err)
	}
	c.writeMu.Lock()
	writeErr := writeFramedMessage(stdin, requestPayload)
	c.writeMu.Unlock()
	if writeErr != nil {
		c.removePending(requestID)
		return nil, fmt.Errorf("mcp: send request: %w", writeErr)
	}

	select {
	case <-ctx.Done():
		c.removePending(requestID)
		return nil, ctx.Err()
	case reply := <-replyCh:
		return reply.result, reply.err
	}
}

// sendNotification 发送无需响应的 RPC 通知；skipEnsure=true 用于初始化流程。
func (c *StdIOClient) sendNotification(ctx context.Context, method string, params any, skipEnsure bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !skipEnsure {
		if err := c.ensureStarted(ctx); err != nil {
			return err
		}
		if err := c.ensureInitialized(ctx); err != nil {
			return err
		}
	}

	c.mu.Lock()
	if c.shutdown {
		c.mu.Unlock()
		return errors.New("mcp: stdio client closed")
	}
	stdin := c.stdin
	c.mu.Unlock()
	if stdin == nil {
		return errors.New("mcp: stdio client is not connected")
	}

	payload, err := json.Marshal(jsonRPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return fmt.Errorf("mcp: marshal notification: %w", err)
	}

	c.writeMu.Lock()
	writeErr := writeFramedMessage(stdin, payload)
	c.writeMu.Unlock()
	if writeErr != nil {
		return fmt.Errorf("mcp: send notification: %w", writeErr)
	}
	return nil
}

func (c *StdIOClient) ensureStarted(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.shutdown {
		return errors.New("mcp: stdio client closed")
	}
	if c.started {
		return nil
	}
	if !c.retryAt.IsZero() && time.Now().Before(c.retryAt) {
		return fmt.Errorf("mcp: stdio restart backoff in effect until %s", c.retryAt.Format(time.RFC3339))
	}

	startCtx, cancel := context.WithTimeout(ctx, c.cfg.StartTimeout)
	defer cancel()

	command := exec.Command(c.cfg.Command, c.cfg.Args...)
	command.Env = append(os.Environ(), c.cfg.Env...)
	command.Dir = strings.TrimSpace(c.cfg.Workdir)

	stdin, err := command.StdinPipe()
	if err != nil {
		return fmt.Errorf("mcp: create stdin pipe: %w", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return fmt.Errorf("mcp: create stdout pipe: %w", err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return fmt.Errorf("mcp: create stderr pipe: %w", err)
	}

	startErrCh := make(chan error, 1)
	go func() {
		startErrCh <- command.Start()
	}()
	select {
	case <-startCtx.Done():
		return startCtx.Err()
	case err := <-startErrCh:
		if err != nil {
			c.bumpBackoffLocked()
			return fmt.Errorf("mcp: start stdio server: %w", err)
		}
	}

	c.cmd = command
	c.stdin = stdin
	c.stdout = stdout
	c.reader = bufio.NewReader(stdout)
	c.exited = make(chan struct{})
	c.exitErr = nil
	c.started = true
	c.initialized = false
	c.initializing = false
	c.initDone = nil
	c.backoff = c.cfg.RestartBackoff
	c.retryAt = time.Time{}

	go c.readLoop()
	go c.waitLoop(command)
	go io.Copy(io.Discard, stderr)
	return nil
}

// ensureInitialized 确保 MCP 会话完成 initialize/initialized 握手，并发调用共享结果。
func (c *StdIOClient) ensureInitialized(ctx context.Context) error {
	for {
		c.mu.Lock()
		if c.shutdown {
			c.mu.Unlock()
			return errors.New("mcp: stdio client closed")
		}
		if !c.started {
			c.mu.Unlock()
			return errors.New("mcp: stdio client is not started")
		}
		if c.initialized {
			c.mu.Unlock()
			return nil
		}
		if c.initializing {
			wait := c.initDone
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-wait:
				continue
			}
		}
		c.initializing = true
		c.initDone = make(chan struct{})
		done := c.initDone
		c.mu.Unlock()

		initErr := c.performInitialize(ctx)

		c.mu.Lock()
		if c.started && initErr == nil {
			c.initialized = true
		}
		c.initializing = false
		close(done)
		c.mu.Unlock()
		return initErr
	}
}

// performInitialize 执行标准 MCP 初始化握手：initialize -> notifications/initialized。
func (c *StdIOClient) performInitialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": defaultMCPProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    defaultMCPClientName,
			"version": defaultMCPClientVersion,
		},
	}
	if _, err := c.callRequest(ctx, "initialize", params, true); err != nil {
		return fmt.Errorf("mcp: initialize session: %w", err)
	}
	if err := c.sendNotification(ctx, "notifications/initialized", map[string]any{}, true); err != nil {
		return fmt.Errorf("mcp: notify initialized: %w", err)
	}
	return nil
}

func (c *StdIOClient) readLoop() {
	for {
		message, err := readFramedMessage(c.reader)
		if err != nil {
			c.markExited(fmt.Errorf("mcp: read response: %w", err))
			return
		}

		var response jsonRPCResponse
		if err := json.Unmarshal(message, &response); err != nil {
			continue
		}
		if strings.TrimSpace(response.ID) == "" {
			continue
		}

		c.mu.Lock()
		replyCh, ok := c.pending[response.ID]
		if ok {
			delete(c.pending, response.ID)
		}
		c.mu.Unlock()
		if !ok {
			continue
		}

		if response.Error != nil {
			replyCh <- rpcReply{
				err: fmt.Errorf("mcp: rpc error %d: %s", response.Error.Code, strings.TrimSpace(response.Error.Message)),
			}
			continue
		}
		replyCh <- rpcReply{result: response.Result}
	}
}

func (c *StdIOClient) waitLoop(command *exec.Cmd) {
	if command == nil {
		c.markExited(errors.New("mcp: stdio process is nil"))
		return
	}
	err := command.Wait()
	c.markExited(fmt.Errorf("mcp: stdio process exited: %w", err))
}

func (c *StdIOClient) markExited(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return
	}
	c.started = false
	c.initialized = false
	if c.initializing && c.initDone != nil {
		close(c.initDone)
	}
	c.initializing = false
	c.initDone = nil
	c.exitErr = err
	if c.exited != nil {
		close(c.exited)
	}
	c.stdin = nil
	c.stdout = nil
	c.reader = nil
	c.cmd = nil
	c.failAllPendingLocked(err)
	c.bumpBackoffLocked()
}

func (c *StdIOClient) removePending(requestID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pending, requestID)
}

func (c *StdIOClient) failAllPendingLocked(err error) {
	for requestID, replyCh := range c.pending {
		replyCh <- rpcReply{err: err}
		delete(c.pending, requestID)
	}
}

func (c *StdIOClient) bumpBackoffLocked() {
	if c.backoff <= 0 {
		c.backoff = c.cfg.RestartBackoff
	}
	c.retryAt = time.Now().Add(c.backoff)
	c.backoff *= 2
	if c.backoff > maxStdioRestartBackoff {
		c.backoff = maxStdioRestartBackoff
	}
}

func writeFramedMessage(writer io.Writer, payload []byte) error {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(payload))
	if _, err := io.WriteString(writer, header); err != nil {
		return err
	}
	if _, err := writer.Write(payload); err != nil {
		return err
	}
	return nil
}

func readFramedMessage(reader *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			break
		}

		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "content-length:") {
			rawLength := strings.TrimSpace(trimmed[len("content-length:"):])
			length, convErr := strconv.Atoi(rawLength)
			if convErr != nil {
				return nil, fmt.Errorf("mcp: invalid content-length %q", rawLength)
			}
			contentLength = length
		}
	}
	if contentLength < 0 {
		return nil, errors.New("mcp: missing content-length header")
	}
	if contentLength > maxStdioFrameBytes {
		return nil, fmt.Errorf("mcp: content-length %d exceeds limit %d", contentLength, maxStdioFrameBytes)
	}

	payload := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func decodeCallResult(raw json.RawMessage) CallResult {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return CallResult{
			Content:  strings.TrimSpace(string(raw)),
			IsError:  false,
			Metadata: map[string]any{"raw_result": string(raw)},
		}
	}

	content := ""
	switch typed := payload["content"].(type) {
	case string:
		content = strings.TrimSpace(typed)
	case []any:
		lines := make([]string, 0, len(typed))
		for _, item := range typed {
			switch value := item.(type) {
			case map[string]any:
				text, _ := value["text"].(string)
				if strings.TrimSpace(text) != "" {
					lines = append(lines, strings.TrimSpace(text))
				}
			case string:
				if strings.TrimSpace(value) != "" {
					lines = append(lines, strings.TrimSpace(value))
				}
			}
		}
		content = strings.Join(lines, "\n")
	default:
		if typed != nil {
			content = strings.TrimSpace(fmt.Sprintf("%v", typed))
		}
	}
	if content == "" {
		content = "ok"
	}

	isError := false
	if value, ok := payload["isError"].(bool); ok {
		isError = value
	}
	if value, ok := payload["is_error"].(bool); ok {
		isError = isError || value
	}

	metadata := map[string]any{}
	for key, value := range payload {
		if key == "content" || key == "isError" || key == "is_error" {
			continue
		}
		metadata[key] = value
	}
	metadata["raw_result"] = string(bytes.TrimSpace(raw))

	return CallResult{
		Content:  content,
		IsError:  isError,
		Metadata: metadata,
	}
}
