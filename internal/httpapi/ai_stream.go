package httpapi

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// streamedChunk is one server-sent event from an OpenAI-compatible provider.
type streamedChunk struct {
	Choices []struct {
		Delta struct {
			Role      string          `json:"role"`
			Content   any             `json:"content"`
			ToolCalls []toolCallDelta `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage any `json:"usage"`
}

// toolCallDelta is a tool call arriving in pieces: the name comes in the first
// fragment and the arguments accumulate across the rest, keyed by index.
type toolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// readCompletion reads one model turn, streamed or not.
//
// The agent used to wait for the whole run before sending anything, so a
// question that needed three tool calls looked frozen. Reading the turn as it
// arrives lets the answer appear while it is being written, and the caller is
// handed each fragment through onDelta.
func readCompletion(response *http.Response, onDelta func(string)) (assistantMessage, any, error) {
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if !strings.Contains(contentType, "event-stream") {
		return readJSONCompletion(response)
	}

	message := assistantMessage{Role: "assistant"}
	var usage any
	var content strings.Builder
	// Fragments are keyed by the index the provider assigns, which is the only
	// thing tying a name in one chunk to its arguments in the next.
	arguments := map[int]*toolCall{}
	order := make([]int, 0, 4)

	reader := bufio.NewReader(io.LimitReader(response.Body, 32<<20))
	for {
		line, err := reader.ReadString('\n')
		if payload, ok := sseData(line); ok {
			var chunk streamedChunk
			if json.Unmarshal([]byte(payload), &chunk) == nil {
				if chunk.Usage != nil {
					usage = chunk.Usage
				}
				for _, choice := range chunk.Choices {
					if text, ok := normalizeMessageContent(choice.Delta.Content).(string); ok && text != "" {
						content.WriteString(text)
						if onDelta != nil {
							onDelta(text)
						}
					}
					for _, fragment := range choice.Delta.ToolCalls {
						existing, seen := arguments[fragment.Index]
						if !seen {
							existing = &toolCall{Type: "function"}
							arguments[fragment.Index] = existing
							order = append(order, fragment.Index)
						}
						if fragment.ID != "" {
							existing.ID = fragment.ID
						}
						if fragment.Type != "" {
							existing.Type = fragment.Type
						}
						if fragment.Function.Name != "" {
							existing.Function.Name = fragment.Function.Name
						}
						existing.Function.Arguments += fragment.Function.Arguments
					}
				}
			}
		}
		if err != nil {
			break
		}
	}

	message.Content = content.String()
	for _, index := range order {
		if call := arguments[index]; call != nil && call.Function.Name != "" {
			message.ToolCalls = append(message.ToolCalls, *call)
		}
	}
	return message, usage, nil
}

// sseData returns the payload of a `data:` line, skipping keep-alives and the
// end marker.
func sseData(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "data:") {
		return "", false
	}
	payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
	if payload == "" || payload == "[DONE]" {
		return "", false
	}
	return payload, true
}

func readJSONCompletion(response *http.Response) (assistantMessage, any, error) {
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return assistantMessage{}, nil, err
	}
	var parsed completionResponse
	if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Choices) == 0 {
		return assistantMessage{}, nil, &aiFormatError{body: truncate(string(body), 300)}
	}
	return parsed.Choices[0].Message, parsed.Usage, nil
}

// aiFormatError is an answer muni could not read, kept apart from a transport
// failure so the message can say which happened.
type aiFormatError struct{ body string }

func (e *aiFormatError) Error() string {
	return "AI 응답 형식을 이해하지 못했습니다: " + e.body
}
