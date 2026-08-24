package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/settings"
)

func newAIServer() *Server {
	return &Server{aiCompat: newAICompatibility()}
}

func TestChatEndpointBuilding(t *testing.T) {
	cases := []struct {
		base      string
		versioned bool
		want      string
	}{
		{"https://api.example.com/v1", false, "https://api.example.com/v1/chat/completions"},
		{"https://api.example.com/v1/", false, "https://api.example.com/v1/chat/completions"},
		{"https://api.example.com", true, "https://api.example.com/v1/chat/completions"},
		{"https://api.example.com/v1/chat/completions", false, "https://api.example.com/v1/chat/completions"},
	}
	for _, item := range cases {
		if got := chatEndpoint(item.base, item.versioned); got != item.want {
			t.Errorf("chatEndpoint(%q,%v) = %q, want %q", item.base, item.versioned, got, item.want)
		}
	}
}

// TestCallAdaptsToProviderRejections mirrors the real failure users hit: a
// gateway that rejects max_tokens, stream_options and an oversized limit, each
// with a bare 400.
func TestCallAdaptsToProviderRejections(t *testing.T) {
	var payloads []map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		payloads = append(payloads, payload)

		if _, ok := payload["max_tokens"]; ok {
			w.WriteHeader(400)
			io.WriteString(w, `{"error":{"message":"Unsupported parameter: 'max_tokens' is not supported with this model. Use 'max_completion_tokens' instead."}}`)
			return
		}
		if _, ok := payload["stream_options"]; ok {
			w.WriteHeader(400)
			io.WriteString(w, `{"error":{"message":"Unrecognized request argument supplied: stream_options"}}`)
			return
		}
		if limit, ok := payload["max_completion_tokens"].(float64); ok && limit > 4096 {
			w.WriteHeader(400)
			io.WriteString(w, `{"error":{"message":"max_completion_tokens must be less than or equal to 4096 tokens"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer upstream.Close()

	server := newAIServer()
	response, quirks, err := server.call(context.Background(), aiRequest{
		config:    settings.AI{BaseURL: upstream.URL + "/v1", Model: "gpt-test", MaxTokens: 32768, TimeoutSeconds: 30},
		messages:  []aiMessage{{Role: "user", Content: "안녕"}},
		maxTokens: 32768,
		stream:    true,
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	defer response.Body.Close()
	if !quirks.MaxCompletionTokens || !quirks.NoStreamOptions || quirks.TokenCap != 4096 {
		t.Fatalf("quirks not learned: %+v", quirks)
	}
	final := payloads[len(payloads)-1]
	if _, ok := final["max_tokens"]; ok {
		t.Error("final request still used max_tokens")
	}
	if final["max_completion_tokens"] != float64(4096) {
		t.Errorf("final max_completion_tokens = %v", final["max_completion_tokens"])
	}

	// The learned shape is reused, so the next call succeeds immediately.
	before := len(payloads)
	response2, _, err := server.call(context.Background(), aiRequest{
		config:    settings.AI{BaseURL: upstream.URL + "/v1", Model: "gpt-test", MaxTokens: 32768, TimeoutSeconds: 30},
		messages:  []aiMessage{{Role: "user", Content: "다시"}},
		maxTokens: 32768,
		stream:    true,
	})
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	response2.Body.Close()
	if len(payloads)-before != 1 {
		t.Errorf("expected one request on the second call, got %d", len(payloads)-before)
	}
}

func TestCallRetriesMissingVersionSegment(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(404)
			io.WriteString(w, `{"error":{"message":"not found"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer upstream.Close()

	server := newAIServer()
	response, quirks, err := server.call(context.Background(), aiRequest{
		config:    settings.AI{BaseURL: upstream.URL, Model: "m", MaxTokens: 100, TimeoutSeconds: 30},
		messages:  []aiMessage{{Role: "user", Content: "hi"}},
		maxTokens: 100,
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	response.Body.Close()
	if !quirks.VersionedPath {
		t.Fatal("expected the client to retry against /v1")
	}
}

func TestCallSurfacesProviderMessage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		io.WriteString(w, `{"error":{"message":"model 'nope' does not exist","param":"model"}}`)
	}))
	defer upstream.Close()

	server := newAIServer()
	_, _, err := server.call(context.Background(), aiRequest{
		config:    settings.AI{BaseURL: upstream.URL + "/v1", Model: "nope", MaxTokens: 100, TimeoutSeconds: 30},
		messages:  []aiMessage{{Role: "user", Content: "hi"}},
		maxTokens: 100,
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "does not exist") || !strings.Contains(err.Error(), "param: model") {
		t.Fatalf("provider message not surfaced: %v", err)
	}
}

func TestPrepareMessagesDropsEmptySystemPrompt(t *testing.T) {
	messages := prepareMessages([]aiMessage{
		{Role: "system", Content: "   "},
		{Role: "user", Content: []any{map[string]any{"type": "text", "text": "안"}, map[string]any{"type": "text", "text": "녕"}}},
	}, aiQuirks{})
	if len(messages) != 1 {
		t.Fatalf("expected the blank system turn to be dropped: %+v", messages)
	}
	if messages[0]["content"] != "안녕" {
		t.Fatalf("structured content was not flattened: %+v", messages[0])
	}
}

func TestNormalizeAIFillsMissingLimits(t *testing.T) {
	config := normalizeAI(settings.AI{BaseURL: " https://x/v1 ", Model: " m "})
	if config.MaxTokens != defaultAIMaxTokens || config.TimeoutSeconds != defaultAITimeout {
		t.Fatalf("defaults not applied: %+v", config)
	}
	if config.BaseURL != "https://x/v1" || config.Model != "m" {
		t.Fatalf("values not trimmed: %+v", config)
	}
}

func TestWriteSSEFromJSON(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeSSEFromJSON(recorder, recorder, []byte(`{"choices":[{"message":{"content":"안녕하세요"}}]}`))
	body := recorder.Body.String()
	if !strings.Contains(body, "안녕하세요") || !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("unexpected SSE output: %s", body)
	}
}
