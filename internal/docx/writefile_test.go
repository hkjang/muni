package docx

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

// TestWriteSampleForInspection writes a document that uses every new setting
// so an outside reader can check what Word is actually handed.
func TestWriteSampleForInspection(t *testing.T) {
	path := os.Getenv("MUNI_DOCX_SAMPLE")
	if path == "" {
		t.Skip("set MUNI_DOCX_SAMPLE to write a sample")
	}
	raw := `{"type":"doc","content":[
		{"type":"heading","attrs":{"level":1,"lineHeight":"1.5"},"content":[{"type":"text","text":"보고서"}]},
		{"type":"paragraph","attrs":{"lineHeight":"1.6","indent":2,"firstLine":true,"textAlign":"justify"},
		 "content":[{"type":"text","text":"들여쓰기와 줄 간격이 함께 적용된 문단입니다."}]},
		{"type":"pageBreak"},
		{"type":"paragraph","content":[{"type":"text","text":"둘째 장 첫 문단"}]},
		{"type":"table","content":[{"type":"tableRow","content":[
			{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"항목"}]}]},
			{"type":"tableCell","content":[{"type":"paragraph","attrs":{"lineHeight":"2"},"content":[{"type":"text","text":"셀"}]}]}]}]}
	]}`
	doc, err := richdoc.Parse(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	data, err := Build(doc, Options{Title: "검증"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
