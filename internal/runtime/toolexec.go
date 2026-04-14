package runtime

import (
	"context"
	"errors"
	"strings"
	"sync"

	providertypes "neo-code/internal/provider/types"
)

// executeAssistantToolCalls 并发执行 assistant 返回的全部工具调用并回写结果。
func (s *Service) executeAssistantToolCalls(
	ctx context.Context,
	state *runState,
	snapshot turnSnapshot,
	assistant providertypes.Message,
) error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for _, call := range assistant.ToolCalls {
		if err := ctx.Err(); err != nil {
			return err
		}
		wg.Add(1)
		go func(call providertypes.ToolCall) {
			defer wg.Done()
			s.emitRunScoped(ctx, EventToolStart, state, call)

			result, execErr := s.executeToolCallWithPermission(ctx, permissionExecutionInput{
				RunID:       state.runID,
				SessionID:   state.session.ID,
				Call:        call,
				Workdir:     snapshot.workdir,
				ToolTimeout: snapshot.toolTimeout,
			})

			if errors.Is(execErr, context.Canceled) {
				mu.Lock()
				if firstErr == nil {
					firstErr = execErr
				}
				mu.Unlock()
				return
			}
			if execErr == nil {
				if err := ctx.Err(); err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					return
				}
			}

			if execErr != nil && strings.TrimSpace(result.Content) == "" {
				result.Content = execErr.Error()
			}

			if err := s.appendToolMessageAndSave(ctx, state, call, result); err != nil {
				if execErr != nil && errors.Is(err, context.Canceled) {
					s.emitRunScoped(ctx, EventToolResult, state, result)
				}
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}

			if err := ctx.Err(); err != nil && execErr == nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}

			s.emitRunScoped(ctx, EventToolResult, state, result)

			if isSuccessfulRememberToolCall(call.Name, result, execErr) {
				state.mu.Lock()
				state.rememberedThisRun = true
				state.mu.Unlock()
			}

			if execErr != nil {
				if err := ctx.Err(); err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					return
				}
			}
		}(call)
	}

	wg.Wait()
	return firstErr
}
