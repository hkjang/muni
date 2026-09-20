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

// importFile uploads one file as a new document, with the title the form
// carries (empty for none), and returns the document the endpoint answers.
func importFile(t *testing.T, srv *serverUnderTest, workspaceID uuid.UUID, name, title string, file []byte) map[string]any {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("workspaceId", workspaceID.String()); err != nil {
		t.Fatal(err)
	}
	if title != "" {
		if err := writer.WriteField("title", title); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(file); err != nil {
		t.Fatal(err)
	}
	writer.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := srv.admin.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		t.Fatalf("import = %d %s", resp.StatusCode, raw)
	}
	var out struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out.Data
}

// firstBlockType reads the type of the first block a document was stored with.
func firstBlockType(t *testing.T, srv *serverUnderTest, documentID string) (string, string) {
	t.Helper()
	var content json.RawMessage
	var text string
	if err := srv.db.QueryRow(t.Context(), `SELECT content_json,content_text FROM documents WHERE id=$1`, uuid.MustParse(documentID)).Scan(&content, &text); err != nil {
		t.Fatal(err)
	}
	var document struct {
		Content []struct {
			Type string `json:"type"`
		} `json:"content"`
	}
	if err := json.Unmarshal(content, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Content) == 0 {
		t.Fatalf("document %s has no blocks", documentID)
	}
	return document.Content[0].Type, text
}

// A Markdown file muni exported starts with its own title as a heading, and
// is named after it. Imported back, the title is decided from the file name
// and the heading that only repeats it leaves the body — and the search text.
func TestAnExportedMarkdownFileImportsWithoutItsTitleTwice(t *testing.T) {
	srv := newServerUnderTest(t)
	workspaceID := adminWorkspace(t, srv)

	exported := []byte("# 회의록\n\n첫 문단.\n\n# 안건\n\n둘째 문단.\n")
	document := importFile(t, srv, workspaceID, "회의록.md", "", exported)
	if document["title"] != "회의록" {
		t.Fatalf("title = %v", document["title"])
	}
	first, text := firstBlockType(t, srv, document["id"].(string))
	if first != "paragraph" {
		t.Errorf("first block = %q, want the paragraph after the title", first)
	}
	if strings.Contains(text, "회의록") || !strings.Contains(text, "안건") {
		t.Errorf("search text = %q", text)
	}

	// The same file under another title is someone's document whose first
	// heading is theirs.
	document = importFile(t, srv, workspaceID, "회의록.md", "9월 회의", exported)
	if document["title"] != "9월 회의" {
		t.Fatalf("title = %v", document["title"])
	}
	first, text = firstBlockType(t, srv, document["id"].(string))
	if first != "heading" || !strings.Contains(text, "회의록") {
		t.Errorf("first block = %q, text = %q: the author's heading was taken for the title", first, text)
	}

	// Dropped on an open document the rule does not apply: the title goes to
	// the editor separately and the body is placed whole.
	status, data := importIntoDocument(t, srv, document["id"].(string), "메모.md", []byte("# 메모\n\n넣은 본문입니다."))
	if status != 200 {
		t.Fatalf("import into document = %d %v", status, data)
	}
	content, _ := json.Marshal(data["content"])
	if !strings.Contains(string(content), `"heading"`) || data["title"] != "메모" {
		t.Errorf("dropped file lost its heading: title=%v %s", data["title"], content)
	}
}

// An HTML file muni exported writes its title as the first heading of the
// body too, and the same rule applies: the file is what the export function
// writes, so a change to the export's shape is felt here.
func TestAnExportedHTMLFileImportsWithoutItsTitleTwice(t *testing.T) {
	srv := newServerUnderTest(t)
	workspaceID := adminWorkspace(t, srv)

	body := json.RawMessage(`{"type":"doc","content":[
		{"type":"paragraph","content":[{"type":"text","text":"첫 문단."}]},
		{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"안건"}]},
		{"type":"paragraph","content":[{"type":"text","text":"둘째 문단."}]}
	]}`)
	exported := []byte(fullHTMLWithDrawing("회의록", false, renderHTML(body), false))
	document := importFile(t, srv, workspaceID, "회의록.html", "", exported)
	if document["title"] != "회의록" {
		t.Fatalf("title = %v", document["title"])
	}
	first, text := firstBlockType(t, srv, document["id"].(string))
	if first != "paragraph" {
		t.Errorf("first block = %q, want the paragraph after the title", first)
	}
	if strings.Contains(text, "회의록") || !strings.Contains(text, "안건") {
		t.Errorf("search text = %q", text)
	}

	// Under another title the export's heading is the author's and stays.
	document = importFile(t, srv, workspaceID, "회의록.html", "9월 회의", exported)
	if document["title"] != "9월 회의" {
		t.Fatalf("title = %v", document["title"])
	}
	first, text = firstBlockType(t, srv, document["id"].(string))
	if first != "heading" || !strings.Contains(text, "회의록") {
		t.Errorf("first block = %q, text = %q: the author's heading was taken for the title", first, text)
	}

	// A plain text file never had a title written into it; its first line
	// stays even when it says what the title says.
	document = importFile(t, srv, workspaceID, "회의록.txt", "", []byte("회의록\n\n첫 문단.\n"))
	if document["title"] != "회의록" {
		t.Fatalf("title = %v", document["title"])
	}
	if _, text = firstBlockType(t, srv, document["id"].(string)); !strings.HasPrefix(text, "회의록") {
		t.Errorf("plain text lost its first line: %q", text)
	}

	// Dropped on an open document the rule does not apply.
	status, data := importIntoDocument(t, srv, document["id"].(string), "회의록.html", exported)
	if status != 200 {
		t.Fatalf("import into document = %d %v", status, data)
	}
	content, _ := json.Marshal(data["content"])
	if !strings.Contains(string(content), `"heading"`) || data["title"] != "회의록" {
		t.Errorf("dropped file lost its heading: title=%v %s", data["title"], content)
	}
}
