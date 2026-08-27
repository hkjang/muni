package docx

import (
	"encoding/json"
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

// Word writes a cell's vertical alignment per cell and defaults to the top.
// muni had no place to keep it and wrote every exported cell as centred, so a
// table with a tall 비고 column whose text the author had left at the top came
// back centred — not a loss of the alignment so much as a replacement of it.

func alignedCell(align, text string) string {
	properties := `<w:tcPr>`
	if align != "" {
		properties += `<w:vAlign w:val="` + align + `"/>`
	}
	properties += `</w:tcPr>`
	return `<w:tc>` + properties + `<w:p><w:r><w:t>` + escapeXML(text) + `</w:t></w:r></w:p></w:tc>`
}

func cellAlignments(t *testing.T, document *richdoc.Node) []string {
	t.Helper()
	var out []string
	var walk func(*richdoc.Node)
	walk = func(node *richdoc.Node) {
		if node == nil {
			return
		}
		if node.Type == "tableCell" || node.Type == "tableHeader" {
			out = append(out, node.AttrString("verticalAlign"))
		}
		for _, child := range node.Content {
			walk(child)
		}
	}
	walk(document)
	return out
}

func TestACellKeepsWhereItsTextSat(t *testing.T) {
	body := `<w:tbl><w:tblPr/><w:tr>` +
		alignedCell("", "말없는칸") + alignedCell("center", "가운데칸") + alignedCell("bottom", "아래칸") +
		`</w:tr></w:tbl>`
	document, _, _, err := Parse(wordPackage(t, body))
	if err != nil {
		t.Fatal(err)
	}
	got := cellAlignments(t, document)
	want := []string{"top", "middle", "bottom"}
	if len(got) != len(want) {
		t.Fatalf("칸 = %v", got)
	}
	for index, alignment := range want {
		if got[index] != alignment {
			t.Errorf("%d번째 칸 = %q, %q 를 기대했습니다", index, got[index], alignment)
		}
	}
}

func TestTheAlignmentSurvivesTheRoundTrip(t *testing.T) {
	body := `<w:tbl><w:tblPr/><w:tr>` + alignedCell("", "위칸") + alignedCell("bottom", "아래칸") + `</w:tr></w:tbl>`
	imported, _, _, err := Parse(wordPackage(t, body))
	if err != nil {
		t.Fatal(err)
	}
	built, err := Build(imported, Options{Title: "표"})
	if err != nil {
		t.Fatal(err)
	}
	back, _, _, err := Parse(built)
	if err != nil {
		t.Fatal(err)
	}
	got := cellAlignments(t, back)
	if len(got) != 2 || got[0] != "top" || got[1] != "bottom" {
		t.Fatalf("왕복 뒤 = %v", got)
	}
}

// A table muni wrote itself says nothing about alignment, and muni has always
// shown it at the top: the editor's stylesheet and the one the PDF is printed
// from both say so. The Word export was the only thing that centred it.
func TestAMuniTableExportsTheWayItLooks(t *testing.T) {
	source := `{"type":"doc","content":[{"type":"table","content":[{"type":"tableRow","content":[
		{"type":"tableCell","attrs":{"colspan":1,"rowspan":1},"content":[{"type":"paragraph","content":[{"type":"text","text":"칸"}]}]}]}]}]}`
	node, err := richdoc.Parse(json.RawMessage(source))
	if err != nil {
		t.Fatal(err)
	}
	built, err := Build(node, Options{Title: "표"})
	if err != nil {
		t.Fatal(err)
	}
	if body := documentXMLOf(t, built); !contains(body, `<w:vAlign w:val="top"/>`) {
		t.Error("화면에서는 위, 워드에서는 가운데입니다")
	}
}
