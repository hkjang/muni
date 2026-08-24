package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/settings"
)

// stubProvider answers each request with the next scripted completion and
// records what it was sent.
func stubProvider(t *testing.T, replies ...string) (*httptest.Server, *[]map[string]any) {
	t.Helper()
	seen := make([]map[string]any, 0, len(replies))
	round := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		seen = append(seen, payload)
		w.Header().Set("Content-Type", "application/json")
		if round < len(replies) {
			io.WriteString(w, replies[round])
			round++
			return
		}
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"끝"}}]}`)
	}))
	t.Cleanup(server.Close)
	return server, &seen
}

func agentConfig(url string) settings.AI {
	return settings.AI{BaseURL: url + "/v1", Model: "test", MaxTokens: 1000, TimeoutSeconds: 30}
}

func toolCallReply(id, name, args string) string {
	encoded, _ := json.Marshal(args)
	return `{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[` +
		`{"id":"` + id + `","type":"function","function":{"name":"` + name + `","arguments":` + string(encoded) + `}}]}}]}`
}

func TestAgentAnswersWithoutToolsWhenNoneAreNeeded(t *testing.T) {
	provider, seen := stubProvider(t, `{"choices":[{"message":{"role":"assistant","content":"바로 답합니다"}}]}`)
	server := newAIServer()

	run, err := server.runAgent(context.Background(), agentConfig(provider.URL), User{},
		[]aiMessage{{Role: "user", Content: "안녕"}}, 500, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if run.Answer != "바로 답합니다" || len(run.Calls) != 0 {
		t.Fatalf("unexpected run: %+v", run)
	}
	// The tools have to be offered for the model to be able to choose them.
	if _, ok := (*seen)[0]["tools"]; !ok {
		t.Fatal("the request did not offer any tools")
	}
}

func TestAgentReportsAToolFailureBackToTheModel(t *testing.T) {
	provider, seen := stubProvider(t,
		toolCallReply("call_1", "read_document", `{"documentId":"not-a-uuid"}`),
		`{"choices":[{"message":{"role":"assistant","content":"문서를 찾지 못했습니다"}}]}`)
	server := newAIServer()

	observed := make([]agentCall, 0, 1)
	run, err := server.runAgent(context.Background(), agentConfig(provider.URL), User{},
		[]aiMessage{{Role: "user", Content: "읽어줘"}}, 500, nil,
		func(call agentCall) { observed = append(observed, call) })
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Calls) != 1 || run.Calls[0].Error == "" {
		t.Fatalf("the failure was not recorded: %+v", run.Calls)
	}
	if len(observed) != 1 {
		t.Fatalf("the caller was not told about the call: %+v", observed)
	}
	// A bad argument is something the model can correct, so the run continues.
	if run.Answer != "문서를 찾지 못했습니다" {
		t.Fatalf("answer = %q", run.Answer)
	}
	// The second round must carry the assistant turn and the tool result.
	second := (*seen)[1]["messages"].([]any)
	roles := make([]string, 0, len(second))
	for _, item := range second {
		roles = append(roles, item.(map[string]any)["role"].(string))
	}
	if !contains(roles, "tool") || !contains(roles, "assistant") {
		t.Fatalf("tool plumbing was dropped: %v", roles)
	}
}

func TestAgentStopsAfterTheRoundLimit(t *testing.T) {
	replies := make([]string, 0, maxToolRounds)
	for index := 0; index < maxToolRounds; index++ {
		replies = append(replies, toolCallReply("call", "search_documents", `{"query":"계획"}`))
	}
	replies = append(replies, `{"choices":[{"message":{"role":"assistant","content":"정리하면 이렇습니다"}}]}`)
	provider, _ := stubProvider(t, replies...)

	server := newAIServer()
	// The tool reaches for a database this server does not have. runTool turns
	// that into an error the model is told about, which is exactly the path
	// that has to keep the loop alive.
	run, err := server.runAgent(context.Background(), agentConfig(provider.URL), User{},
		[]aiMessage{{Role: "user", Content: "계속 찾아봐"}}, 500, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Calls) != maxToolRounds {
		t.Fatalf("expected one call per round, got %d", len(run.Calls))
	}
	if run.Answer == "" {
		t.Fatal("the reader must still get an answer when the rounds run out")
	}
}

func TestAgentDegradesWhenTheProviderRejectsTools(t *testing.T) {
	round := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		if _, ok := payload["tools"]; ok {
			w.WriteHeader(400)
			io.WriteString(w, `{"error":{"message":"Unsupported parameter: tools"}}`)
			return
		}
		round++
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"도구 없이 답합니다"}}]}`)
	}))
	defer provider.Close()

	server := newAIServer()
	run, err := server.runAgent(context.Background(), agentConfig(provider.URL), User{},
		[]aiMessage{{Role: "user", Content: "질문"}}, 500, nil, nil)
	if err != nil {
		t.Fatalf("a provider without tool calling should still answer: %v", err)
	}
	if run.Answer != "도구 없이 답합니다" {
		t.Fatalf("answer = %q", run.Answer)
	}
}

func TestAgentSurfacesAnUpstreamFailure(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(401)
		io.WriteString(w, `{"error":{"message":"invalid api key"}}`)
	}))
	defer provider.Close()

	server := newAIServer()
	_, err := server.runAgent(context.Background(), agentConfig(provider.URL), User{},
		[]aiMessage{{Role: "user", Content: "질문"}}, 500, nil, nil)
	if err == nil {
		t.Fatal("expected the upstream failure to reach the caller")
	}
	var upstream *aiUpstreamError
	if !errors.As(err, &upstream) || !strings.Contains(err.Error(), "invalid api key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWriteAgentEventsReportsToolsThenTheAnswer(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeAgentEvents(recorder, recorder, agentRun{
		Answer: "정리했습니다",
		Calls: []agentCall{
			{Name: "search_documents", Args: `{"query":"계획"}`},
			{Name: "read_document", Args: `{"documentId":"x"}`, Error: "권한이 없습니다"},
		},
	})
	body := recorder.Body.String()
	if strings.Index(body, "event: tool") > strings.Index(body, "정리했습니다") {
		t.Fatal("tool events must arrive before the answer")
	}
	for _, expected := range []string{"search_documents", "read_document", "권한이 없습니다", "정리했습니다", "data: [DONE]"} {
		if !strings.Contains(body, expected) {
			t.Errorf("event stream missing %q:\n%s", expected, body)
		}
	}
}

func TestToolDefinitionsDescribeEveryTool(t *testing.T) {
	definitions := toolDefinitions(aiTools())
	if len(definitions) == 0 {
		t.Fatal("no tools were defined")
	}
	names := map[string]bool{}
	for _, definition := range definitions {
		function := definition["function"].(map[string]any)
		name := function["name"].(string)
		if names[name] {
			t.Errorf("duplicate tool name %q", name)
		}
		names[name] = true
		if description, _ := function["description"].(string); len(description) < 10 {
			t.Errorf("%s has no useful description", name)
		}
		if _, ok := function["parameters"].(map[string]any)["properties"]; !ok {
			t.Errorf("%s declares no parameters", name)
		}
	}
	for _, expected := range []string{"search_documents", "read_document", "compare_revisions"} {
		if !names[expected] {
			t.Errorf("missing tool %s", expected)
		}
	}
}

func TestUnknownToolIsRejected(t *testing.T) {
	_, err := runTool(context.Background(), newAIServer(), User{}, "delete_everything", "{}")
	if !errors.Is(err, errToolNotFound) {
		t.Fatalf("unexpected error: %v", err)
	}
}
