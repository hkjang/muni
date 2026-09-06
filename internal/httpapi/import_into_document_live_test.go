package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// A file dropped on an open document comes back as content for the editor
// to put where it landed, with its pictures already this document's
// attachments — and the document itself untouched until the editor saves.
func importIntoDocument(t *testing.T, srv *serverUnderTest, documentID string, name string, file []byte) (int, map[string]any) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(file); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/documents/"+documentID+"/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := srv.admin.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out struct {
		Data  map[string]any `json:"data"`
		Error map[string]any `json:"error"`
	}
	_ = json.Unmarshal(raw, &out)
	if out.Data == nil {
		out.Data = out.Error
	}
	return resp.StatusCode, out.Data
}

func TestAFileDroppedOnADocumentComesBackAsContent(t *testing.T) {
	srv := newServerUnderTest(t)
	workspaceID := adminWorkspace(t, srv)
	created := importWordFile(t, srv, workspaceID, wordFileAWordUserWouldSend(t))
	documentID := created["id"].(string)

	status, data := importIntoDocument(t, srv, documentID, "메모.md", []byte("## 넣은 제목\n\n넣은 본문입니다."))
	if status != 200 {
		t.Fatalf("import into document = %d %v", status, data)
	}
	content, _ := json.Marshal(data["content"])
	if !strings.Contains(string(content), `"heading"`) || !strings.Contains(string(content), "넣은 본문입니다") {
		t.Errorf("내용이 돌아오지 않았습니다: %s", content)
	}
	if data["format"] != "md" || data["title"] != "메모" {
		t.Errorf("format/title = %v / %v", data["format"], data["title"])
	}

	// The document itself is untouched: the editor decides where it goes.
	var stored string
	if err := srv.db.QueryRow(t.Context(), `SELECT content_text FROM documents WHERE id=$1`, uuid.MustParse(documentID)).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "넣은 본문") {
		t.Errorf("문서가 편집기를 거치지 않고 바뀌었습니다: %q", stored)
	}

	// A file with a picture leaves the picture as this document's attachment.
	status, data = importIntoDocument(t, srv, documentID, "그림.docx", wordFileAWordUserWouldSend(t))
	if status != 200 {
		t.Fatalf("import into document = %d %v", status, data)
	}
	if images, _ := data["images"].(float64); images != 1 {
		t.Errorf("images = %v", data["images"])
	}
	// The document had one picture from its own import; the drop adds one.
	var attachments int
	_ = srv.db.QueryRow(t.Context(), `SELECT count(*) FROM attachments WHERE document_id=$1`, uuid.MustParse(documentID)).Scan(&attachments)
	if attachments != 2 {
		t.Errorf("첨부가 %d개입니다", attachments)
	}

	status, data = importIntoDocument(t, srv, documentID, "archive.zip", []byte("PK"))
	if status != 400 || data["code"] != "UNSUPPORTED_IMPORT" {
		t.Errorf("unsupported = %d %v", status, data)
	}
}
