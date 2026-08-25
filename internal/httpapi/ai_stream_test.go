package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sseProvider answers with the given event-stream bodies, one per round.
func sseProvider(t *testing.T, rounds ...string) *httptest.Server {
	t.Helper()
	round := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		body := "data: [DONE]\n\n"
		if round < len(rounds) {
			body = rounds[round]
			round++
		}
		io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}

func chunk(content string) string {
	return `data: {"choices":[{"index":0,"delta":{"role":"assistant","content":"` + content + `"}}]}` + "\n\n"
}

func TestStreamedAnswerReachesTheReaderInPieces(t *testing.T) {
	provider := sseProvider(t, chunk("안녕")+chunk("하세요")+"data: [DONE]\n\n")
	server := newAIServer()

	seen := make([]string, 0, 2)
	run, err := server.runAgent(context.Background(), agentConfig(provider.URL), User{},
		[]aiMessage{{Role: "user", Content: "인사"}}, 500, nil, nil,
		func(delta string) { seen = append(seen, delta) })
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || seen[0] != "안녕" || seen[1] != "하세요" {
		t.Fatalf("the answer did not arrive in pieces: %#v", seen)
	}
	if run.Answer != "안녕하세요" {
		t.Fatalf("assembled answer = %q", run.Answer)
	}
}

func TestStreamedToolCallIsReassembledFromItsFragments(t *testing.T) {
	// The name arrives in one chunk and the arguments across the next two,
	// which is how every provider that streams tool calls sends them.
	stream := `data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"search_documents","arguments":""}}]}}]}` + "\n\n" +
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"query\":"}}]}}]}` + "\n\n" +
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"보고서\"}"}}]}}]}` + "\n\n" +
		"data: [DONE]\n\n"
	provider := sseProvider(t, stream, chunk("찾았습니다")+"data: [DONE]\n\n")
	server := newAIServer()

	observed := make([]agentCall, 0, 1)
	run, err := server.runAgent(context.Background(), agentConfig(provider.URL), User{},
		[]aiMessage{{Role: "user", Content: "찾아줘"}}, 500, nil,
		func(call agentCall) { observed = append(observed, call) },
		func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(observed) != 1 {
		t.Fatalf("expected one tool call, got %#v", observed)
	}
	if observed[0].Name != "search_documents" {
		t.Fatalf("tool name = %q", observed[0].Name)
	}
	if observed[0].Args != `{"query":"보고서"}` {
		t.Fatalf("arguments were not reassembled: %q", observed[0].Args)
	}
	if run.Answer != "찾았습니다" {
		t.Fatalf("answer = %q", run.Answer)
	}
}

func TestStreamFallsBackWhenTheProviderAnswersWithJSON(t *testing.T) {
	// Asking for a stream does not guarantee getting one.
	provider, _ := stubProvider(t, `{"choices":[{"message":{"role":"assistant","content":"한 번에"}}]}`)
	server := newAIServer()

	deltas := 0
	run, err := server.runAgent(context.Background(), agentConfig(provider.URL), User{},
		[]aiMessage{{Role: "user", Content: "질문"}}, 500, nil, nil,
		func(string) { deltas++ })
	if err != nil {
		t.Fatal(err)
	}
	if run.Answer != "한 번에" {
		t.Fatalf("answer = %q", run.Answer)
	}
	if deltas != 0 {
		t.Fatalf("a JSON answer has no deltas, got %d", deltas)
	}
}

func TestStreamIgnoresKeepAlivesAndComments(t *testing.T) {
	stream := ": keep-alive\n\n" + "data: \n\n" + chunk("답") + "data: [DONE]\n\n"
	provider := sseProvider(t, stream)
	server := newAIServer()

	run, err := server.runAgent(context.Background(), agentConfig(provider.URL), User{},
		[]aiMessage{{Role: "user", Content: "질문"}}, 500, nil, nil, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(run.Answer) != "답" {
		t.Fatalf("answer = %q", run.Answer)
	}
}
