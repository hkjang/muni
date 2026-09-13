package httpapi

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/handoff"
)

// A claim is a credential, and the endpoint that redeems it carries the claim
// in its path — the one place the request log would otherwise write it.
func TestLogPathHidesTheClaim(t *testing.T) {
	if got := logPath("/api/v1/handoff/claims/AbC123_-xyz"); got != "/api/v1/handoff/claims/{claim}" {
		t.Errorf("logPath = %q", got)
	}
	for _, path := range []string{"/api/v1/handoff/claims", "/handoff", "/api/v1/documents/1"} {
		if got := logPath(path); got != path {
			t.Errorf("logPath(%q) = %q", path, got)
		}
	}
}

// The refusal page names the source that was refused, and the source is a
// value from outside — so it is text on the page, never markup.
func TestHandoffPageEscapesWhatCameFromOutside(t *testing.T) {
	status, title, message := handoffFailure(handoff.ErrNotAllowed, `https://evil.example/<script>alert(1)</script>`)
	if status != 403 {
		t.Fatalf("status = %d", status)
	}
	recorder := httptest.NewRecorder()
	handoffPage(recorder, status, title, message)
	body := recorder.Body.String()
	if strings.Contains(body, "<script>") || !strings.Contains(body, "&lt;script&gt;") {
		t.Errorf("page = %s", body)
	}
	if !strings.Contains(recorder.Header().Get("Content-Type"), "text/html") {
		t.Errorf("Content-Type = %q", recorder.Header().Get("Content-Type"))
	}
	// Every failure has words of its own.
	seen := map[string]bool{}
	for _, err := range []error{handoff.ErrNotAllowed, handoff.ErrBadClaim, handoff.ErrNotFound, handoff.ErrRedirect, handoff.ErrUnsupported, handoff.ErrTooLarge, handoff.ErrUnreachable, errors.New("other")} {
		_, title, _ := handoffFailure(err, "")
		seen[title] = true
	}
	if len(seen) != 6 {
		t.Errorf("%d distinct titles for 8 errors (bad claim and not found share one, unreachable and other share one)", len(seen))
	}
}
