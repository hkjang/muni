package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// aiUsage is what the provider reported it spent. Not every provider sends it,
// so every field is optional and a zero means "not reported" rather than zero.
type aiUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

// readUsage picks the token counts out of whatever shape the provider used.
func readUsage(value any) aiUsage {
	if value == nil {
		return aiUsage{}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return aiUsage{}
	}
	var usage aiUsage
	if json.Unmarshal(encoded, &usage) != nil {
		return aiUsage{}
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	return usage
}

// aiInvocation is one AI call, as the audit records it.
//
// The content of the prompt and of the answer is deliberately not stored:
// documents are the reason people hesitate to turn AI on, and a size is enough
// to answer a question about cost. What is stored is who asked, for what, on
// which document, how long it took and how it ended.
type aiInvocation struct {
	sessionID       *uuid.UUID
	userID          uuid.UUID
	documentID      *uuid.UUID
	action          string
	model           string
	status          string
	errorCode       string
	errorText       string
	promptChars     int
	completionChars int
	toolCalls       int
	usage           aiUsage
	started         time.Time
	metadata        map[string]any
}

const (
	aiStatusCompleted = "COMPLETED"
	aiStatusFailed    = "FAILED"
)

// startAIInvocation opens a record for a call that is about to be made.
func startAIInvocation(user User, action, model string, documentID *uuid.UUID, promptChars int) *aiInvocation {
	if action == "" {
		action = "chat"
	}
	return &aiInvocation{
		userID:      user.ID,
		documentID:  documentID,
		action:      truncate(action, 80),
		model:       model,
		status:      aiStatusCompleted,
		promptChars: promptChars,
		started:     time.Now(),
		metadata:    map[string]any{},
	}
}

func (a *aiInvocation) fail(code string, err error) {
	a.status = aiStatusFailed
	a.errorCode = code
	if err != nil {
		a.errorText = truncate(err.Error(), 500)
	}
}

func (a *aiInvocation) note(key string, value any) {
	if a.metadata == nil {
		a.metadata = map[string]any{}
	}
	a.metadata[key] = value
}

// record writes the call to the audit table.
//
// It runs on a context detached from the request on purpose: a reader who
// closes the tab cancels the request, and a cancelled call is exactly the one
// an administrator wants to see in the record.
func (s *Server) record(ctx context.Context, invocation *aiInvocation) {
	if invocation == nil || s.db == nil {
		return
	}
	detached, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	metadata := invocation.metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	_, err := s.db.Exec(detached, `
		INSERT INTO ai_actions(session_id,user_id,document_id,action,model,status,
			prompt_tokens,completion_tokens,prompt_chars,completion_chars,
			duration_ms,error_code,error_message,tool_calls,metadata)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		invocation.sessionID, invocation.userID, invocation.documentID,
		invocation.action, invocation.model, invocation.status,
		nullInt64(invocation.usage.PromptTokens), nullInt64(invocation.usage.CompletionTokens),
		int64(invocation.promptChars), int64(invocation.completionChars),
		int(time.Since(invocation.started).Milliseconds()),
		nullString(invocation.errorCode), nullString(invocation.errorText),
		invocation.toolCalls, metadata)
	if err != nil {
		s.logger.Warn("ai audit was not recorded", "action", invocation.action, "error", err)
	}
}

func nullInt64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

// messageChars is the size of what was sent, which stands in for the prompt
// when the prompt itself is not stored.
func messageChars(messages []aiMessage) int {
	total := 0
	for _, message := range messages {
		if text, ok := normalizeMessageContent(message.Content).(string); ok {
			total += len([]rune(text))
		}
	}
	return total
}

// usageCounter follows a stream that is being copied to the client, picking out
// the token usage and the size of the answer without holding either.
type usageCounter struct {
	usage aiUsage
	chars int
}

func (c *usageCounter) feed(line []byte) {
	payload, ok := sseData(string(line))
	if !ok {
		return
	}
	var chunk streamedChunk
	if json.Unmarshal([]byte(payload), &chunk) != nil {
		return
	}
	if chunk.Usage != nil {
		if usage := readUsage(chunk.Usage); usage.TotalTokens > 0 {
			c.usage = usage
		}
	}
	for _, choice := range chunk.Choices {
		if text, ok := normalizeMessageContent(choice.Delta.Content).(string); ok {
			c.chars += len([]rune(text))
		}
	}
}

// feedJSON reads a whole non-streamed completion.
func (c *usageCounter) feedJSON(body []byte) {
	var parsed completionResponse
	if json.Unmarshal(body, &parsed) != nil {
		return
	}
	if usage := readUsage(parsed.Usage); usage.TotalTokens > 0 {
		c.usage = usage
	}
	for _, choice := range parsed.Choices {
		if text, ok := normalizeMessageContent(choice.Message.Content).(string); ok {
			c.chars += len([]rune(text))
		}
	}
}

// listAIUsage answers the admin screen: the calls themselves, and the totals
// that go above them.
func (s *Server) listAIUsage(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit := parseLimit(query.Get("limit"), 100)
	// parseLimit caps at 100, which is right for a row count and wrong for a
	// number of days.
	days := 30
	if value, err := strconv.Atoi(strings.TrimSpace(query.Get("days"))); err == nil && value >= 1 && value <= 365 {
		days = value
	}
	status := strings.ToUpper(strings.TrimSpace(query.Get("status")))
	if status != aiStatusCompleted && status != aiStatusFailed {
		status = ""
	}
	action := strings.TrimSpace(query.Get("action"))
	var user *uuid.UUID
	if raw := strings.TrimSpace(query.Get("userId")); raw != "" {
		if parsed, err := uuid.Parse(raw); err == nil {
			user = &parsed
		}
	}
	since := time.Now().AddDate(0, 0, -days)

	where := `WHERE a.created_at >= $1
		AND ($2 = '' OR a.status = $2)
		AND ($3 = '' OR a.action = $3)
		AND ($4::uuid IS NULL OR a.user_id = $4)`

	var summary struct {
		Calls     int64 `json:"calls"`
		Failed    int64 `json:"failed"`
		Tokens    int64 `json:"tokens"`
		PromptTok int64 `json:"promptTokens"`
		OutputTok int64 `json:"completionTokens"`
		People    int64 `json:"people"`
		AvgMs     int64 `json:"averageMs"`
		ToolCalls int64 `json:"toolCalls"`
	}
	err := s.db.QueryRow(r.Context(), `
		SELECT count(*),
			count(*) FILTER (WHERE a.status <> 'COMPLETED'),
			coalesce(sum(coalesce(a.prompt_tokens,0) + coalesce(a.completion_tokens,0)),0),
			coalesce(sum(coalesce(a.prompt_tokens,0)),0),
			coalesce(sum(coalesce(a.completion_tokens,0)),0),
			count(DISTINCT a.user_id),
			coalesce(round(avg(a.duration_ms)),0),
			coalesce(sum(a.tool_calls),0)
		FROM ai_actions a `+where,
		since, status, action, user).Scan(
		&summary.Calls, &summary.Failed, &summary.Tokens, &summary.PromptTok,
		&summary.OutputTok, &summary.People, &summary.AvgMs, &summary.ToolCalls)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "AI 호출 기록을 불러오지 못했습니다.")
		return
	}

	byAction := make([]map[string]any, 0)
	rows, err := s.db.Query(r.Context(), `
		SELECT a.action, count(*),
			count(*) FILTER (WHERE a.status <> 'COMPLETED'),
			coalesce(sum(coalesce(a.prompt_tokens,0) + coalesce(a.completion_tokens,0)),0)
		FROM ai_actions a `+where+`
		GROUP BY a.action ORDER BY count(*) DESC LIMIT 20`,
		since, status, action, user)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			var calls, failed, tokens int64
			if rows.Scan(&name, &calls, &failed, &tokens) == nil {
				byAction = append(byAction, map[string]any{
					"action": name, "calls": calls, "failed": failed, "tokens": tokens,
				})
			}
		}
	}

	items := make([]map[string]any, 0, limit)
	list, err := s.db.Query(r.Context(), `
		SELECT a.id, a.user_id, u.display_name, a.document_id, d.title, a.action, a.model,
			a.status, a.prompt_tokens, a.completion_tokens, a.prompt_chars, a.completion_chars,
			a.duration_ms, a.error_code, a.error_message, a.tool_calls, a.metadata, a.created_at
		FROM ai_actions a
		LEFT JOIN users u ON u.id = a.user_id
		LEFT JOIN documents d ON d.id = a.document_id `+where+`
		ORDER BY a.created_at DESC LIMIT $5`,
		since, status, action, user, limit)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "AI 호출 기록을 불러오지 못했습니다.")
		return
	}
	defer list.Close()
	for list.Next() {
		var id uuid.UUID
		var userID uuid.UUID
		var displayName *string
		var documentID *uuid.UUID
		var title *string
		var name, model, itemStatus string
		var promptTokens, completionTokens, promptChars, completionChars *int64
		var duration *int32
		var errorCode, errorMessage *string
		var toolCalls int32
		var metadata any
		var created time.Time
		if list.Scan(&id, &userID, &displayName, &documentID, &title, &name, &model,
			&itemStatus, &promptTokens, &completionTokens, &promptChars, &completionChars,
			&duration, &errorCode, &errorMessage, &toolCalls, &metadata, &created) != nil {
			continue
		}
		items = append(items, map[string]any{
			"id": id, "userId": userID, "userName": displayName,
			"documentId": documentID, "documentTitle": title,
			"action": name, "model": model, "status": itemStatus,
			"promptTokens": promptTokens, "completionTokens": completionTokens,
			"promptChars": promptChars, "completionChars": completionChars,
			"durationMs": duration, "errorCode": errorCode, "errorMessage": errorMessage,
			"toolCalls": toolCalls, "metadata": metadata, "createdAt": created,
		})
	}

	writeData(w, 200, map[string]any{
		"summary":  summary,
		"byAction": byAction,
		"items":    items,
		"days":     days,
	})
}
