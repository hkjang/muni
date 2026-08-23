package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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
	Role    string `json:"role"`
	Content any    `json:"content"`
}
type aiChatInput struct {
	Messages    []aiMessage `json:"messages"`
	DocumentID  *uuid.UUID  `json:"documentId"`
	SessionID   *uuid.UUID  `json:"sessionId"`
	Action      string      `json:"action"`
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
	maxTokens := input.MaxTokens
	if maxTokens == 0 {
		maxTokens = all.AI.MaxTokens
	}
	if maxTokens < 1 || maxTokens > all.AI.MaxTokens || maxTokens > settings.MaxAITokens {
		writeError(w, 400, "INVALID_MAX_TOKENS", fmt.Sprintf("max token은 1~%d 범위여야 합니다.", min(all.AI.MaxTokens, settings.MaxAITokens)))
		return
	}
	messages := make([]aiMessage, 0, len(input.Messages)+2)
	messages = append(messages, aiMessage{Role: "system", Content: all.AI.SystemPrompt})
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
	payload := map[string]any{"model": all.AI.Model, "messages": messages, "stream": true, "stream_options": map[string]any{"include_usage": true}, "max_tokens": maxTokens}
	if input.Temperature != nil {
		payload["temperature"] = *input.Temperature
	}
	encoded, _ := json.Marshal(payload)
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(all.AI.TimeoutSeconds)*time.Second)
	defer cancel()
	endpoint := strings.TrimRight(all.AI.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		writeError(w, 500, "AI_REQUEST_FAILED", "AI 요청을 만들지 못했습니다.")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if all.AI.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+all.AI.APIKey)
	}
	response, err := (&http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, MaxIdleConns: 20, IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second}}).Do(req)
	if err != nil {
		writeError(w, 502, "AI_UPSTREAM_UNAVAILABLE", "AI API에 연결할 수 없습니다: "+err.Error())
		return
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		s.audit(r, &p.User.ID, "AI_ERROR", "AI", input.DocumentID, map[string]any{"status": response.StatusCode, "model": all.AI.Model})
		writeError(w, 502, "AI_UPSTREAM_ERROR", fmt.Sprintf("AI API가 %d 상태를 반환했습니다: %s", response.StatusCode, truncate(string(body), 500)))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "STREAMING_UNAVAILABLE", "이 서버에서는 스트리밍을 사용할 수 없습니다.")
		return
	}
	w.WriteHeader(200)
	reader := bufio.NewReader(response.Body)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			_, _ = w.Write(line)
			flusher.Flush()
		}
		if readErr != nil {
			if readErr != io.EOF {
				event, _ := json.Marshal(map[string]any{"error": "upstream_stream_interrupted"})
				_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", event)
				flusher.Flush()
			}
			break
		}
	}
	action := input.Action
	if action == "" {
		action = "chat"
	}
	_, _ = s.db.Exec(r.Context(), `INSERT INTO ai_actions(session_id,user_id,document_id,action,model,status,metadata) VALUES($1,$2,$3,$4,$5,'COMPLETED',$6)`, input.SessionID, p.User.ID, input.DocumentID, truncate(action, 80), all.AI.Model, map[string]any{"maxTokens": maxTokens, "stream": true})
	s.audit(r, &p.User.ID, "AI_"+strings.ToUpper(action), "DOCUMENT", input.DocumentID, map[string]any{"model": all.AI.Model, "maxTokens": maxTokens, "stream": true})
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
	payload, _ := json.Marshal(map[string]any{"model": input.Model, "messages": []map[string]string{{"role": "user", "content": "Reply with OK."}}, "stream": false, "max_tokens": 8})
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(input.BaseURL, "/")+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		writeError(w, 400, "AI_TEST_FAILED", err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if input.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+input.APIKey)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		writeError(w, 502, "AI_TEST_FAILED", "AI API에 연결할 수 없습니다: "+err.Error())
		return
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		writeError(w, 502, "AI_TEST_FAILED", fmt.Sprintf("AI API %d: %s", response.StatusCode, truncate(string(body), 500)))
		return
	}
	writeData(w, 200, map[string]any{"ok": true, "model": input.Model, "response": json.RawMessage(body)})
}

func truncateRunes(value string, max int) string {
	if utf8.RuneCountInString(value) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max]) + "\n[…문서 컨텍스트가 길어 일부 생략됨…]"
}
