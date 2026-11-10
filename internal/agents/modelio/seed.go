package modelio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	agent "ta/agent"
	"ta/agent/providers"
)

// SeededCall sends one chat completion whose final message is the assistant's
// half-written answer, with DeepSeek's prefix completion flag switched on, and
// returns the completed text.
//
// It bypasses the provider runtime on purpose. The runtime treats a trailing
// assistant message as replayed history and normalises the request body on the
// way out, which loses both the continuation semantics and the prefix flag: the
// model then answers the last user turn from scratch. A seeded call has to be
// the bare request DeepSeek documents.
func SeededCall(ctx context.Context, config providers.ModelConfig, messages []*agent.Message, seed string) (string, error) {
	if strings.TrimSpace(config.BaseURL) == "" {
		return "", errors.New("seeded call needs a base URL")
	}
	// An empty seed is a plain call: some models reject a trailing model turn
	// outright ("Requests ending with a model turn are not supported"), so the
	// continuation shape is only sent when there is something to continue.
	continuing := strings.TrimSpace(seed) != ""
	body := map[string]any{
		"model":       config.Model,
		"messages":    seededMessages(messages, seed),
		"stream":      false,
		"max_tokens":  seededMaxTokens(config.MaxOutputTokens),
		"temperature": seededTemperature(config.Temperature),
	}
	if continuing {
		body["prefix"] = true
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("encode seeded request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, seededEndpoint(config.BaseURL), bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build seeded request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if config.APIKey != "" {
		request.Header.Set("Authorization", "Bearer "+config.APIKey)
	}
	for key, value := range config.Headers {
		if strings.EqualFold(key, "authorization") || strings.EqualFold(key, "content-type") {
			continue
		}
		request.Header.Set(key, value)
	}
	client := &http.Client{Timeout: 180 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("seeded request: %w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("read seeded response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("seeded request: HTTP %d: %s", response.StatusCode, truncateRunes(string(raw), 240))
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", fmt.Errorf("decode seeded response: %w: %s", err, truncateRunes(string(raw), 240))
	}
	if decoded.Error != nil {
		return "", fmt.Errorf("seeded request: %s", decoded.Error.Message)
	}
	if len(decoded.Choices) == 0 {
		return "", errors.New("seeded request: no choices")
	}
	return decoded.Choices[0].Message.Content, nil
}

func seededMessages(messages []*agent.Message, seed string) []map[string]string {
	out := make([]map[string]string, 0, len(messages)+1)
	for _, message := range messages {
		if message == nil {
			continue
		}
		role := string(message.Role)
		if role == string(agent.ToolRole) {
			continue
		}
		if role == string(agent.Assistant) {
			// Only the seed may appear as the assistant's turn.
			continue
		}
		out = append(out, map[string]string{"role": role, "content": message.Content})
	}
	if strings.TrimSpace(seed) == "" {
		return out
	}
	return append(out, map[string]string{"role": string(agent.Assistant), "content": seed})
}

func seededEndpoint(baseURL string) string {
	trimmed := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(trimmed, "/chat/completions") {
		return trimmed
	}
	if !strings.Contains(trimmed, "/v1") {
		trimmed += "/v1"
	}
	return trimmed + "/chat/completions"
}

func seededMaxTokens(configured *int) int {
	if configured != nil && *configured > 0 {
		return *configured
	}
	return 4096
}

func seededTemperature(configured *float32) float32 {
	if configured != nil {
		return *configured
	}
	return 0.9
}

func truncateRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}
