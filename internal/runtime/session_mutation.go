package runtime

import (
	"context"
	"strings"

	providertypes "neo-code/internal/provider/types"
	"neo-code/internal/tools"
)

// appendUserMessageAndSave 将用户消息追加到会话并立即持久化。
func (s *Service) appendUserMessageAndSave(ctx context.Context, state *runState, content string) error {
	message := providertypes.Message{
		Role:    providertypes.RoleUser,
		Content: content,
	}
	state.session.Messages = append(state.session.Messages, message)
	state.touchSession()
	if err := s.sessionStore.Save(ctx, &state.session); err != nil {
		return err
	}
	s.emitRunScoped(ctx, EventUserMessage, state, message)
	return nil
}

// appendAssistantMessageAndSave 将 assistant 消息和本轮模型元数据写回会话。
func (s *Service) appendAssistantMessageAndSave(
	ctx context.Context,
	state *runState,
	snapshot turnSnapshot,
	assistant providertypes.Message,
) error {
	metadataChanged := state.session.Provider != snapshot.providerConfig.Name || state.session.Model != snapshot.model
	state.session.Provider = snapshot.providerConfig.Name
	state.session.Model = snapshot.model

	if strings.TrimSpace(assistant.Content) != "" || len(assistant.ToolCalls) > 0 {
		state.session.Messages = append(state.session.Messages, assistant)
		state.touchSession()
		return s.sessionStore.Save(ctx, &state.session)
	}
	if metadataChanged {
		state.touchSession()
		return s.sessionStore.Save(ctx, &state.session)
	}
	return nil
}

// appendToolMessageAndSave 将工具原始结果写回会话，避免污染持久化对话内容。
func (s *Service) appendToolMessageAndSave(
	ctx context.Context,
	state *runState,
	call providertypes.ToolCall,
	result tools.ToolResult,
) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	toolMessage := providertypes.Message{
		Role:         providertypes.RoleTool,
		Content:      result.Content,
		ToolCallID:   call.ID,
		IsError:      result.IsError,
		ToolMetadata: tools.SanitizeToolMetadata(result.Name, result.Metadata),
	}
	state.session.Messages = append(state.session.Messages, toolMessage)
	state.touchSession()
	return s.sessionStore.Save(ctx, &state.session)
}
