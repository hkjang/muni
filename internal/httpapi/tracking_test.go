package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/tracking"
)

func momentoConfig() tracking.Config {
	return tracking.Config{Enabled: true, Provider: tracking.ProviderMomento, MomentoURL: "https://momento.corp.example", MomentoSiteID: "muni", MomentoProxy: true}
}

const shell = "<!doctype html><html><head><title>muni</title></head><body><div id=\"root\"></div></body></html>"

// A fresh install changes nothing: the page policy is the string it always
// was, byte for byte, and the page has no snippet.
func TestOffLeavesThePolicyAndThePageAlone(t *testing.T) {
	if got := policyFor(tracking.Config{}, "/docs/1", "n"); got != pagePolicy {
		t.Fatalf("policy %q", got)
	}
	if strings.Contains(pagePolicy, "unsafe-inline'; img") == false || strings.Contains(pagePolicy, "script-src 'self';") == false {
		t.Fatalf("the page policy should be the historical one: %q", pagePolicy)
	}
	if strings.Contains(pagePolicy, "report-uri") || strings.Contains(pagePolicy, "nonce") {
		t.Fatal("nothing tracking-related while off")
	}
	server := &Server{}
	recorder := httptest.NewRecorder()
	server.servePage(recorder, httptest.NewRequest(http.MethodGet, "/docs/1", nil), []byte(shell), tracking.Config{})
	if recorder.Body.String() != shell || recorder.Header().Get("Content-Security-Policy") != "" {
		t.Fatalf("body %q header %q", recorder.Body.String(), recorder.Header().Get("Content-Security-Policy"))
	}
}

func TestOnPutsTheNonceInBothThePolicyAndTheSnippet(t *testing.T) {
	config := momentoConfig()
	config.MomentoProxy = false
	server := &Server{}
	for _, placement := range []string{"head", "body"} {
		config.Placement = placement
		recorder := httptest.NewRecorder()
		server.servePage(recorder, httptest.NewRequest(http.MethodGet, "/docs/1", nil), []byte(shell), config)
		body := recorder.Body.String()
		policy := recorder.Header().Get("Content-Security-Policy")
		start := strings.Index(body, `nonce="`) + len(`nonce="`)
		if start < len(`nonce="`) {
			t.Fatalf("no nonce in %q", body)
		}
		nonce := body[start : start+strings.Index(body[start:], `"`)]
		if len(nonce) < 16 {
			t.Fatalf("nonce %q is too short", nonce)
		}
		if !strings.Contains(policy, "script-src 'self' 'nonce-"+nonce+"' https://momento.corp.example") {
			t.Fatalf("policy %q does not name the nonce %q and the collector", policy, nonce)
		}
		if !strings.Contains(policy, "connect-src 'self' ws: wss: https://momento.corp.example") || !strings.Contains(policy, "img-src 'self' data: blob: https://momento.corp.example") {
			t.Fatalf("policy %q", policy)
		}
		if !strings.HasSuffix(policy, "; report-uri "+cspReportPath) || strings.Contains(policy, "unsafe-inline'; img-src") == false {
			t.Fatalf("policy %q", policy)
		}
		if strings.Contains(strings.Replace(policy, "style-src 'self' 'unsafe-inline'", "", 1), "unsafe-inline") {
			t.Fatalf("script-src must never be loosened with unsafe-inline: %q", policy)
		}
		marker := "</head>"
		if placement == "body" {
			marker = "</body>"
		}
		if !strings.Contains(body, `<script nonce="`+nonce+`" async src="https://momento.corp.example/tracker.js"`) {
			t.Fatalf("body %q", body)
		}
		if at := strings.Index(body, "<script"); at > strings.Index(body, marker) || (placement == "body" && at < strings.Index(body, "</head>")) {
			t.Fatalf("placement %s put the snippet at %d in %q", placement, at, body)
		}
	}
}

func TestThroughTheProxyNoOutsideOriginEntersThePolicy(t *testing.T) {
	policy := policyFor(momentoConfig(), "/", "abc")
	if strings.Contains(policy, "momento.corp.example") {
		t.Fatalf("policy %q", policy)
	}
	if !strings.Contains(policy, "script-src 'self' 'nonce-abc';") {
		t.Fatalf("policy %q", policy)
	}
}

func TestNonPagePathsAreNarrowAndCarryNothing(t *testing.T) {
	config := momentoConfig()
	config.IncludeAdmin = true
	for _, path := range []string{"/api/v1/documents", "/api/openapi.yaml", "/mcp", "/healthz", "/readyz", "/metrics", "/momento/tracker.js", "/.well-known/oauth-protected-resource"} {
		if got := policyFor(config, path, "n"); got != apiPolicy {
			t.Fatalf("%s: %q", path, got)
		}
		if config.Active(path) && isPagePath(path) {
			t.Fatalf("%s should not carry the snippet", path)
		}
	}
	// The middleware narrows those paths before any handler runs.
	server := &Server{mux: http.NewServeMux()}
	server.mux.HandleFunc("/api/v1/x", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	recorder := httptest.NewRecorder()
	server.securityHeaders(server.mux).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/x", nil))
	if got := recorder.Header().Get("Content-Security-Policy"); got != apiPolicy {
		t.Fatalf("api header %q", got)
	}
	recorder = httptest.NewRecorder()
	server.securityHeaders(server.mux).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if got := recorder.Header().Get("Content-Security-Policy"); got != pagePolicy {
		t.Fatalf("asset header %q", got)
	}
}

func TestTheConsoleIsLeftOutUnlessAsked(t *testing.T) {
	config := momentoConfig()
	if got := policyFor(config, "/admin/settings", "n"); got != pagePolicy {
		t.Fatalf("admin policy %q", got)
	}
	server := &Server{}
	recorder := httptest.NewRecorder()
	server.servePage(recorder, httptest.NewRequest(http.MethodGet, "/admin/settings", nil), []byte(shell), config)
	if strings.Contains(recorder.Body.String(), "<script") {
		t.Fatal("no snippet on the console by default")
	}
	config.IncludeAdmin = true
	recorder = httptest.NewRecorder()
	server.servePage(recorder, httptest.NewRequest(http.MethodGet, "/admin/settings", nil), []byte(shell), config)
	if !strings.Contains(recorder.Body.String(), "<script") {
		t.Fatal("include_admin adds the console")
	}
}

func TestReportsAreRecordedAndAlwaysAnswered204(t *testing.T) {
	server := &Server{violations: tracking.NewRecorder()}
	body := `{"csp-report":{"blocked-uri":"https://momento.corp.example/collect/v1/events","effective-directive":"connect-src","document-uri":"https://muni.corp.example/docs/1"}}`
	for range 3 {
		recorder := httptest.NewRecorder()
		server.receiveCSPReport(recorder, httptest.NewRequest(http.MethodPost, cspReportPath, strings.NewReader(body)))
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("status %d", recorder.Code)
		}
	}
	items := server.violations.List(tracking.Config{})
	if len(items) != 1 || items[0].Origin != "https://momento.corp.example" || items[0].Directive != "connect-src" || items[0].Count != 3 || items[0].Page != "https://muni.corp.example/docs/1" {
		t.Fatalf("items %+v", items)
	}
	for _, broken := range []string{"", "not json", `{"csp-report":{"blocked-uri":"inline"}}`, strings.Repeat("x", maxReportBytes*2)} {
		recorder := httptest.NewRecorder()
		server.receiveCSPReport(recorder, httptest.NewRequest(http.MethodPost, cspReportPath, strings.NewReader(broken)))
		if recorder.Code != http.StatusNoContent || len(server.violations.List(tracking.Config{})) != 1 {
			t.Fatalf("%q: status %d items %d", broken, recorder.Code, len(server.violations.List(tracking.Config{})))
		}
	}
	// The one-click fix marks it allowed without anybody clearing the list.
	fixed := momentoConfig()
	fixed.MomentoProxy = false
	if !server.violations.List(fixed)[0].Allowed {
		t.Fatal("allowed")
	}
}

func TestTheReportRouteIsOpenAndTheAdminRoutesAreNot(t *testing.T) {
	patterns := strings.Join(routePatterns(), "\n")
	for _, want := range []string{"POST " + cspReportPath, "GET /api/v1/admin/tracking/violations", "DELETE /api/v1/admin/tracking/violations", "POST /api/v1/admin/tracking/allow", tracking.ProxyPath + "/"} {
		if !strings.Contains(patterns, want) {
			t.Fatalf("route %s is not registered", want)
		}
	}
}

func TestTheProxyForwardsWithoutTheSessionAndOnlyWhenChosen(t *testing.T) {
	var seen *http.Request
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte("tracker"))
	}))
	defer collector.Close()

	config := tracking.Config{Enabled: true, Provider: tracking.ProviderMomento, MomentoURL: collector.URL + "/base/", MomentoSiteID: "muni", MomentoProxy: true}
	server := &Server{}
	request := httptest.NewRequest(http.MethodGet, "/momento/tracker.js?v=1", nil)
	request.Header.Set("Cookie", sessionCookie+"=secret")
	request.Header.Set("Authorization", "Bearer muni_key")
	request.RemoteAddr = "10.1.2.3:4444"
	recorder := httptest.NewRecorder()
	server.forwardToMomento(recorder, request, config)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "tracker" {
		t.Fatalf("status %d body %q", recorder.Code, recorder.Body.String())
	}
	if seen == nil || seen.URL.Path != "/base/tracker.js" || seen.URL.RawQuery != "v=1" {
		t.Fatalf("collector saw %v", seen)
	}
	if seen.Header.Get("Cookie") != "" || seen.Header.Get("Authorization") != "" {
		t.Fatalf("the session must not reach the collector: %v", seen.Header)
	}
	if seen.Header.Get("X-Forwarded-For") != "10.1.2.3" {
		t.Fatalf("the visitor's address is what the collector counts: %v", seen.Header)
	}

	for _, off := range []tracking.Config{{}, func() tracking.Config { c := config; c.MomentoProxy = false; return c }(), func() tracking.Config { c := config; c.Enabled = false; return c }()} {
		recorder := httptest.NewRecorder()
		server.forwardToMomento(recorder, httptest.NewRequest(http.MethodGet, "/momento/tracker.js", nil), off)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%+v should not relay: %d", off, recorder.Code)
		}
	}
}
