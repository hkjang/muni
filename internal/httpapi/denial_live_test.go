package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// A refusal that arrives as 200 with an empty body is the worst kind: the
// action is correctly blocked, and every client is told it worked. An editor
// sees its save confirmed and keeps typing into a document the server threw
// away; a share that never happened looks shared.
//
// That is what every one of these endpoints did, because the handlers returned
// without writing anything when the permission lookup failed. This is the test
// that stops it coming back.
func TestARefusalSaysItIsARefusal(t *testing.T) {
	srv := newServerUnderTest(t)
	ctx := context.Background()
	documentID := ownedDocument(t, srv, "남의 문서")

	outsider := createAccount(t, srv, "denied@example.com", "권한없는사람")
	if _, err := srv.db.Exec(ctx,
		`UPDATE users SET password_reset_required=false, password_hash=$2 WHERE id=$1`,
		outsider, mustHash(t, "권한없는사람비밀번호")); err != nil {
		t.Fatal(err)
	}
	client := signIn(t, srv.Server, "denied@example.com", "권한없는사람비밀번호")

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"editing", "PUT", "/api/v1/documents/{id}", `{"title":"내가 바꿈"}`},
		{"sharing", "PUT", "/api/v1/documents/{id}/permissions",
			`{"subjectType":"USER","subjectId":"` + outsider.String() + `","role":"EDITOR"}`},
		{"commenting", "POST", "/api/v1/documents/{id}/comments", `{"body":"댓글"}`},
		{"making a share link", "POST", "/api/v1/documents/{id}/links", `{}`},
		{"listing share links", "GET", "/api/v1/documents/{id}/links", ""},
		{"tagging", "PUT", "/api/v1/documents/{id}/tags", `{"tags":["가로채기"]}`},
		{"moving", "POST", "/api/v1/documents/{id}/move", `{"folderId":null}`},
		{"suggesting an edit", "POST", "/api/v1/documents/{id}/suggestions", `{"body":"제안"}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := srv.URL + replaceID(c.path, documentID)
			var reader io.Reader
			if c.body != "" {
				reader = bytes.NewReader([]byte(c.body))
			}
			req, err := http.NewRequest(c.method, path, reader)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			raw, _ := io.ReadAll(resp.Body)

			if resp.StatusCode == http.StatusNotFound &&
				bytes.Contains(raw, []byte("요청한 API를 찾을 수 없습니다")) {
				t.Skipf("no such route: %s %s", c.method, c.path)
			}
			if resp.StatusCode < 400 {
				t.Fatalf("refused but answered %d %q", resp.StatusCode, raw)
			}
			// And it has to say something. An empty body with a 4xx is only
			// marginally better than an empty body with a 200.
			var envelope struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if json.Unmarshal(raw, &envelope) != nil || envelope.Error.Code == "" {
				t.Fatalf("no error in the body: %d %q", resp.StatusCode, raw)
			}
			if envelope.Error.Message == "" {
				t.Fatalf("no message for the reader: %q", raw)
			}
		})
	}

	// Nothing moved, on any of them.
	var title string
	if err := srv.db.QueryRow(ctx, `SELECT title FROM documents WHERE id=$1`, documentID).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "남의 문서" {
		t.Fatalf("the document changed: %q", title)
	}
	var shared, linked int
	_ = srv.db.QueryRow(ctx, `SELECT count(*) FROM document_permissions WHERE document_id=$1`, documentID).Scan(&shared)
	_ = srv.db.QueryRow(ctx, `SELECT count(*) FROM document_links WHERE document_id=$1`, documentID).Scan(&linked)
	if shared != 0 || linked != 0 {
		t.Fatalf("permissions=%d links=%d", shared, linked)
	}
}

func TestAMissingDocumentIsNotAPermissionProblem(t *testing.T) {
	// Answering 403 for something that does not exist tells the asker it does.
	srv := newServerUnderTest(t)
	req, _ := http.NewRequest("PUT", srv.URL+"/api/v1/documents/"+uuid.New().String(),
		bytes.NewReader([]byte(`{"title":"없는 문서"}`)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.admin.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("= %d %q, want 404", resp.StatusCode, raw)
	}
}

func replaceID(path string, id uuid.UUID) string {
	return bytes.NewBuffer(bytes.ReplaceAll([]byte(path), []byte("{id}"), []byte(id.String()))).String()
}
