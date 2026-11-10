package modelio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	agent "ta/agent"
	"ta/agent/providers"

	"ta/config"
)

// ConfigForAgent resolves the provider-neutral model configuration for one
// Agent kind.
func ConfigForAgent(cfg *config.Config, agentKind string) (providers.ModelConfig, error) {
	return ConfigFromResolved(config.ResolveAgentModel(cfg, agentKind))
}

func ConfigFromResolved(resolved config.ResolvedModelSettings) (providers.ModelConfig, error) {
	provider := providers.ProviderID(resolved.Provider)
	protocol := providers.ProtocolID(resolved.Protocol)
	if provider == "" {
		provider = providers.ProviderOpenAICompatible
	}
	modelConfig := providers.ModelConfig{
		Provider:          provider,
		Protocol:          protocol,
		APIKey:            resolved.APIKey,
		Model:             resolved.Model,
		BaseURL:           resolved.BaseURL,
		Headers:           resolved.Headers,
		SessionKeyMapping: resolved.SessionKeyMapping,
		MaxOutputTokens:   resolved.MaxTokens,
		ThinkingLevel:     providers.ThinkingLevel(resolved.ThinkingLevel),
	}
	if len(resolved.ProtocolOptions) != 0 {
		options, err := providers.EncodeProtocolOptions(resolved.ProtocolOptions)
		if err != nil {
			return providers.ModelConfig{}, fmt.Errorf("convert model protocol options: %w", err)
		}
		modelConfig.ProtocolOptions = options
	}
	if resolved.Temperature != nil {
		temperature := float32(*resolved.Temperature)
		modelConfig.Temperature = &temperature
	}
	return modelConfig, nil
}

// CanWrite reports whether cfg names a model a story could actually be written
// with, by running the same provider, protocol, base URL and model checks a
// real call goes through without making the call.
//
// A UI gate must ask this rather than infer from the persisted settings: the
// effective configuration also carries environment-variable overrides and
// provider presets that supply their own base URL, and a local provider such as
// Ollama legitimately needs no API key. Inferring from one layer of the settings
// files reports "no model" for configurations that work.
func CanWrite(cfg *config.Config) error {
	if cfg == nil {
		return errors.New("no configuration")
	}
	_, err := defaultRuntime.ResolveModel(config.ResolveAgentModel(cfg, config.AgentKindInteractiveStory))
	return err
}

func NewChatModel(ctx context.Context, config providers.ModelConfig) (agent.ToolCallingChatModel, error) {
	return defaultRuntime.NewChatModel(ctx, config)
}

func WithJSONObjectOutput(config providers.ModelConfig) providers.ModelConfig {
	config.OutputFormat = &providers.OutputFormat{Type: providers.OutputFormatJSONObject}
	return config
}

// WithPrefixCompletion turns on prefix completion: the trailing assistant
// message is treated as the beginning of this answer, and the reply continues it
// rather than starting over. The adapter writes it as an extra request field, so
// a provider that does not know the field ignores it instead of failing.
//
// The adapter returns the prefix as part of the content, so a caller that seeds
// a document receives that document back whole.
func WithPrefixCompletion(config providers.ModelConfig) providers.ModelConfig {
	options := map[string]any{}
	if len(config.ProtocolOptions) > 0 {
		if err := json.Unmarshal(config.ProtocolOptions, &options); err != nil {
			return config
		}
	}
	extra, _ := options["extra_body"].(map[string]any)
	if extra == nil {
		extra = map[string]any{}
	}
	extra["prefix"] = true
	options["extra_body"] = extra
	encoded, err := json.Marshal(options)
	if err != nil {
		return config
	}
	config.ProtocolOptions = encoded
	return config
}
