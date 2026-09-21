package httpapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

func TestDOCXFileImportsWithoutItsTitleTwice(t *testing.T) {
	srv := newServerUnderTest(t)
	workspaceID := adminWorkspace(t, srv)
	file := buildDOCXFile(t, "회의록", docxTitleBody)
	document := importFile(t, srv, workspaceID, "회의록.docx", "", file)
	if document["title"] != "회의록" {
		t.Fatalf("title = %v", document["title"])
	}
	first, text := firstBlockType(t, srv, document["id"].(string))
	if first != "paragraph" || strings.Contains(text, "회의록") || !strings.Contains(text, "첫 문단") || !strings.Contains(text, "안건") || !strings.Contains(text, "항목") {
		t.Errorf("first block = %q, search text = %q", first, text)
	}
	var content, revision json.RawMessage
	var revisionText string
	var same bool
	if err := srv.db.QueryRow(t.Context(), `SELECT d.content_json,r.content_json,r.content_text,d.content_json=r.content_json
		FROM documents d JOIN document_revisions r ON r.document_id=d.id
		WHERE d.id=$1 AND r.revision_no=1`, document["id"]).Scan(&content, &revision, &revisionText, &same); err != nil {
		t.Fatal(err)
	}
	if !same || revisionText != text || strings.Contains(extractDocumentText(revision), "회의록") {
		t.Errorf("initial revision differs or retains title: %s, text=%q", revision, revisionText)
	}
}

func TestDOCXImportPreservesUnmatchedAndLaterHeadings(t *testing.T) {
	srv := newServerUnderTest(t)
	workspaceID := adminWorkspace(t, srv)
	for _, tc := range []struct {
		name, exportTitle, formTitle, body string
		drop                               bool
	}{
		{"different form title", "회의록", "9월 회의", docxTitleBody, false},
		{"H2 first", "", "", `{"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"회의록"}]}]}`, false},
		{"paragraph before H1", "", "", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"회의록"}]},{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"회의록"}]}]}`, false},
		{"consecutive matching H1", "회의록", "", `{"type":"doc","content":[{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"회의록"}]}]}`, true},
		{"normalized filename", "계획/2026", "", docxTitleBody, false},
		{"truncated title", strings.Repeat("가", 241), strings.Repeat("가", 241), docxTitleBody, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := buildDOCXFile(t, tc.exportTitle, tc.body)
			name := "회의록.docx"
			if tc.name == "normalized filename" {
				name = safeFilename(tc.exportTitle) + ".docx"
			}
			document := importFile(t, srv, workspaceID, name, tc.formTitle, file)
			parsed, err := parseUpload(t.Context(), ".docx", file)
			if err != nil {
				t.Fatal(err)
			}
			expected, err := richdoc.Parse(parsed.content)
			if err != nil {
				t.Fatal(err)
			}
			if tc.drop {
				expected.Content = expected.Content[1:]
			}
			var stored json.RawMessage
			if err := srv.db.QueryRow(t.Context(), `SELECT content_json FROM documents WHERE id=$1`, document["id"]).Scan(&stored); err != nil {
				t.Fatal(err)
			}
			actual, err := richdoc.Parse(stored)
			if err != nil {
				t.Fatal(err)
			}
			// Newly assigned block identities are the only expected addition.
			var removeIDs func(*richdoc.Node)
			removeIDs = func(n *richdoc.Node) {
				delete(n.Attrs, richdoc.BlockIDAttr)
				for _, child := range n.Content {
					removeIDs(child)
				}
			}
			removeIDs(actual)
			want, _ := json.Marshal(expected)
			got, _ := json.Marshal(actual)
			if string(got) != string(want) {
				t.Errorf("body changed: got %s, want %s", got, want)
			}
		})
	}
}

func TestDOCXImportIntoDocumentKeepsTitle(t *testing.T) {
	srv := newServerUnderTest(t)
	workspaceID := adminWorkspace(t, srv)
	file := buildDOCXFile(t, "회의록", docxTitleBody)
	document := importFile(t, srv, workspaceID, "회의록.docx", "", file)
	status, data := importIntoDocument(t, srv, document["id"].(string), "회의록.docx", file)
	if status != 200 {
		t.Fatalf("import into = %d %v", status, data)
	}
	content, err := json.Marshal(data["content"])
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := richdoc.Parse(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Content) != 4 || parsed.Content[0].Type != "heading" || strings.TrimSpace(parsed.Content[0].PlainText()) != "회의록" {
		t.Fatalf("insert lost title: %s", content)
	}
}

func TestDOCXTitleOnlyImportsAsEmptyParagraph(t *testing.T) {
	srv := newServerUnderTest(t)
	document := importFile(t, srv, adminWorkspace(t, srv), "회의록.docx", "", buildDOCXFile(t, "회의록", `{"type":"doc"}`))
	first, text := firstBlockType(t, srv, document["id"].(string))
	if first != "paragraph" || strings.TrimSpace(text) != "" {
		t.Fatalf("first=%q text=%q", first, text)
	}
}
