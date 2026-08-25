package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/settings"
)

type aiMessage struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}
type aiChatInput struct {
	Messages    []aiMessage `json:"messages"`
	DocumentID  *uuid.UUID  `json:"documentId"`
	SessionID   *uuid.UUID  `json:"sessionId"`
	Action      string      `json:"action"`
	Tools       bool        `json:"tools"`
	MaxTokens   int         `json:"maxTokens"`
	Temperature *float64    `json:"temperature,omitempty"`
}

func (s *Server) aiChat(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	var input aiChatInput
	if !decodeJSON(w, r, &input) {
		return
	}
	if len(input.Messages) == 0 || len(input.Messages) > 100 {
		writeError(w, 400, "INVALID_MESSAGES", "AI 대화 메시지는 1~100개여야 합니다.")
		return
	}
	for _, message := range input.Messages {
		if !contains([]string{"system", "user", "assistant", "tool"}, message.Role) {
			writeError(w, 400, "INVALID_MESSAGE_ROLE", "AI 메시지 역할이 올바르지 않습니다.")
			return
		}
	}
	all, err := s.settings.GetAll(r.Context(), true)
	if err != nil || !all.AI.Enabled {
		writeError(w, 409, "AI_DISABLED", "관리자가 AI 기능을 설정하지 않았습니다.")
		return
	}
	config := normalizeAI(all.AI)
	if config.BaseURL == "" || config.Model == "" {
		writeError(w, 409, "AI_NOT_CONFIGURED", "관리자 설정에서 AI API 주소와 모델 이름을 입력해 주세요.")
		return
	}
	if input.MaxTokens < 0 {
		writeError(w, 400, "INVALID_MAX_TOKENS", "max token은 1 이상이어야 합니다.")
		return
	}
	// Requests above the configured ceiling are clamped rather than rejected:
	// the client cannot know the administrator's limit ahead of time.
	maxTokens := input.MaxTokens
	if maxTokens < 1 || maxTokens > config.MaxTokens {
		maxTokens = config.MaxTokens
	}
	if maxTokens > settings.MaxAITokens {
		maxTokens = settings.MaxAITokens
	}
	if input.Temperature != nil && (*input.Temperature < 0 || *input.Temperature > 2) {
		writeError(w, 400, "INVALID_TEMPERATURE", "temperature는 0~2 사이여야 합니다.")
		return
	}

	messages := make([]aiMessage, 0, len(input.Messages)+2)
	if prompt := strings.TrimSpace(config.SystemPrompt); prompt != "" {
		messages = append(messages, aiMessage{Role: "system", Content: prompt})
	}
	if input.DocumentID != nil {
		role, err := s.documentRole(r.Context(), p.User, *input.DocumentID, false)
		if err != nil || roleRank[role] < roleRank["VIEWER"] {
			writeError(w, 403, "DOCUMENT_PERMISSION_DENIED", "AI가 이 문서를 읽을 권한이 없습니다.")
			return
		}
		var title, text string
		if err := s.db.QueryRow(r.Context(), `SELECT title,content_text FROM documents WHERE id=$1`, *input.DocumentID).Scan(&title, &text); err != nil {
			writeError(w, 404, "DOCUMENT_NOT_FOUND", "문서를 찾을 수 없습니다.")
			return
		}
		text = truncateRunes(text, 220000)
		messages = append(messages, aiMessage{Role: "system", Content: "현재 문서 제목: " + title + "\n\n현재 문서 본문:\n" + text + "\n\n문서 밖의 정보라고 추정하지 말고, 답변에서 문서 기반인지 명확히 하세요."})
	}
	messages = append(messages, input.Messages...)

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(config.TimeoutSeconds)*time.Second)
	defer cancel()

	invocation := startAIInvocation(p.User, input.Action, config.Model, input.DocumentID, messageChars(messages))
	invocation.sessionID = input.SessionID
	invocation.note("maxTokens", maxTokens)

	if input.Tools {
		s.streamAgentAnswer(w, r, ctx, config, p, input, messages, maxTokens, invocation)
		return
	}

	response, quirks, err := s.call(ctx, aiRequest{
		config:      config,
		messages:    messages,
		maxTokens:   maxTokens,
		temperature: input.Temperature,
		stream:      true,
	})
	if err != nil {
		var upstream *aiUpstreamError
		status, code := 502, "AI_UPSTREAM_UNAVAILABLE"
		if errors.As(err, &upstream) {
			code = "AI_UPSTREAM_ERROR"
			s.audit(r, &p.User.ID, "AI_ERROR", "AI", input.DocumentID, map[string]any{"status": upstream.status, "model": config.Model})
			invocation.note("upstreamStatus", upstream.status)
			if upstream.status == 401 || upstream.status == 403 {
				code = "AI_UPSTREAM_UNAUTHORIZED"
			}
		}
		s.logger.Warn("ai upstream failed", "model", config.Model, "error", err.Error())
		invocation.fail(code, err)
		s.record(r.Context(), invocation)
		writeError(w, status, code, err.Error())
		return
	}
	defer response.Body.Close()

	flusher, ok := w.(http.Flusher)
	if !ok {
		invocation.fail("STREAMING_UNAVAILABLE", nil)
		s.record(r.Context(), invocation)
		writeError(w, 500, "STREAMING_UNAVAILABLE", "이 서버에서는 스트리밍을 사용할 수 없습니다.")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)

	counter := &usageCounter{}
	if quirks.NoStreaming || !strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "event-stream") {
		// The provider answered with a single JSON completion; replay it as one
		// SSE chunk so the editor's reader works unchanged.
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 8<<20))
		if readErr != nil {
			event, _ := json.Marshal(map[string]any{"error": "upstream_read_failed"})
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", event)
			flusher.Flush()
			invocation.fail("AI_UPSTREAM_READ_FAILED", readErr)
			s.record(r.Context(), invocation)
			return
		}
		counter.feedJSON(body)
		writeSSEFromJSON(w, flusher, body)
	} else {
		reader := bufio.NewReader(response.Body)
		for {
			line, readErr := reader.ReadBytes('\n')
			if len(line) > 0 {
				// The bytes go straight to the reader; the counter only looks
				// at them on the way past, so nothing is buffered.
				counter.feed(line)
				_, _ = w.Write(line)
				flusher.Flush()
			}
			if readErr != nil {
				if readErr != io.EOF {
					event, _ := json.Marshal(map[string]any{"error": "upstream_stream_interrupted"})
					_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", event)
					flusher.Flush()
					invocation.fail("AI_STREAM_INTERRUPTED", readErr)
				}
				break
			}
		}
	}

	action := invocation.action
	invocation.usage = counter.usage
	invocation.completionChars = counter.chars
	invocation.note("stream", !quirks.NoStreaming)
	s.record(r.Context(), invocation)
	s.audit(r, &p.User.ID, "AI_"+strings.ToUpper(action), "DOCUMENT", input.DocumentID, map[string]any{"model": config.Model, "maxTokens": maxTokens, "stream": !quirks.NoStreaming})
}

func (s *Server) testAI(w http.ResponseWriter, r *http.Request) {
	var input settings.AI
	if !decodeJSON(w, r, &input) {
		return
	}
	if strings.TrimSpace(input.BaseURL) == "" || strings.TrimSpace(input.Model) == "" {
		writeError(w, 400, "AI_CONFIG_REQUIRED", "AI API 주소와 모델 이름이 필요합니다.")
		return
	}
	if input.APIKey == "" {
		all, _ := s.settings.GetAll(r.Context(), true)
		input.APIKey = all.AI.APIKey
	}
	config := normalizeAI(input)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	response, quirks, err := s.call(ctx, aiRequest{
		config:    config,
		messages:  []aiMessage{{Role: "user", Content: "Reply with OK."}},
		maxTokens: 16,
	})
	if err != nil {
		writeError(w, 502, "AI_TEST_FAILED", err.Error())
		return
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	writeData(w, 200, map[string]any{
		"ok":       true,
		"model":    config.Model,
		"endpoint": chatEndpoint(config.BaseURL, quirks.VersionedPath),
		"adjustments": map[string]any{
			"maxCompletionTokens": quirks.MaxCompletionTokens,
			"noStreamOptions":     quirks.NoStreamOptions,
			"noTemperature":       quirks.NoTemperature,
			"noStreaming":         quirks.NoStreaming,
			"tokenCap":            quirks.TokenCap,
		},
		"response": json.RawMessage(body),
	})
}

func truncateRunes(value string, max int) string {
	if utf8.RuneCountInString(value) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max]) + "\n[…문서 컨텍스트가 길어 일부 생략됨…]"
}

// streamAgentAnswer runs the tool-using agent and reports it on the same event
// stream a plain chat uses, so the editor reads one protocol either way.
func (s *Server) streamAgentAnswer(
	w http.ResponseWriter,
	r *http.Request,
	ctx context.Context,
	config settings.AI,
	p principal,
	input aiChatInput,
	messages []aiMessage,
	maxTokens int,
	invocation *aiInvocation,
) {
	messages = append([]aiMessage{{
		Role: "system",
		Content: "필요하면 제공된 도구로 문서를 찾아 읽은 뒤 답하세요. " +
			"확인하지 않은 내용을 지어내지 말고, 근거가 된 문서의 제목을 답변에 밝히세요. " +
			"도구는 사용자가 접근할 수 있는 문서만 돌려줍니다.",
	}}, messages...)

	flusher, ok := w.(http.Flusher)
	if !ok {
		invocation.fail("STREAMING_UNAVAILABLE", nil)
		s.record(r.Context(), invocation)
		writeError(w, 500, "STREAMING_UNAVAILABLE", "이 서버에서는 스트리밍을 사용할 수 없습니다.")
		return
	}
	// The stream is opened before the agent starts, so each tool it runs and
	// each piece of the answer reaches the reader as it happens rather than
	// all at once when the whole run is over.
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)

	streamed := false
	answered := 0
	writeDelta := func(text string) {
		if text == "" {
			return
		}
		answered += len([]rune(text))
		chunk, _ := json.Marshal(map[string]any{
			"object": "chat.completion.chunk",
			"choices": []map[string]any{
				{"index": 0, "delta": map[string]any{"role": "assistant", "content": text}, "finish_reason": nil},
			},
		})
		fmt.Fprintf(w, "data: %s\n\n", chunk)
		flusher.Flush()
		streamed = true
	}

	run, err := s.runAgent(ctx, config, p.User, messages, maxTokens, input.Temperature,
		func(call agentCall) {
			event, _ := json.Marshal(map[string]any{
				"tool": call.Name, "arguments": call.Args, "error": call.Error,
			})
			fmt.Fprintf(w, "event: tool\ndata: %s\n\n", event)
			flusher.Flush()
			s.audit(r, &p.User.ID, "AI_TOOL_"+strings.ToUpper(call.Name), "DOCUMENT", input.DocumentID,
				map[string]any{"tool": call.Name, "error": call.Error})
		},
		writeDelta)
	if err != nil {
		// The response has already begun, so the failure has to travel on the
		// stream the client is reading.
		s.logger.Warn("ai agent failed", "model", config.Model, "error", err.Error())
		event, _ := json.Marshal(map[string]any{"error": err.Error()})
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", event)
		flusher.Flush()
		invocation.fail("AI_AGENT_FAILED", err)
		s.record(r.Context(), invocation)
		return
	}

	// A provider that could not stream answered in one piece; send it now.
	if !streamed {
		writeDelta(run.Answer)
	}
	final, _ := json.Marshal(map[string]any{
		"object":  "chat.completion.chunk",
		"choices": []map[string]any{{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}},
		"usage":   run.Usage,
	})
	fmt.Fprintf(w, "data: %s\n\n", final)
	io.WriteString(w, "data: [DONE]\n\n")
	flusher.Flush()

	names := make([]string, 0, len(run.Calls))
	for _, call := range run.Calls {
		names = append(names, call.Name)
	}
	if invocation.action == "chat" {
		invocation.action = "agent"
	}
	invocation.usage = readUsage(run.Usage)
	invocation.completionChars = answered
	invocation.toolCalls = len(run.Calls)
	invocation.note("tools", names)
	s.record(r.Context(), invocation)
	s.audit(r, &p.User.ID, "AI_"+strings.ToUpper(invocation.action), "DOCUMENT", input.DocumentID,
		map[string]any{"model": config.Model, "maxTokens": maxTokens, "toolCalls": len(run.Calls)})
}
