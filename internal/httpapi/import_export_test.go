package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMarkdownImport(t *testing.T) {
	document, err := markdownDocument("# 제목\n\n본문\n- [x] 완료")
	if err != nil {
		t.Fatal(err)
	}
	if !validDocumentJSON(document) {
		t.Fatalf("invalid document: %s", document)
	}
	text := extractDocumentText(document)
	if !strings.Contains(text, "제목") || !strings.Contains(text, "완료") {
		t.Fatalf("missing imported text: %q", text)
	}
}

func TestHTMLImportDropsScript(t *testing.T) {
	document, err := htmlDocument([]byte(`<html><body><h1>안전한 제목</h1><p>본문</p><script>alert('x')</script></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	text := extractDocumentText(document)
	if strings.Contains(text, "alert") || !strings.Contains(text, "안전한 제목") {
		t.Fatalf("unexpected imported text: %q", text)
	}
}

func TestDOCXExportImportRoundTrip(t *testing.T) {
	content, _ := json.Marshal(map[string]any{"type": "doc", "content": []any{
		map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "한글 문서 본문"}}},
	}})
	docx, err := makeDOCX("테스트 제목", content)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := docxImport(docx)
	if err != nil {
		t.Fatal(err)
	}
	text := extractDocumentText(imported)
	if !strings.Contains(text, "테스트 제목") || !strings.Contains(text, "한글 문서 본문") {
		t.Fatalf("DOCX round trip lost text: %q", text)
	}
}

func TestHTMLRendererEscapesContent(t *testing.T) {
	content := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"<script>alert(1)</script>"}]}]}`)
	rendered := renderHTML(content)
	if strings.Contains(rendered, "<script>") || !strings.Contains(rendered, "&lt;script&gt;") {
		t.Fatalf("unsafe HTML: %s", rendered)
	}
}
