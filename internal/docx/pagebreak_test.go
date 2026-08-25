package docx

import (
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

func buildBody(t *testing.T, node *richdoc.Node) string {
	t.Helper()
	b := &builder{}
	b.block(node, blockContext{})
	return b.body.String()
}

func TestPageBreakBecomesAWordPageBreak(t *testing.T) {
	body := buildBody(t, &richdoc.Node{Type: "pageBreak"})
	if !strings.Contains(body, `<w:br w:type="page"/>`) {
		t.Fatalf("page break was not written: %s", body)
	}
}

func TestLineSpacingReachesTheExport(t *testing.T) {
	node := &richdoc.Node{Type: "paragraph", Content: []*richdoc.Node{richdoc.Text("본문")}}
	node.SetAttr("lineHeight", "1.5")
	body := buildBody(t, node)
	// 240 twips is single spacing, so 1.5 lines is 360.
	if !strings.Contains(body, `w:line="360"`) {
		t.Fatalf("line spacing was dropped: %s", body)
	}
	if !strings.Contains(body, `w:lineRule="auto"`) {
		t.Fatalf("spacing must be proportional, not fixed: %s", body)
	}
}

func TestIndentationReachesTheExport(t *testing.T) {
	node := &richdoc.Node{Type: "paragraph", Content: []*richdoc.Node{richdoc.Text("본문")}}
	node.SetAttr("indent", 2)
	node.SetAttr("firstLine", true)
	body := buildBody(t, node)
	if !strings.Contains(body, `w:left="880"`) {
		t.Fatalf("paragraph indent was dropped: %s", body)
	}
	if !strings.Contains(body, `w:firstLine="220"`) {
		t.Fatalf("first line indent was dropped: %s", body)
	}
}

func TestOnlyOneSpacingElementInsideATable(t *testing.T) {
	// CT_PPr allows one w:spacing; Word rejects the part when there are two.
	node := &richdoc.Node{Type: "paragraph", Content: []*richdoc.Node{richdoc.Text("셀")}}
	node.SetAttr("lineHeight", "2")
	b := &builder{}
	b.block(node, blockContext{inTable: true})
	body := b.body.String()
	if strings.Count(body, "<w:spacing") != 1 {
		t.Fatalf("expected exactly one spacing element: %s", body)
	}
	if !strings.Contains(body, `w:line="480"`) {
		t.Fatalf("the author's spacing should win inside a table too: %s", body)
	}
}

func TestAbsurdLineSpacingIsIgnored(t *testing.T) {
	node := &richdoc.Node{Type: "paragraph", Content: []*richdoc.Node{richdoc.Text("본문")}}
	node.SetAttr("lineHeight", "999")
	body := buildBody(t, node)
	if strings.Contains(body, "<w:spacing") {
		t.Fatalf("a value out of range should be ignored: %s", body)
	}
}

func TestPageBreakSurvivesTheRoundTrip(t *testing.T) {
	doc := &richdoc.Node{Type: "doc", Content: []*richdoc.Node{
		richdoc.Paragraph(richdoc.Text("첫 장")),
		{Type: "pageBreak"},
		richdoc.Paragraph(richdoc.Text("둘째 장")),
	}}
	data, err := Build(doc, Options{})
	if err != nil {
		t.Fatal(err)
	}
	imported, _, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	types := make([]string, 0, len(imported.Content))
	for _, node := range imported.Content {
		types = append(types, node.Type)
	}
	found := false
	for _, kind := range types {
		if kind == "pageBreak" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the page break did not come back: %v", types)
	}
}
