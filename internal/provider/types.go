package provider

import (
	"fmt"
	"sort"
	"strings"
)

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	IsError    bool       `json:"is_error,omitempty"`
}

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Schema      map[string]any `json:"schema"`
}

type ChatRequest struct {
	Model        string     `json:"model"`
	SystemPrompt string     `json:"system_prompt"`
	Messages     []Message  `json:"messages"`
	Tools        []ToolSpec `json:"tools,omitempty"`
}

type ChatResponse struct {
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
	Usage        Usage   `json:"usage"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type SupportLevel string

const (
	SupportLevelMVP        SupportLevel = "mvp"
	SupportLevelScaffolded SupportLevel = "scaffolded"
)

type ModelOption struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type ProviderDescriptor struct {
	Name         string        `json:"name"`
	DisplayName  string        `json:"display_name"`
	SupportLevel SupportLevel  `json:"support_level"`
	MVPVisible   bool          `json:"mvp_visible"`
	Available    bool          `json:"available"`
	Summary      string        `json:"summary"`
	Models       []ModelOption `json:"models,omitempty"`
}

type Describer interface {
	Descriptor() ProviderDescriptor
}

func Describe(p Provider) ProviderDescriptor {
	if p == nil {
		return ProviderDescriptor{}
	}

	if describer, ok := p.(Describer); ok {
		return normalizeDescriptor(describer.Descriptor(), p.Name())
	}

	return ProviderDescriptor{
		Name:         strings.TrimSpace(p.Name()),
		DisplayName:  strings.TrimSpace(p.Name()),
		SupportLevel: SupportLevelScaffolded,
		Summary:      "Provider metadata is not available.",
	}
}

func ScaffoldedProviderError(name string) error {
	providerName := strings.TrimSpace(name)
	if providerName == "" {
		providerName = "unknown"
	}
	return fmt.Errorf(
		"%s provider is scaffolded only and not available in this MVP; only OpenAI-compatible provider is officially supported",
		providerName,
	)
}

func normalizeDescriptor(desc ProviderDescriptor, fallbackName string) ProviderDescriptor {
	name := strings.TrimSpace(desc.Name)
	if name == "" {
		name = strings.TrimSpace(fallbackName)
	}
	desc.Name = name

	if strings.TrimSpace(desc.DisplayName) == "" {
		desc.DisplayName = name
	}
	if strings.TrimSpace(desc.Summary) == "" {
		desc.Summary = "Provider metadata is not available."
	}

	desc.Models = normalizedModels(desc.Models)
	return desc
}

func normalizedModels(models []ModelOption) []ModelOption {
	if len(models) == 0 {
		return nil
	}

	deduped := make([]ModelOption, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		name := strings.TrimSpace(model.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, ModelOption{
			Name:        name,
			Description: strings.TrimSpace(model.Description),
		})
	}

	sort.Slice(deduped, func(i, j int) bool {
		return strings.ToLower(deduped[i].Name) < strings.ToLower(deduped[j].Name)
	})

	return deduped
}
