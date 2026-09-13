package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

// Handing a document to another service, checked the way the standard lists
// it: the claim is single-use and short-lived, nobody gets a claim on a
// document they cannot read, an unlisted source is refused without a request,
// an oversized body is cut off, a redirect is not followed, and the document
// that arrives says where it came from.

// setPeers replaces the administrator's peer list and restores the original
// form when the test ends.
func setPeers(t *testing.T, srv *serverUnderTest, peers []map[string]any) {
	t.Helper()
	resp, err := srv.admin.Get(srv.URL + "/api/v1/admin/settings")
	if err != nil {
		t.Fatal(err)
	}
	var current struct {
		Data map[string]any `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&current)
	resp.Body.Close()
	original, _ := json.Marshal(current.Data)
	t.Cleanup(func() { putSettings(t, srv, original) })
	current.Data["handoff"] = map[string]any{"peers": peers}
	body, _ := json.Marshal(current.Data)
	putSettings(t, srv, body)
}

// markdownDocumentOwnedByAdmin makes a document with a body worth exporting.
func markdownDocumentOwnedByAdmin(t *testing.T, srv *serverUnderTest, title string) uuid.UUID {
	t.Helper()
	id := ownedDocument(t, srv, title)
	content := `{"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"개편안"}]},{"type":"paragraph","content":[{"type":"text","text":"첫 문단입니다."}]}]}`
	if _, err := srv.db.Exec(context.Background(), `UPDATE documents SET content_json=$2 WHERE id=$1`, id, content); err != nil {
		t.Fatal(err)
	}
	return id
}

type issuedClaim struct {
	Claim       string `json:"claim"`
	Source      string `json:"source"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Bytes       int    `json:"bytes"`
	ExpiresAt   string `json:"expires_at"`
}

func issueClaim(t *testing.T, client *http.Client, base string, documentID uuid.UUID, format string) (int, issuedClaim) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"resource": documentID.String(), "format": format})
	resp, err := client.Post(base+"/api/v1/handoff/claims", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var claim issuedClaim
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &claim)
	return resp.StatusCode, claim
}

func TestAClaimIsIssuedOnceAndRedeemedOnce(t *testing.T) {
	srv := newServerUnderTest(t)
	documentID := markdownDocumentOwnedByAdmin(t, srv, "2026년 3분기 개편안")

	status, claim := issueClaim(t, srv.admin, srv.URL, documentID, "markdown")
	if status != http.StatusCreated {
		t.Fatalf("issue = %d %+v", status, claim)
	}
	if len(claim.Claim) < 32 || claim.Source != srv.URL || claim.Filename != "2026년 3분기 개편안.md" ||
		!strings.HasPrefix(claim.ContentType, "text/markdown") || claim.Bytes == 0 || claim.ExpiresAt == "" {
		t.Fatalf("claim = %+v", claim)
	}

	// Redeeming needs no session at all.
	anonymous := &http.Client{}
	resp, err := anonymous.Get(srv.URL + "/api/v1/handoff/claims/" + claim.Claim)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("redeem = %d %s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/markdown") {
		t.Errorf("Content-Type = %q", got)
	}
	if got := resp.Header.Get("Content-Disposition"); !strings.Contains(got, "filename*=UTF-8''") || !strings.Contains(got, "attachment") {
		t.Errorf("Content-Disposition = %q", got)
	}
	// The receiving service reads the name back with a header parser, so
	// the Korean title must be encoded the way one expects.
	if _, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition")); err != nil || params["filename"] != "2026년 3분기 개편안.md" {
		t.Errorf("filename read back = %q (%v) from %q", params["filename"], err, resp.Header.Get("Content-Disposition"))
	}
	if len(body) != claim.Bytes || !strings.Contains(string(body), "개편안") || !strings.Contains(string(body), "첫 문단입니다.") {
		t.Errorf("body (%d bytes) = %q", len(body), body)
	}

	// The second request finds nothing — the claim was consumed by the first.
	again, err := anonymous.Get(srv.URL + "/api/v1/handoff/claims/" + claim.Claim)
	if err != nil {
		t.Fatal(err)
	}
	again.Body.Close()
	if again.StatusCode != http.StatusNotFound {
		t.Errorf("second redeem = %d, want 404", again.StatusCode)
	}

	// An expired claim is the same 404, and is swept when the next is issued.
	_, expired := issueClaim(t, srv.admin, srv.URL, documentID, "markdown")
	if _, err := srv.db.Exec(context.Background(), `UPDATE handoff_claims SET expires_at=now()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	late, err := anonymous.Get(srv.URL + "/api/v1/handoff/claims/" + expired.Claim)
	if err != nil {
		t.Fatal(err)
	}
	late.Body.Close()
	if late.StatusCode != http.StatusNotFound {
		t.Errorf("expired redeem = %d, want 404", late.StatusCode)
	}
	issueClaim(t, srv.admin, srv.URL, documentID, "markdown")
	var stale int
	_ = srv.db.QueryRow(context.Background(), `SELECT count(*) FROM handoff_claims WHERE expires_at<now()`).Scan(&stale)
	if stale != 0 {
		t.Errorf("%d expired claims survived the next issue", stale)
	}

	// The claim itself is in no audit row.
	var leaked int
	_ = srv.db.QueryRow(context.Background(), `SELECT count(*) FROM activity_logs WHERE metadata::text LIKE '%'||$1||'%'`, claim.Claim).Scan(&leaked)
	if leaked != 0 {
		t.Error("the claim was written into the audit log")
	}
	var served int
	_ = srv.db.QueryRow(context.Background(), `SELECT count(*) FROM activity_logs WHERE action='HANDOFF_SERVE' AND resource_id=$1`, documentID).Scan(&served)
	if served != 1 {
		t.Errorf("HANDOFF_SERVE rows = %d, want 1", served)
	}
}

func TestNoClaimOnSomebodyElsesDocument(t *testing.T) {
	srv := newServerUnderTest(t)
	documentID := markdownDocumentOwnedByAdmin(t, srv, "남의 문서")
	outsider := createAccount(t, srv, "handoff-outsider@example.com", "남")
	if _, err := srv.db.Exec(context.Background(),
		`UPDATE users SET password_reset_required=false, password_hash=$2 WHERE id=$1`,
		outsider, mustHash(t, "외부인비밀번호입니다")); err != nil {
		t.Fatal(err)
	}
	client := signIn(t, srv.Server, "handoff-outsider@example.com", "외부인비밀번호입니다")
	status, claim := issueClaim(t, client, srv.URL, documentID, "markdown")
	if status != http.StatusNotFound || claim.Claim != "" {
		t.Fatalf("outsider's claim = %d %+v", status, claim)
	}
	// Nor without a session, nor in a format muni does not send.
	if status, _ := issueClaim(t, &http.Client{}, srv.URL, documentID, "markdown"); status != http.StatusUnauthorized {
		t.Errorf("anonymous claim = %d", status)
	}
	if status, _ := issueClaim(t, srv.admin, srv.URL, documentID, "pptx"); status != http.StatusBadRequest {
		t.Errorf("pptx claim = %d", status)
	}
	var rows int
	_ = srv.db.QueryRow(context.Background(), `SELECT count(*) FROM handoff_claims WHERE document_id=$1`, documentID).Scan(&rows)
	if rows != 0 {
		t.Errorf("%d claims were stored for refused requests", rows)
	}
}

func TestReceivingTakesOnlyFromAListedSource(t *testing.T) {
	srv := newServerUnderTest(t)
	// What an earlier run left behind must not be mistaken for this one's.
	if _, err := srv.db.Exec(context.Background(), `DELETE FROM documents WHERE title LIKE '캔버스%'`); err != nil {
		t.Fatal(err)
	}
	var asked atomic.Int32
	// The peer: what a listed service answers, and what an unlisted one
	// would answer if it were ever asked.
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		claim := path.Base(r.URL.Path)
		switch {
		case strings.HasPrefix(claim, "redirect-me"):
			http.Redirect(w, r, "/api/v1/handoff/claims/elsewhere", http.StatusFound)
		case strings.HasPrefix(claim, "too-big-to-take"):
			w.Header().Set("Content-Type", "text/markdown")
			w.(http.Flusher).Flush()
			chunk := []byte(strings.Repeat("a", 1<<20))
			for i := 0; i < 26; i++ {
				if _, err := w.Write(chunk); err != nil {
					return
				}
			}
		case strings.HasPrefix(claim, "not-markdown-at-all"):
			w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.presentationml.presentation")
			_, _ = w.Write([]byte("PK"))
		case strings.HasPrefix(claim, "gone-already-used"):
			http.NotFound(w, r)
		default:
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename*=UTF-8''%EC%BA%94%EB%B2%84%EC%8A%A4%20%EC%83%9D%EA%B0%81.md`)
			_, _ = w.Write([]byte("# 캔버스에서 온 생각\n\n첫 문단.\n\n- 하나\n- 둘\n"))
		}
	}))
	defer peer.Close()
	unlisted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(100)
	}))
	defer unlisted.Close()

	// Before any peer is listed nothing is offered and nothing is taken.
	if targets := capabilityTargets(t, srv); len(targets) != 0 {
		t.Fatalf("targets before any peer = %v", targets)
	}
	page := receive(t, srv.admin, srv.URL, peer.URL, "abcdefghijklmnopqrstuvwxyz")
	if page.status != http.StatusForbidden || asked.Load() != 0 {
		t.Fatalf("unlisted source = %d, asked %d times", page.status, asked.Load())
	}

	setPeers(t, srv, []map[string]any{{"origin": peer.URL, "name": "umm", "receives": []string{"markdown"}}})

	// Listed but still not this one: refused without a request.
	page = receive(t, srv.admin, srv.URL, unlisted.URL, "abcdefghijklmnopqrstuvwxyz")
	if page.status != http.StatusForbidden || asked.Load() != 0 {
		t.Fatalf("other unlisted source = %d, asked %d times", page.status, asked.Load())
	}
	if !strings.Contains(page.body, "허용") {
		t.Errorf("page says nothing about the allow list: %s", page.body)
	}
	// A source that is the listed host with a path is not the listed origin.
	page = receive(t, srv.admin, srv.URL, peer.URL+"/api", "abcdefghijklmnopqrstuvwxyz")
	if page.status != http.StatusForbidden || asked.Load() != 0 {
		t.Fatalf("source with a path = %d, asked %d times", page.status, asked.Load())
	}

	// Each way the peer can misbehave is a page, not a document.
	for claim, want := range map[string]int{
		"redirect-me-000000000": http.StatusBadGateway,
		"too-big-to-take-00000": http.StatusRequestEntityTooLarge,
		"not-markdown-at-all-0": http.StatusUnsupportedMediaType,
		"gone-already-used-000": http.StatusNotFound,
	} {
		page = receive(t, srv.admin, srv.URL, peer.URL, claim)
		if page.status != want {
			t.Errorf("%s = %d, want %d: %s", claim, page.status, want, page.body)
		}
	}
	if asked.Load() != 4 {
		t.Errorf("peer asked %d times, want 4", asked.Load())
	}
	var arrived int
	_ = srv.db.QueryRow(context.Background(), `SELECT count(*) FROM documents WHERE title LIKE '캔버스%'`).Scan(&arrived)
	if arrived != 0 {
		t.Fatalf("%d documents arrived from failed handoffs", arrived)
	}

	// The good claim becomes a document in the person's own workspace, and
	// the browser is sent to it.
	page = receive(t, srv.admin, srv.URL, peer.URL, "good-claim-abcdefghijk")
	if page.status != http.StatusFound || !strings.HasPrefix(page.location, "/docs/") {
		t.Fatalf("good claim = %d → %q: %s", page.status, page.location, page.body)
	}
	documentID, err := uuid.Parse(strings.TrimPrefix(page.location, "/docs/"))
	if err != nil {
		t.Fatal(err)
	}
	var title, kind, reason string
	err = srv.db.QueryRow(context.Background(), `SELECT d.title,w.kind,r.reason FROM documents d JOIN workspaces w ON w.id=d.workspace_id JOIN document_revisions r ON r.document_id=d.id AND r.revision_no=1 WHERE d.id=$1`, documentID).Scan(&title, &kind, &reason)
	if err != nil {
		t.Fatal(err)
	}
	if title != "캔버스 생각" || kind != "PERSONAL" || reason != "handoff:"+strings.ToLower(peer.URL) {
		t.Errorf("title=%q kind=%q reason=%q", title, kind, reason)
	}
	var source string
	_ = srv.db.QueryRow(context.Background(), `SELECT metadata->>'source' FROM activity_logs WHERE action='HANDOFF_RECEIVE' AND resource_id=$1`, documentID).Scan(&source)
	if source != strings.ToLower(peer.URL) {
		t.Errorf("audit source = %q", source)
	}
	resp, err := srv.admin.Get(srv.URL + "/api/v1/documents/" + documentID.String() + "/export/md")
	if err != nil {
		t.Fatal(err)
	}
	exported, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(exported), "첫 문단.") || !strings.Contains(string(exported), "- 둘") {
		t.Errorf("the received document lost its body: %s", exported)
	}

	// Now that a peer is listed which receives markdown, the menu offers it.
	targets := capabilityTargets(t, srv)
	if len(targets) != 1 || targets[0]["name"] != "umm" {
		t.Errorf("targets = %v", targets)
	}
}

func TestReceivingWithoutASessionGoesThroughLogin(t *testing.T) {
	srv := newServerUnderTest(t)
	page := receive(t, noRedirects(), srv.URL, "https://umm.intra", "abcdefghijklmnopqrstuvwxyz")
	if page.status != http.StatusFound {
		t.Fatalf("anonymous = %d", page.status)
	}
	location, err := url.Parse(page.location)
	if err != nil || location.Path != "/login" {
		t.Fatalf("location = %q", page.location)
	}
	returnTo := location.Query().Get("return_to")
	if !strings.HasPrefix(returnTo, "/handoff?") || !strings.Contains(returnTo, "claim=abcdefghijklmnopqrstuvwxyz") {
		t.Errorf("return_to = %q", returnTo)
	}
}

type receivedPage struct {
	status   int
	location string
	body     string
}

// receive opens /handoff the way a peer's button does, without following
// the redirect to the new document.
func receive(t *testing.T, client *http.Client, base, source, claim string) receivedPage {
	t.Helper()
	stay := *client
	stay.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := stay.Get(base + "/handoff?source=" + url.QueryEscape(source) + "&claim=" + url.QueryEscape(claim))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return receivedPage{status: resp.StatusCode, location: resp.Header.Get("Location"), body: string(body)}
}

func capabilityTargets(t *testing.T, srv *serverUnderTest) []map[string]any {
	t.Helper()
	resp, err := srv.admin.Get(srv.URL + "/api/v1/system/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Data struct {
			Targets []map[string]any `json:"handoffTargets"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Data.Targets
}
