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
	if link := config.EditorURL("abc"); link != "https://ptium.example.com/presentations/abc/editor" {
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
		// Ptium wraps every response in {"data": …}; a client that reads the
		// bare object silently produces an empty presentation.
		io.WriteString(w, `{"data":{"id":"pres-1","title":"사업 추진 계획","status":"queued","version":3},"requestId":"req-1"}`)
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
	if presentation.Version != 3 {
		t.Errorf("the version used to guard writes was lost: %+v", presentation)
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

func TestSourceAndReviseUnwrapTheEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/source") && r.Method == http.MethodGet:
			io.WriteString(w, `{"data":{"source":"# 표지\n@cover\n","slideCount":1},"requestId":"r"}`)
		case strings.HasSuffix(r.URL.Path, "/revise"):
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "instruction") {
				t.Errorf("the instruction was not sent: %s", body)
			}
			io.WriteString(w, `{"data":{"slide":2,"source":"# 추진 배경\n- 51개 시스템\n"},"requestId":"r"}`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()

	client := NewClient(testConfig(server.URL))
	source, err := client.Source(context.Background(), "pres-1")
	if err != nil || !strings.Contains(source, "@cover") {
		t.Fatalf("source = %q, err = %v", source, err)
	}
	revised, err := client.ReviseSlide(context.Background(), "pres-1", 2, "바뀐 내용 반영")
	if err != nil || !strings.Contains(revised, "51개") {
		t.Fatalf("revised = %q, err = %v", revised, err)
	}
}

func TestApplySourceGuardsAgainstAConcurrentEdit(t *testing.T) {
	var sent map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		// A fresh map per request: unmarshalling into the previous one keeps
		// keys the new request never sent.
		sent = map[string]any{}
		_ = json.Unmarshal(body, &sent)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":{},"requestId":"r"}`)
	}))
	defer server.Close()

	client := NewClient(testConfig(server.URL))
	if err := client.ApplySource(context.Background(), "pres-1", "# 표지\n", 7, false); err != nil {
		t.Fatal(err)
	}
	if sent["version"] != float64(7) {
		t.Errorf("the version was not sent: %+v", sent)
	}
	// A dry run must not claim a version, or Ptium would reject the preview.
	if err := client.ApplySource(context.Background(), "pres-1", "# 표지\n", 7, true); err != nil {
		t.Fatal(err)
	}
	if _, ok := sent["version"]; ok {
		t.Errorf("a dry run should not send a version: %+v", sent)
	}
	if sent["dryRun"] != true {
		t.Errorf("dryRun was not sent: %+v", sent)
	}
}

// Someone editing the deck in Ptium first is a conflict a retry resolves, not
// an outage the person should be told to report.
func TestAConcurrentDeckEditIsReportedAsAConflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(409)
		io.WriteString(w, `{"error":{"message":"이 발표자료가 그사이 수정되었습니다"}}`)
	}))
	defer server.Close()

	err := NewClient(testConfig(server.URL)).ApplySource(context.Background(), "pres-1", "# 표지\n", 3, false)
	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("unexpected error: %v", err)
	}
	if HTTPStatus(err) != http.StatusConflict {
		t.Errorf("status = %d, want 409", HTTPStatus(err))
	}
	if !apiError.Retryable() {
		t.Error("a conflict is worth retrying")
	}
	if !strings.Contains(err.Error(), "그사이 수정") {
		t.Errorf("the reason was lost: %v", err)
	}
}
