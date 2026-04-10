package openai

import (
	"context"

	providertypes "neo-code/internal/provider/types"
)

// emitTextDelta 发送文本增量事件，空文本时跳过。
func emitTextDelta(ctx context.Context, events chan<- providertypes.StreamEvent, text string) error {
	if text == "" {
		return nil
	}
	return emitStreamEvent(ctx, events, providertypes.NewTextDeltaStreamEvent(text))
}

// emitToolCallStart 发送工具调用开始事件，空名称时跳过。
func emitToolCallStart(ctx context.Context, events chan<- providertypes.StreamEvent, index int, id, name string) error {
	if name == "" {
		return nil
	}
	return emitStreamEvent(ctx, events, providertypes.NewToolCallStartStreamEvent(index, id, name))
}

// emitToolCallDelta 发送工具调用参数增量事件。
// id 为工具调用 ID，由上游 mergeToolCallDelta 从累积状态中传入。
func emitToolCallDelta(ctx context.Context, events chan<- providertypes.StreamEvent, index int, id, argumentsDelta string) error {
	if argumentsDelta == "" {
		return nil
	}
	return emitStreamEvent(ctx, events, providertypes.NewToolCallDeltaStreamEvent(index, id, argumentsDelta))
}

// emitMessageDone 发送消息完成事件。
func emitMessageDone(ctx context.Context, events chan<- providertypes.StreamEvent, finishReason string, usage *providertypes.Usage) error {
	event := providertypes.NewMessageDoneStreamEvent(finishReason, usage)
	if ctx == nil || ctx.Err() == nil {
		return emitStreamEvent(ctx, events, event)
	}
	if events == nil {
		return nil
	}

	select {
	case events <- event:
		return nil
	default:
		return nil
	}
}

// emitStreamEvent 通过 channel 安全发送流式事件，支持上下文取消和 nil channel 保护。
func emitStreamEvent(ctx context.Context, events chan<- providertypes.StreamEvent, event providertypes.StreamEvent) error {
	if events == nil {
		return nil
	}

	select {
	case events <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// flushDataLines 逐行处理缓冲的 data lines，每行作为独立 payload 通过 processChunk 处理。
// SSE 规范允许同一事件内多行 data 拼接，但 OpenAI 实际行为是每行 data 为独立 JSON，
// 因此逐行处理更可靠，避免拼接产生无效 JSON。
func flushDataLines(dataLines []string, processChunk func(string) error) error {
	for _, line := range dataLines {
		if err := processChunk(line); err != nil {
			return err
		}
	}
	return nil
}
