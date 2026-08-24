package ptium

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testConfig(url string) Config {
	return Config{Enabled: true, BaseURL: url, APIKey: "ptium_test", DefaultTheme: "aurora", DefaultLang: "ko"}
}

func TestConfigNormalizeFillsTheGaps(t *testing.T) {
	config := Config{BaseURL: "https://ptium.example.com/ ", APIKey: " ptium_k "}.Normalize()
	if config.BaseURL != "https://ptium.example.com" {
		t.Errorf("base url = %q", config.BaseURL)
	}
	if config.APIKey != "ptium_k" {
		t.Errorf("api key was not trimmed: %q", config.APIKey)
	}
	if config.DefaultLang != "ko" || config.TimeoutSeconds != defaultTimeoutSeconds {
		t.Errorf("defaults not applied: %+v", config)
	}
	// The editor link falls back to the API origin, which is the usual setup.
	if config.WebURL != "https://ptium.example.com" {
		t.Errorf("web url = %q", config.WebURL)
	}
	if link := config.EditorURL("abc"); link != "https://ptium.example.com/presentations/abc" {
		t.Errorf("editor link = %q", link)
	}
}

func TestConfigUsable(t *testing.T) {
	cases := []struct {
		name   string
		config Config
		want   bool
	}{
		{"complete", Config{Enabled: true, BaseURL: "https://p.example.com", APIKey: "k"}, true},
		{"disabled", Config{BaseURL: "https://p.example.com", APIKey: "k"}, false},
		{"no key", Config{Enabled: true, BaseURL: "https://p.example.com"}, false},
		{"not a url", Config{Enabled: true, BaseURL: "ptium", APIKey: "k"}, false},
	}
	for _, item := range cases {
		if got := item.config.Normalize().Usable(); got != item.want {
			t.Errorf("%s: Usable() = %v", item.name, got)
		}
	}
}

func TestGenerateSendsTheBriefAsAPrompt(t *testing.T) {
	var seen generateRequest
	var key string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/presentations/generate" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		key = r.Header.Get("X-API-Key")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &seen)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"pres-1","title":"사업 추진 계획","status":"queued"}`)
	}))
	defer server.Close()

	brief := Brief{
		Source:       BriefSource{Title: "사업 추진 계획"},
		Presentation: BriefPreferences{Title: "사업 추진 계획"},
		Sections:     []Section{{Title: "추진 배경", Blocks: []Block{{Kind: BlockParagraph, Text: "42개 시스템"}}}},
	}
	client := NewClient(testConfig(server.URL))
	presentation, err := client.Generate(context.Background(), brief,
		Options{Audience: "executive", Tone: "concise", SlideCount: 10})
	if err != nil {
		t.Fatal(err)
	}
	if presentation.ID != "pres-1" || presentation.Status != "queued" {
		t.Fatalf("unexpected presentation: %+v", presentation)
	}
	if key != "ptium_test" {
		t.Errorf("the api key was not sent: %q", key)
	}
	if seen.RequestedSlideCount != 10 || seen.Audience != "executive" {
		t.Errorf("options were not forwarded: %+v", seen)
	}
	// The theme and language fall back to what the administrator configured.
	if seen.Theme != "aurora" || seen.Language != "ko" {
		t.Errorf("configured defaults were not applied: %+v", seen)
	}
	if !strings.Contains(seen.Prompt, "추진 배경") {
		t.Errorf("the brief did not reach the prompt: %q", seen.Prompt)
	}
}

func TestClientRefusesWhenNotConfigured(t *testing.T) {
	client := NewClient(Config{Enabled: false})
	_, err := client.Get(context.Background(), "pres-1")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClientSurfacesTheReasonForARejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(400)
		io.WriteString(w, `{"error":{"code":"invalid_prompt","message":"prompt는 12000자를 넘을 수 없습니다"}}`)
	}))
	defer server.Close()

	_, err := NewClient(testConfig(server.URL)).Get(context.Background(), "pres-1")
	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("unexpected error type: %v", err)
	}
	if !strings.Contains(err.Error(), "12000자") {
		t.Fatalf("the reason was lost: %v", err)
	}
	if apiError.Retryable() {
		t.Error("a rejected request should not be retried")
	}
	if HTTPStatus(err) != http.StatusBadRequest {
		t.Errorf("status = %d", HTTPStatus(err))
	}
}

func TestClientReportsAnOutageAsRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(503)
		io.WriteString(w, `{"message":"generation queue unavailable"}`)
	}))
	defer server.Close()

	_, err := NewClient(testConfig(server.URL)).Get(context.Background(), "pres-1")
	var apiError *APIError
	if !errors.As(err, &apiError) || !apiError.Retryable() {
		t.Fatalf("a 503 should be retryable: %v", err)
	}
	if HTTPStatus(err) != http.StatusBadGateway {
		t.Errorf("status = %d", HTTPStatus(err))
	}
}

func TestExportStreamsTheFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("format") != "pptx" {
			t.Errorf("format = %q", r.URL.Query().Get("format"))
		}
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.presentationml.presentation")
		io.WriteString(w, "PK-fake-pptx")
	}))
	defer server.Close()

	body, contentType, err := NewClient(testConfig(server.URL)).Export(context.Background(), "pres-1", "pptx")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	payload, _ := io.ReadAll(body)
	if string(payload) != "PK-fake-pptx" {
		t.Fatalf("payload = %q", payload)
	}
	if !strings.Contains(contentType, "presentationml") {
		t.Errorf("content type = %q", contentType)
	}
}

func TestPresentationTerminal(t *testing.T) {
	for status, want := range map[string]bool{
		"draft": false, "queued": false, "generating": false,
		"completed": true, "failed": true,
	} {
		if got := (Presentation{Status: status}).Terminal(); got != want {
			t.Errorf("%s: Terminal() = %v", status, got)
		}
	}
}
