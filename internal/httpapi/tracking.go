package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/hkjang/muni/internal/cryptoutil"
	"github.com/hkjang/muni/internal/tracking"
)

// pagePolicy is the policy every page has always been served with. The
// tracking snippet adds to script-src, connect-src and img-src and to nothing
// else; switching tracking off returns to exactly this string.
const pagePolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self' ws: wss:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"

// apiPolicy is for what is not a page: JSON, the MCP endpoint, the health
// checks, the metrics scrape and the collector proxy. Nothing there runs
// scripts, so nothing is allowed to.
const apiPolicy = "default-src 'none'; frame-ancestors 'none'"

// cspReportPath is where browsers post what the policy refused. It is
// unauthenticated because the browser sends the report without credentials,
// and it stores nothing but a bounded list of origins in memory.
const cspReportPath = "/api/v1/tracking/csp-report"

// maxReportBytes keeps an unauthenticated endpoint from being handed large
// bodies.
const maxReportBytes = 8 * 1024

// isPagePath reports whether a path serves the web application rather than
// data. Only pages carry the snippet and the widened policy.
func isPagePath(path string) bool {
	for _, prefix := range []string{"/api/", "/mcp", "/momento/", "/.well-known/"} {
		if strings.HasPrefix(path, prefix) {
			return false
		}
	}
	switch path {
	case "/healthz", "/readyz", "/metrics", "/momento":
		return false
	}
	return true
}

// policyFor keeps the strict page policy and adds only what the configured
// snippet needs, including the nonce for its inline code. While tracking is
// on the browser is asked to report what it refused; that report is what
// turns a console error into a one-click fix.
func policyFor(config tracking.Config, path, nonce string) string {
	if !isPagePath(path) {
		return apiPolicy
	}
	if !config.Active(path) {
		return pagePolicy
	}
	scripts := []string{"'self'", "'nonce-" + nonce + "'"}
	connects := []string{"'self'", "ws:", "wss:"}
	images := []string{"'self'", "data:", "blob:"}
	extraScripts, extraConnects, extraImages := config.PolicySources()
	scripts = append(scripts, extraScripts...)
	connects = append(connects, extraConnects...)
	images = append(images, extraImages...)
	return "default-src 'self'; script-src " + strings.Join(scripts, " ") +
		"; style-src 'self' 'unsafe-inline'; img-src " + strings.Join(images, " ") +
		"; font-src 'self' data:; connect-src " + strings.Join(connects, " ") +
		"; frame-ancestors 'none'; base-uri 'self'; form-action 'self'; report-uri " + cspReportPath
}

// injectSnippet places the markup just before the closing tag it belongs to,
// or at the end when the page has no such tag.
func injectSnippet(page []byte, snippet, placement string) []byte {
	marker := "</head>"
	if placement == "body" {
		marker = "</body>"
	}
	text := string(page)
	index := strings.LastIndex(strings.ToLower(text), marker)
	if index < 0 {
		return []byte(text + "\n" + snippet + "\n")
	}
	return []byte(text[:index] + snippet + "\n" + text[index:])
}

// trackingConfig reads the tracking setting. A failure is "no tracking", so
// a settings outage never keeps the page from loading.
func (s *Server) trackingConfig(ctx context.Context) tracking.Config {
	if s.settings == nil {
		return tracking.Config{}
	}
	all, err := s.settings.GetAll(ctx, false)
	if err != nil {
		return tracking.Config{}
	}
	return all.Tracking
}

// servePage writes the application shell for one request: a fresh nonce, the
// policy that names it, and the snippet carrying it. The header and the markup
// are made together so they cannot disagree.
func (s *Server) servePage(w http.ResponseWriter, r *http.Request, page []byte, config tracking.Config) {
	if config.Active(r.URL.Path) {
		nonce, err := cryptoutil.RandomToken(16)
		if err == nil {
			w.Header().Set("Content-Security-Policy", policyFor(config, r.URL.Path, nonce))
			page = injectSnippet(page, config.Snippet(nonce), config.Placement)
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(page)
}

type cspReport struct {
	Report struct {
		BlockedURI         string `json:"blocked-uri"`
		ViolatedDirective  string `json:"violated-directive"`
		EffectiveDirective string `json:"effective-directive"`
		DocumentURI        string `json:"document-uri"`
	} `json:"csp-report"`
}

// receiveCSPReport records what a browser refused to load. Reports are always
// answered with 204: a page that misbehaves never sees an error from here.
func (s *Server) receiveCSPReport(w http.ResponseWriter, r *http.Request) {
	defer w.WriteHeader(http.StatusNoContent)
	if s.violations == nil {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxReportBytes))
	if err != nil || len(body) == 0 {
		return
	}
	var report cspReport
	if json.Unmarshal(body, &report) != nil {
		return
	}
	directive := report.Report.EffectiveDirective
	if directive == "" {
		directive = report.Report.ViolatedDirective
	}
	s.violations.Record(report.Report.BlockedURI, directive, report.Report.DocumentURI)
}

// listTrackingViolations shows the administrator which addresses the policy
// is blocking, so a snippet can be fixed without reading the browser console.
func (s *Server) listTrackingViolations(w http.ResponseWriter, r *http.Request) {
	items := []tracking.Violation{}
	if s.violations != nil {
		items = s.violations.List(s.trackingConfig(r.Context()))
	}
	writeData(w, http.StatusOK, items)
}

// clearTrackingViolations forgets the recorded reports, which is how an
// administrator checks whether a change actually fixed the snippet.
func (s *Server) clearTrackingViolations(w http.ResponseWriter, _ *http.Request) {
	if s.violations != nil {
		s.violations.Forget()
	}
	w.WriteHeader(http.StatusNoContent)
}

// allowTrackingHost adds one blocked origin to the allow list — the one-click
// fix for the reports listed above. Only that one setting is written, so the
// click never carries along an unsaved form.
func (s *Server) allowTrackingHost(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Origin string `json:"origin"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	origin := tracking.OriginOf(input.Origin)
	if origin == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ORIGIN", "허용할 주소가 올바르지 않습니다. http 또는 https 주소여야 합니다.")
		return
	}
	p, _ := principalFrom(r.Context())
	current := s.trackingConfig(r.Context()).AllowedHosts
	updated := tracking.AddAllowedHost(current, origin)
	if err := s.settings.PutValue(r.Context(), "tracking.allowed_hosts", updated, p.User.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "SETTINGS_ERROR", "설정을 저장하지 못했습니다.")
		return
	}
	s.audit(r, &p.User.ID, "UPDATE_SETTINGS", "SETTINGS", nil, map[string]any{"categories": []string{"tracking"}, "allowedHost": origin})
	writeData(w, http.StatusOK, map[string]string{"allowedHosts": updated})
}

// momentoTransport is shared so the connections to the collector are pooled
// rather than opened afresh for every event.
var momentoTransport = &http.Transport{Proxy: http.ProxyFromEnvironment, ResponseHeaderTimeout: 15 * time.Second, MaxIdleConnsPerHost: 8, IdleConnTimeout: 90 * time.Second}

// momentoProxy forwards /momento/* to the collector so the tracker is a
// same-origin file and the events a same-origin request: no outside origin
// has to be named in the policy. It answers only while a Momento collector
// is configured and the proxy chosen — there is no open relay here.
func (s *Server) momentoProxy(w http.ResponseWriter, r *http.Request) {
	s.forwardToMomento(w, r, s.trackingConfig(r.Context()))
}

func (s *Server) forwardToMomento(w http.ResponseWriter, r *http.Request, config tracking.Config) {
	if !config.ProxyActive() {
		http.NotFound(w, r)
		return
	}
	target, err := url.Parse(config.Normalize().MomentoURL)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(target)
			request.SetXForwarded()
			request.Out.Host = target.Host
			// The visitor's session is muni's business, not the collector's.
			request.Out.Header.Del("Cookie")
			request.Out.Header.Del("Authorization")
		},
		Transport: momentoTransport,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			w.WriteHeader(http.StatusBadGateway)
		},
	}
	http.StripPrefix(tracking.ProxyPath, proxy).ServeHTTP(w, r)
}
