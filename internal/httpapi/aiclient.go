package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hkjang/muni/internal/settings"
)

// aiQuirks records the request shape a particular provider accepts. Every
// OpenAI-compatible gateway differs slightly — newer OpenAI models replaced
// max_tokens, some proxies reject stream_options, reasoning models refuse a
// temperature — and each mismatch comes back as a bare HTTP 400. We learn the
// working shape once per model and reuse it.
type aiQuirks struct {
	MaxCompletionTokens bool
	NoStreamOptions     bool
	NoTemperature       bool
	DeveloperRole       bool
	NoSystemRole        bool
	NoStreaming         bool
	TokenCap            int
	VersionedPath       bool
}

type aiCompatibility struct {
	mutex sync.RWMutex
	known map[string]aiQuirks
}

func newAICompatibility() *aiCompatibility {
	return &aiCompatibility{known: map[string]aiQuirks{}}
}

func (c *aiCompatibility) get(key string) aiQuirks {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.known[key]
}

func (c *aiCompatibility) set(key string, quirks aiQuirks) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.known[key] = quirks
}

const (
	defaultAIMaxTokens = 4096
	defaultAITimeout   = 120
)

// normalizeAI fills in sane values for settings an administrator never saved,
// so a half-configured install fails with a clear message instead of a 400.
func normalizeAI(config settings.AI) settings.AI {
	config.BaseURL = strings.TrimSpace(config.BaseURL)
	config.Model = strings.TrimSpace(config.Model)
	if config.MaxTokens < 1 || config.MaxTokens > settings.MaxAITokens {
		config.MaxTokens = defaultAIMaxTokens
	}
	if config.TimeoutSeconds < 5 || config.TimeoutSeconds > 3600 {
		config.TimeoutSeconds = defaultAITimeout
	}
	return config
}

// chatEndpoint builds the completions URL, tolerating base URLs that already
// include the path or omit the version segment.
func chatEndpoint(baseURL string, versioned bool) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(trimmed, "/chat/completions") {
		return trimmed
	}
	if strings.HasSuffix(trimmed, "/completions") {
		return trimmed
	}
	if versioned && !strings.HasSuffix(trimmed, "/v1") {
		trimmed += "/v1"
	}
	return trimmed + "/chat/completions"
}

var aiHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 120 * time.Second,
	},
}

type aiRequest struct {
	config      settings.AI
	messages    []aiMessage
	maxTokens   int
	temperature *float64
	stream      bool
}

type aiUpstreamError struct {
	status int
	body   string
}

func (e *aiUpstreamError) Error() string {
	if message := extractProviderMessage(e.body); message != "" {
		return fmt.Sprintf("AI API가 %d 상태를 반환했습니다: %s", e.status, message)
	}
	return fmt.Sprintf("AI API가 %d 상태를 반환했습니다: %s", e.status, truncate(e.body, 400))
}

// call performs the chat request, retrying with an adjusted payload whenever
// the provider rejects a parameter it does not implement.
func (s *Server) call(ctx context.Context, request aiRequest) (*http.Response, aiQuirks, error) {
	config := normalizeAI(request.config)
	if config.BaseURL == "" || config.Model == "" {
		return nil, aiQuirks{}, errors.New("AI API 주소와 모델 이름을 관리자 설정에서 입력해 주세요")
	}
	key := config.BaseURL + "|" + config.Model
	quirks := s.aiCompat.get(key)

	var lastError error
	for attempt := 0; attempt < 6; attempt++ {
		payload := buildAIPayload(config, request, quirks)
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, quirks, err
		}
		endpoint := chatEndpoint(config.BaseURL, quirks.VersionedPath)
		httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
		if err != nil {
			return nil, quirks, err
		}
		httpRequest.Header.Set("Content-Type", "application/json")
		if request.stream && !quirks.NoStreaming {
			httpRequest.Header.Set("Accept", "text/event-stream")
		} else {
			httpRequest.Header.Set("Accept", "application/json")
		}
		if config.APIKey != "" {
			httpRequest.Header.Set("Authorization", "Bearer "+config.APIKey)
			// Azure OpenAI and a few gateways use their own header instead.
			httpRequest.Header.Set("api-key", config.APIKey)
		}

		response, err := aiHTTPClient.Do(httpRequest)
		if err != nil {
			return nil, quirks, fmt.Errorf("AI API에 연결할 수 없습니다: %w", err)
		}
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			s.aiCompat.set(key, quirks)
			return response, quirks, nil
		}
		body, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		response.Body.Close()
		lastError = &aiUpstreamError{status: response.StatusCode, body: string(body)}

		adjusted, changed := adaptQuirks(quirks, response.StatusCode, string(body), request)
		if !changed {
			return nil, quirks, lastError
		}
		quirks = adjusted
	}
	return nil, quirks, lastError
}

func buildAIPayload(config settings.AI, request aiRequest, quirks aiQuirks) map[string]any {
	maxTokens := request.maxTokens
	if maxTokens < 1 {
		maxTokens = config.MaxTokens
	}
	if quirks.TokenCap > 0 && maxTokens > quirks.TokenCap {
		maxTokens = quirks.TokenCap
	}
	payload := map[string]any{
		"model":    config.Model,
		"messages": prepareMessages(request.messages, quirks),
	}
	if quirks.MaxCompletionTokens {
		payload["max_completion_tokens"] = maxTokens
	} else {
		payload["max_tokens"] = maxTokens
	}
	if request.stream && !quirks.NoStreaming {
		payload["stream"] = true
		if !quirks.NoStreamOptions {
			payload["stream_options"] = map[string]any{"include_usage": true}
		}
	}
	if request.temperature != nil && !quirks.NoTemperature {
		payload["temperature"] = *request.temperature
	}
	return payload
}

// prepareMessages drops empty turns and rewrites the system role for models
// that renamed or removed it.
func prepareMessages(messages []aiMessage, quirks aiQuirks) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		content := normalizeMessageContent(message.Content)
		if content == nil {
			continue
		}
		if text, ok := content.(string); ok && strings.TrimSpace(text) == "" {
			continue
		}
		role := message.Role
		if role == "system" {
			switch {
			case quirks.NoSystemRole:
				role = "user"
			case quirks.DeveloperRole:
				role = "developer"
			}
		}
		out = append(out, map[string]any{"role": role, "content": content})
	}
	if len(out) == 0 {
		out = append(out, map[string]any{"role": "user", "content": " "})
	}
	return out
}

// normalizeMessageContent flattens the structured-content form some clients
// send into the plain string every provider accepts.
func normalizeMessageContent(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return typed
	case []any:
		var builder strings.Builder
		for _, item := range typed {
			part, ok := item.(map[string]any)
			if !ok {
				return typed
			}
			if kind, _ := part["type"].(string); kind != "" && kind != "text" {
				return typed
			}
			text, _ := part["text"].(string)
			builder.WriteString(text)
		}
		return builder.String()
	default:
		return value
	}
}

var tokenLimitPattern = regexp.MustCompile(`(?i)(?:less than or equal to|at most|maximum(?: of)?|max(?:imum)? value|must be\s*<=?|支持最大|최대)\D{0,12}(\d{2,7})`)

// adaptQuirks inspects a rejection and decides how the next attempt should
// differ. It returns false when nothing about the request can be improved.
func adaptQuirks(quirks aiQuirks, status int, body string, request aiRequest) (aiQuirks, bool) {
	lower := strings.ToLower(body)

	if status == 404 && !quirks.VersionedPath {
		quirks.VersionedPath = true
		return quirks, true
	}
	if status != 400 && status != 404 && status != 422 && status != 413 {
		return quirks, false
	}

	unsupported := func(term string) bool {
		if !strings.Contains(lower, term) {
			return false
		}
		for _, marker := range []string{"unsupported", "not supported", "unrecognized", "unknown", "invalid", "deprecated", "extra fields", "not permitted", "unexpected"} {
			if strings.Contains(lower, marker) {
				return true
			}
		}
		return false
	}

	if strings.Contains(lower, "max_completion_tokens") && !quirks.MaxCompletionTokens {
		quirks.MaxCompletionTokens = true
		return quirks, true
	}
	if unsupported("max_tokens") && !quirks.MaxCompletionTokens {
		quirks.MaxCompletionTokens = true
		return quirks, true
	}
	if strings.Contains(lower, "stream_options") && !quirks.NoStreamOptions {
		quirks.NoStreamOptions = true
		return quirks, true
	}
	if strings.Contains(lower, "temperature") && !quirks.NoTemperature && request.temperature != nil {
		quirks.NoTemperature = true
		return quirks, true
	}
	if strings.Contains(lower, "developer") && strings.Contains(lower, "system") && !quirks.DeveloperRole {
		quirks.DeveloperRole = true
		return quirks, true
	}
	if unsupported("system") && !quirks.NoSystemRole {
		quirks.NoSystemRole = true
		quirks.DeveloperRole = false
		return quirks, true
	}
	if unsupported("stream") && !quirks.NoStreaming {
		quirks.NoStreaming = true
		return quirks, true
	}

	// "max_tokens must be <= 16384" and friends: honour the stated ceiling.
	if strings.Contains(lower, "token") {
		if match := tokenLimitPattern.FindStringSubmatch(body); match != nil {
			if limit, err := strconv.Atoi(match[1]); err == nil && limit > 0 {
				current := request.maxTokens
				if quirks.TokenCap > 0 && quirks.TokenCap < current {
					current = quirks.TokenCap
				}
				if limit < current {
					quirks.TokenCap = limit
					return quirks, true
				}
			}
		}
		if strings.Contains(lower, "too large") || strings.Contains(lower, "exceed") || strings.Contains(lower, "context length") {
			current := quirks.TokenCap
			if current == 0 {
				current = request.maxTokens
			}
			if halved := current / 2; halved >= 256 {
				quirks.TokenCap = halved
				return quirks, true
			}
		}
	}
	return quirks, false
}

// extractProviderMessage pulls the human-readable text out of the various
// error envelopes providers use, so operators see the real cause.
func extractProviderMessage(body string) string {
	var envelope struct {
		Error struct {
			Message string `json:"message"`
			Code    any    `json:"code"`
			Type    string `json:"type"`
			Param   string `json:"param"`
		} `json:"error"`
		Message string `json:"message"`
		Detail  any    `json:"detail"`
	}
	if json.Unmarshal([]byte(body), &envelope) != nil {
		return ""
	}
	if envelope.Error.Message != "" {
		message := envelope.Error.Message
		if envelope.Error.Param != "" {
			message += " (param: " + envelope.Error.Param + ")"
		}
		return truncate(message, 400)
	}
	if envelope.Message != "" {
		return truncate(envelope.Message, 400)
	}
	if envelope.Detail != nil {
		if text, ok := envelope.Detail.(string); ok {
			return truncate(text, 400)
		}
		if encoded, err := json.Marshal(envelope.Detail); err == nil {
			return truncate(string(encoded), 400)
		}
	}
	return ""
}

// writeSSEFromJSON converts a non-streaming completion into the event stream
// the editor expects, so providers without streaming still work.
func writeSSEFromJSON(w http.ResponseWriter, flusher http.Flusher, body []byte) {
	var response struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage any `json:"usage"`
	}
	content := ""
	if json.Unmarshal(body, &response) == nil && len(response.Choices) > 0 {
		if text, ok := normalizeMessageContent(response.Choices[0].Message.Content).(string); ok {
			content = text
		}
	}
	chunk := map[string]any{
		"object":  "chat.completion.chunk",
		"choices": []map[string]any{{"index": 0, "delta": map[string]any{"role": "assistant", "content": content}, "finish_reason": nil}},
	}
	encoded, _ := json.Marshal(chunk)
	fmt.Fprintf(w, "data: %s\n\n", encoded)
	final := map[string]any{
		"object":  "chat.completion.chunk",
		"choices": []map[string]any{{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}},
		"usage":   response.Usage,
	}
	encoded, _ = json.Marshal(final)
	fmt.Fprintf(w, "data: %s\n\n", encoded)
	io.WriteString(w, "data: [DONE]\n\n")
	flusher.Flush()
}
