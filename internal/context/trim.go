package context

import "neo-code/internal/provider"

const maxRetainedMessageSpans = 10

func trimMessages(messages []provider.Message) []provider.Message {
	if len(messages) <= maxRetainedMessageSpans {
		return append([]provider.Message(nil), messages...)
	}

	type span struct {
		start int
		end   int
	}

	spans := make([]span, 0, len(messages))
	for i := 0; i < len(messages); {
		start := i
		i++

		if messages[start].Role == provider.RoleAssistant && len(messages[start].ToolCalls) > 0 {
			for i < len(messages) && messages[i].Role == provider.RoleTool {
				i++
			}
		}

		spans = append(spans, span{start: start, end: i})
	}

	if len(spans) <= maxRetainedMessageSpans {
		return append([]provider.Message(nil), messages...)
	}

	start := spans[len(spans)-maxRetainedMessageSpans].start
	return append([]provider.Message(nil), messages[start:]...)
}
