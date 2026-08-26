package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/database"
)

// A link endpoint answers people muni has never met, so these check what it
// refuses as carefully as what it serves.

func enableLinks(t *testing.T, srv *serverUnderTest, on bool) {
	t.Helper()
	// The settings endpoint replaces the whole document, so the current one is
	// read first — sending only the security group would blank the rest.
	resp, err := srv.admin.Get(srv.URL + "/api/v1/admin/settings")
	if err != nil {
		t.Fatal(err)
	}
	var current struct {
		Data map[string]any `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&current)
	resp.Body.Close()
	security, _ := current.Data["security"].(map[string]any)
	if security == nil {
		security = map[string]any{}
	}
	security["allowPublicLinks"] = on
	current.Data["security"] = security

	body, _ := json.Marshal(current.Data)
	req, _ := http.NewRequest("PUT", srv.URL+"/api/v1/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	saved, err := srv.admin.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(saved.Body)
	saved.Body.Close()
	if saved.StatusCode != 200 {
		t.Fatalf("settings = %d %s", saved.StatusCode, raw)
	}
}

// ownedDocument makes a document the bootstrap administrator owns.
func ownedDocument(t *testing.T, srv *serverUnderTest, title string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var adminID, workspaceID uuid.UUID
	if err := srv.db.QueryRow(ctx, `SELECT id FROM users WHERE email='admin@muni.local'`).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	if err := srv.db.QueryRow(ctx, `SELECT id FROM workspaces WHERE owner_id=$1 LIMIT 1`, adminID).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	if _, err := srv.db.Exec(ctx,
		`INSERT INTO documents(id,workspace_id,owner_id,title) VALUES($1,$2,$3,$4)`,
		id, workspaceID, adminID, title); err != nil {
		t.Fatal(err)
	}
	return id
}

func makeLink(t *testing.T, srv *serverUnderTest, documentID uuid.UUID, body map[string]any) (string, map[string]any) {
	t.Helper()
	status, data := postJSON(t, srv.admin, srv.URL+"/api/v1/documents/"+documentID.String()+"/links", body)
	if status != 201 {
		t.Fatalf("create link = %d %v", status, data)
	}
	token, _ := data["token"].(string)
	if token == "" {
		t.Fatalf("no token in %v", data)
	}
	return token, data
}

func openLink(t *testing.T, srv *serverUnderTest, token string, password string) (int, map[string]any) {
	t.Helper()
	body := map[string]any{}
	if password != "" {
		body["password"] = password
	}
	// A plain client: the point is that no session is involved.
	return postJSON(t, &http.Client{}, srv.URL+"/api/v1/public/documents/"+token, body)
}

func TestAnyoneWithTheLinkCanReadAndNothingElse(t *testing.T) {
	srv := newServerUnderTest(t)
	enableLinks(t, srv, true)
	documentID := ownedDocument(t, srv, "링크로 공유한 문서")
	token, created := makeLink(t, srv, documentID, map[string]any{"label": "고객사"})

	if created["path"] != "/s/"+token {
		t.Fatalf("path = %v", created["path"])
	}

	status, data := openLink(t, srv, token, "")
	if status != 200 {
		t.Fatalf("open = %d %v", status, data)
	}
	if data["title"] != "링크로 공유한 문서" {
		t.Fatalf("title = %v", data["title"])
	}
	// Nothing about the organisation goes out this door.
	for _, leaked := range []string{"workspace", "ownerName", "ownerId", "workspaceId", "email"} {
		if _, present := data[leaked]; present {
			t.Errorf("the public response exposes %q", leaked)
		}
	}
	if data["role"] != "VIEWER" {
		t.Fatalf("role = %v; a link is read-only", data["role"])
	}
}

func TestTheTokenIsNotStored(t *testing.T) {
	// Someone with a copy of the database must not end up with working links.
	srv := newServerUnderTest(t)
	enableLinks(t, srv, true)
	documentID := ownedDocument(t, srv, "문서")
	token, _ := makeLink(t, srv, documentID, map[string]any{})

	var found int
	if err := srv.db.QueryRow(context.Background(),
		`SELECT count(*) FROM document_links WHERE encode(token_hash,'hex') LIKE '%'||$1||'%' OR token_prefix = $1`,
		token).Scan(&found); err != nil {
		t.Fatal(err)
	}
	if found != 0 {
		t.Fatal("the full token is recoverable from the table")
	}
	// The owner's own listing must not hand it back either.
	resp, err := srv.admin.Get(srv.URL + "/api/v1/documents/" + documentID.String() + "/links")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(raw), token) {
		t.Fatal("the link listing returns the token")
	}
}

func TestAWrongTokenIsIndistinguishableFromAMissingOne(t *testing.T) {
	srv := newServerUnderTest(t)
	enableLinks(t, srv, true)
	documentID := ownedDocument(t, srv, "문서")
	token, _ := makeLink(t, srv, documentID, map[string]any{})

	// Right prefix, wrong remainder — must not reveal that the prefix existed.
	tampered := token[:linkTokenPrefixLength] + strings.Repeat("a", len(token)-linkTokenPrefixLength)
	wrongStatus, wrongData := openLink(t, srv, tampered, "")
	missingStatus, missingData := openLink(t, srv, strings.Repeat("z", len(token)), "")
	if wrongStatus != missingStatus || wrongData["_errorCode"] != missingData["_errorCode"] {
		t.Fatalf("tampered=%d/%v missing=%d/%v", wrongStatus, wrongData["_errorCode"], missingStatus, missingData["_errorCode"])
	}
	if wrongStatus != 404 {
		t.Fatalf("status = %d", wrongStatus)
	}
}

func TestAProtectedLinkAsksAndThenChecks(t *testing.T) {
	srv := newServerUnderTest(t)
	enableLinks(t, srv, true)
	documentID := ownedDocument(t, srv, "비밀번호 문서")
	token, created := makeLink(t, srv, documentID, map[string]any{"password": "1234"})
	if created["hasPassword"] != true {
		t.Fatal("hasPassword should be reported")
	}

	if status, data := openLink(t, srv, token, ""); status != 401 || data["_errorCode"] != "LINK_PASSWORD_REQUIRED" {
		t.Fatalf("no password = %d %v", status, data)
	}
	if status, data := openLink(t, srv, token, "9999"); status != 401 || data["_errorCode"] != "LINK_PASSWORD_INVALID" {
		t.Fatalf("wrong password = %d %v", status, data)
	}
	if status, data := openLink(t, srv, token, "1234"); status != 200 {
		t.Fatalf("right password = %d %v", status, data)
	}
}

func TestAViewLimitIsEnforcedByTheDatabaseNotTheCheck(t *testing.T) {
	// Two people opening a last-view link at the same moment must not both get
	// in, so the count and the ceiling move in one statement.
	srv := newServerUnderTest(t)
	enableLinks(t, srv, true)
	documentID := ownedDocument(t, srv, "한 번만")
	token, _ := makeLink(t, srv, documentID, map[string]any{"maxViews": 2})

	for i := 0; i < 2; i++ {
		if status, data := openLink(t, srv, token, ""); status != 200 {
			t.Fatalf("view %d = %d %v", i+1, status, data)
		}
	}
	if status, data := openLink(t, srv, token, ""); status != 410 || data["_errorCode"] != "LINK_EXPIRED" {
		t.Fatalf("third view = %d %v", status, data)
	}
}

func TestAnExpiredOrRevokedLinkIsClosed(t *testing.T) {
	srv := newServerUnderTest(t)
	enableLinks(t, srv, true)
	ctx := context.Background()

	expiring := ownedDocument(t, srv, "만료 문서")
	soon := time.Now().Add(2 * time.Second)
	expiredToken, _ := makeLink(t, srv, expiring, map[string]any{"expiresAt": soon.Format(time.RFC3339)})
	if _, err := srv.db.Exec(ctx,
		`UPDATE document_links SET expires_at = now() - interval '1 minute' WHERE document_id=$1`, expiring); err != nil {
		t.Fatal(err)
	}
	if status, _ := openLink(t, srv, expiredToken, ""); status != 410 {
		t.Fatalf("expired link = %d", status)
	}

	revoking := ownedDocument(t, srv, "해지 문서")
	revokedToken, created := makeLink(t, srv, revoking, map[string]any{})
	linkID := created["id"].(string)
	req, _ := http.NewRequest("DELETE",
		srv.URL+"/api/v1/documents/"+revoking.String()+"/links/"+linkID, nil)
	resp, err := srv.admin.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("revoke = %d", resp.StatusCode)
	}
	if status, _ := openLink(t, srv, revokedToken, ""); status != 410 {
		t.Fatalf("revoked link = %d", status)
	}
	// The row survives revocation: it is the record that this went outside.
	var stillThere bool
	_ = srv.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM document_links WHERE document_id=$1)`, revoking).Scan(&stillThere)
	if !stillThere {
		t.Fatal("revoking must not erase the record that the document was shared")
	}
}

func TestTurningTheSettingOffClosesLinksThatAlreadyExist(t *testing.T) {
	// A switch that only stops new links is not the switch an administrator
	// reaches for when a document has gone somewhere it should not.
	srv := newServerUnderTest(t)
	enableLinks(t, srv, true)
	documentID := ownedDocument(t, srv, "정책 문서")
	token, _ := makeLink(t, srv, documentID, map[string]any{})
	if status, _ := openLink(t, srv, token, ""); status != 200 {
		t.Fatal("the link should work while the policy allows it")
	}

	enableLinks(t, srv, false)
	if status, data := openLink(t, srv, token, ""); status != 403 || data["_errorCode"] != "PUBLIC_LINK_DISABLED" {
		t.Fatalf("after disabling = %d %v", status, data)
	}
	// And no new ones can be made.
	status, data := postJSON(t, srv.admin, srv.URL+"/api/v1/documents/"+documentID.String()+"/links", map[string]any{})
	if status != 403 || data["_errorCode"] != "PUBLIC_LINK_DISABLED" {
		t.Fatalf("create while disabled = %d %v", status, data)
	}
}

func TestATrashedDocumentIsNotServedByItsLink(t *testing.T) {
	srv := newServerUnderTest(t)
	enableLinks(t, srv, true)
	documentID := ownedDocument(t, srv, "휴지통 문서")
	token, _ := makeLink(t, srv, documentID, map[string]any{})
	if _, err := srv.db.Exec(context.Background(),
		`UPDATE documents SET deleted_at = now() WHERE id=$1`, documentID); err != nil {
		t.Fatal(err)
	}
	if status, _ := openLink(t, srv, token, ""); status != 404 {
		t.Fatalf("trashed document = %d", status)
	}
}

func TestOnlyTheOwnerMakesLinks(t *testing.T) {
	srv := newServerUnderTest(t)
	enableLinks(t, srv, true)
	documentID := ownedDocument(t, srv, "남의 문서")
	outsider := createAccount(t, srv, "outsider@example.com", "남")
	if _, err := srv.db.Exec(context.Background(),
		`UPDATE users SET password_reset_required=false, password_hash=$2 WHERE id=$1`,
		outsider, mustHash(t, "외부인비밀번호입니다")); err != nil {
		t.Fatal(err)
	}
	client := signIn(t, srv.Server, "outsider@example.com", "외부인비밀번호입니다")
	status, _ := postJSON(t, client, srv.URL+"/api/v1/documents/"+documentID.String()+"/links", map[string]any{})
	if status != 403 && status != 404 {
		t.Fatalf("an outsider made a link: %d", status)
	}
}

func TestTheAccessViewShowsTheLink(t *testing.T) {
	// The screen that answers "who can open this" would be wrong the moment
	// links existed if it did not include them.
	srv := newServerUnderTest(t)
	enableLinks(t, srv, true)
	documentID := ownedDocument(t, srv, "링크 있는 문서")
	makeLink(t, srv, documentID, map[string]any{"label": "감사용"})

	resp, err := srv.admin.Get(srv.URL + "/api/v1/admin/documents/" + documentID.String() + "/access")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var view struct {
		Data struct {
			Links []struct {
				Label string `json:"label"`
			} `json:"links"`
			Notes []string `json:"notes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Data.Links) != 1 || view.Data.Links[0].Label != "감사용" {
		t.Fatalf("links = %+v", view.Data.Links)
	}
	if len(view.Data.Notes) == 0 {
		t.Fatal("an administrator should be told the document is reachable without an account")
	}
}

func mustHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := database.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}
