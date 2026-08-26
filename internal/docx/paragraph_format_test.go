package docx

import (
	"encoding/json"
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

// roundTrip writes a document to .docx and reads it straight back.
func roundTrip(t *testing.T, source string) *richdoc.Node {
	t.Helper()
	node, err := richdoc.Parse(json.RawMessage(source))
	if err != nil {
		t.Fatal(err)
	}
	built, err := Build(node, Options{Title: "왕복"})
	if err != nil {
		t.Fatal(err)
	}
	back, _, err := Parse(built)
	if err != nil {
		t.Fatal(err)
	}
	return back
}

// firstParagraph returns the first paragraph after the title heading.
func firstParagraph(t *testing.T, doc *richdoc.Node) *richdoc.Node {
	t.Helper()
	for _, child := range doc.Content {
		if child.Type == "paragraph" {
			return child
		}
	}
	t.Fatal("no paragraph came back")
	return nil
}

// muni wrote indentation and line spacing into every .docx it produced and read
// none of it back, so a document formatted here, exported to Word and opened
// again came back flat. A Word document written the way a Korean office writes
// one — 줄 간격 160%, 첫 줄 들여쓰기 — lost both on the way in.
func TestParagraphFormattingSurvivesTheRoundTrip(t *testing.T) {
	doc := roundTrip(t, `{"type":"doc","content":[
		{"type":"paragraph","attrs":{"firstLine":true,"textAlign":"justify","lineHeight":"1.6"},
		 "content":[{"type":"text","text":"본문"}]}]}`)
	p := firstParagraph(t, doc)
	if !p.AttrBool("firstLine") {
		t.Error("첫 줄 들여쓰기가 사라졌습니다")
	}
	if got := p.AttrString("textAlign"); got != "justify" {
		t.Errorf("textAlign = %q", got)
	}
	if got := p.AttrString("lineHeight"); got != "1.6" {
		t.Errorf("lineHeight = %q, want 1.6", got)
	}
}

func TestIndentationComesBackAtTheSameDepth(t *testing.T) {
	for _, steps := range []int{1, 2, 5} {
		doc := roundTrip(t, `{"type":"doc","content":[
			{"type":"paragraph","attrs":{"indent":`+string(rune('0'+steps))+`},
			 "content":[{"type":"text","text":"들여쓴 문단"}]}]}`)
		if got := firstParagraph(t, doc).AttrInt("indent", 0); got != steps {
			t.Errorf("indent %d 단계가 %d 단계로 돌아왔습니다", steps, got)
		}
	}
}

func TestAListKeepsItsOwnIndentation(t *testing.T) {
	// A list item's w:ind is the list's, not something the author set. Reading
	// it as an author's indent would push every bullet in one step further
	// each time the file went round.
	doc := roundTrip(t, `{"type":"doc","content":[
		{"type":"bulletList","content":[
			{"type":"listItem","content":[
				{"type":"paragraph","content":[{"type":"text","text":"항목"}]}]}]}]}`)
	var check func(*richdoc.Node)
	check = func(node *richdoc.Node) {
		if node.Type == "paragraph" && node.AttrInt("indent", 0) != 0 {
			t.Errorf("목록 항목이 들여쓰기 %d 단계를 얻었습니다", node.AttrInt("indent", 0))
		}
		for _, child := range node.Content {
			check(child)
		}
	}
	check(doc)
}

func TestSingleSpacingIsNotReportedAsASetting(t *testing.T) {
	// Word writes a w:line for paragraphs nobody deliberately spaced. Showing
	// that back as an explicit line height fills the editor's box with a
	// number the author never chose.
	doc := roundTrip(t, `{"type":"doc","content":[
		{"type":"paragraph","content":[{"type":"text","text":"평범한 문단"}]}]}`)
	if got := firstParagraph(t, doc).AttrString("lineHeight"); got != "" {
		t.Errorf("lineHeight = %q; 지정하지 않은 문단에는 값이 없어야 합니다", got)
	}
}
