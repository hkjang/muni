package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/hkjang/muni/internal/settings"
)

// toolCall is one function call the model asked for.
type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type assistantMessage struct {
	Role      string     `json:"role"`
	Content   any        `json:"content"`
	ToolCalls []toolCall `json:"tool_calls,omitempty"`
}

type completionResponse struct {
	Choices []struct {
		Message      assistantMessage `json:"message"`
		FinishReason string           `json:"finish_reason"`
	} `json:"choices"`
	Usage any `json:"usage"`
}

// agentRun is the outcome of letting the model use tools to answer.
type agentRun struct {
	Answer string
	Calls  []agentCall
	Usage  any
}

type agentCall struct {
	Name  string `json:"name"`
	Args  string `json:"arguments"`
	Error string `json:"error,omitempty"`
}

// runAgent lets the model look things up before it answers.
//
// Each round is a plain completion rather than a stream: a streamed tool call
// arrives as fragments that have to be reassembled before anything can run, and
// the tokens cannot be shown to the reader anyway until it is known whether
// they are an answer or a function call. Progress is reported instead through
// the tool events the caller emits.
func (s *Server) runAgent(
	ctx context.Context,
	config settings.AI,
	user User,
	messages []aiMessage,
	maxTokens int,
	temperature *float64,
	onCall func(agentCall),
) (agentRun, error) {
	tools := aiTools()
	definitions := toolDefinitions(tools)
	conversation := append([]aiMessage{}, messages...)
	run := agentRun{}

	for round := 0; round < maxToolRounds; round++ {
		response, quirks, err := s.call(ctx, aiRequest{
			config:      config,
			messages:    conversation,
			maxTokens:   maxTokens,
			temperature: temperature,
			tools:       definitions,
		})
		if err != nil {
			return run, err
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 8<<20))
		response.Body.Close()
		if readErr != nil {
			return run, fmt.Errorf("AI 응답을 읽지 못했습니다: %w", readErr)
		}

		var parsed completionResponse
		if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Choices) == 0 {
			return run, fmt.Errorf("AI 응답 형식을 이해하지 못했습니다: %s", truncate(string(body), 300))
		}
		run.Usage = parsed.Usage
		message := parsed.Choices[0].Message

		// A provider that cannot do tool calling degrades to a plain answer;
		// the adaptive client has already dropped the parameter by this point.
		if len(message.ToolCalls) == 0 || quirks.NoTools {
			run.Answer = contentText(message.Content)
			return run, nil
		}

		conversation = append(conversation, aiMessage{
			Role: "assistant", Content: message.Content, ToolCalls: message.ToolCalls,
		})
		for _, call := range message.ToolCalls {
			if len(run.Calls) >= maxToolCallsTotal {
				conversation = append(conversation, toolResult(call.ID,
					map[string]any{"error": "도구 호출 한도에 도달했습니다. 지금까지 얻은 정보로 답하세요."}))
				continue
			}
			record := agentCall{Name: call.Function.Name, Args: call.Function.Arguments}
			result, err := runTool(ctx, s, user, call.Function.Name, call.Function.Arguments)
			if err != nil {
				// Tool failures are reported back to the model rather than
				// ending the run: a wrong argument is something it can fix.
				record.Error = err.Error()
				result = map[string]any{"error": err.Error()}
			}
			run.Calls = append(run.Calls, record)
			if onCall != nil {
				onCall(record)
			}
			conversation = append(conversation, toolResult(call.ID, result))
		}
	}

	// Out of rounds: ask once more without tools so the reader gets an answer
	// rather than silence.
	conversation = append(conversation, aiMessage{
		Role:    "system",
		Content: "도구를 더 부르지 말고 지금까지 확인한 내용만으로 답하세요.",
	})
	response, _, err := s.call(ctx, aiRequest{
		config: config, messages: conversation, maxTokens: maxTokens, temperature: temperature,
	})
	if err != nil {
		return run, err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	var parsed completionResponse
	if json.Unmarshal(body, &parsed) == nil && len(parsed.Choices) > 0 {
		run.Answer = contentText(parsed.Choices[0].Message.Content)
	}
	return run, nil
}

func toolResult(id string, value any) aiMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		encoded = []byte(`{"error":"도구 결과를 직렬화하지 못했습니다"}`)
	}
	return aiMessage{Role: "tool", Content: string(encoded), ToolCallID: id}
}

func contentText(value any) string {
	if text, ok := normalizeMessageContent(value).(string); ok {
		// A model that reasons out loud must not have its working mistaken for
		// the answer, least of all by the patch parser looking for JSON.
		return stripReasoning(text)
	}
	return ""
}

// writeAgentEvents replays an agent run as the event stream the editor reads:
// one event per tool call so the reader can see what the agent looked at, then
// the answer.
func writeAgentEvents(w http.ResponseWriter, flusher http.Flusher, run agentRun) {
	for _, call := range run.Calls {
		event, _ := json.Marshal(map[string]any{
			"tool": call.Name, "arguments": call.Args, "error": call.Error,
		})
		fmt.Fprintf(w, "event: tool\ndata: %s\n\n", event)
		flusher.Flush()
	}
	chunk, _ := json.Marshal(map[string]any{
		"object": "chat.completion.chunk",
		"choices": []map[string]any{
			{"index": 0, "delta": map[string]any{"role": "assistant", "content": run.Answer}, "finish_reason": nil},
		},
	})
	fmt.Fprintf(w, "data: %s\n\n", chunk)
	final, _ := json.Marshal(map[string]any{
		"object":  "chat.completion.chunk",
		"choices": []map[string]any{{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}},
		"usage":   run.Usage,
	})
	fmt.Fprintf(w, "data: %s\n\n", final)
	io.WriteString(w, "data: [DONE]\n\n")
	flusher.Flush()
}
