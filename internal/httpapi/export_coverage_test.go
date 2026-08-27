package httpapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

// everyNode is one document carrying every kind of block and inline the editor
// can make. Each one holds a distinctive phrase so a format that drops it is
// obvious.
//
// This exists because the same mistake kept arriving in different clothes: an
// exporter meets a node it does not recognise, walks into its children, and
// splices them somewhere they do not belong — or loses them. Footnotes did it
// in all four formats at once, and each was found separately. One document
// through every path finds the next one in a single run.
//
// It lives in frontend/testdata/every-node.json because the editor's own test
// reads the same file, and the frontend has to build from its own directory
// alone — the production image copies nothing else. Two copies of "every kind
// of content" drift apart, and the copy that drifts stops finding things.
func everyNode(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "frontend", "testdata", "every-node.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// phrases every format has to carry through, whatever it does with the markup
// around them.
var carriedPhrases = []string{
	"제목입니다", "평문과", "굵은글씨", "기울임", "밑줄", "취소선", "코드조각",
	"링크글자", "위첨자표시", "아래첨자표시", "형광펜표시", "글자서식표시",
	"각주내용입니다", "줄바꿈뒤문장",
	"인용문입니다", "글머리항목", "하위항목", "셋째항목", "번호항목",
	"할일항목", "코드블록내용", "graph TD",
	"표머리글", "표셀내용", "마지막문단",
}

func TestEveryExportFormatCarriesEveryKindOfContent(t *testing.T) {
	raw := json.RawMessage(everyNode(t))
	formats := []struct {
		name   string
		render func() string
	}{
		{"HTML", func() string { return renderHTML(raw) }},
		{"Markdown", func() string { return renderMarkdown("문서 제목", raw) }},
		{"plain text", func() string { return renderPlainText("문서 제목", raw) }},
	}
	for _, format := range formats {
		t.Run(format.name, func(t *testing.T) {
			out := format.render()
			for _, phrase := range carriedPhrases {
				if !strings.Contains(out, phrase) {
					t.Errorf("%q 가 사라졌습니다", phrase)
				}
			}
			// The contents list is generated at export time, so the heading
			// has to appear in it as well as where it was written.
			if strings.Count(out, "제목입니다") < 2 {
				t.Errorf("목차에 제목이 들어가지 않았습니다:\n%s", out)
			}
		})
	}
}

// A footnote is not part of the sentence it annotates. Every format has to put
// it somewhere else — which is the rule the exporters kept breaking.
func TestNoFormatPutsTheNoteInsideTheSentence(t *testing.T) {
	raw := json.RawMessage(everyNode(t))
	for _, format := range []struct {
		name   string
		render func() string
	}{
		{"HTML", func() string { return renderHTML(raw) }},
		{"Markdown", func() string { return renderMarkdown("문서 제목", raw) }},
		{"plain text", func() string { return renderPlainText("문서 제목", raw) }},
	} {
		t.Run(format.name, func(t *testing.T) {
			out := format.render()
			// The sentence ends with 아래첨자표시 and the note follows it. If
			// the note's words were spliced in, they would sit between the two.
			sentence := strings.Index(out, "아래첨자표시")
			note := strings.Index(out, "각주내용입니다")
			if sentence < 0 || note < 0 {
				t.Fatalf("문장이나 각주를 찾지 못했습니다:\n%s", out)
			}
			between := out[sentence:note]
			if !strings.Contains(between, "마지막문단") {
				t.Errorf("각주가 본문 끝을 지나 뒤에 놓이지 않았습니다:\n%s", out)
			}
		})
	}
}

// Carrying the words of a nested list is not the same as carrying the list. A
// format that flattened three levels into one would pass every phrase check
// above and still turn an outline into a heap.
func TestNestedListsKeepTheirLevels(t *testing.T) {
	raw := json.RawMessage(everyNode(t))
	for _, format := range []struct {
		name   string
		render func() string
	}{
		{"Markdown", func() string { return renderMarkdown("문서 제목", raw) }},
		{"plain text", func() string { return renderPlainText("문서 제목", raw) }},
	} {
		t.Run(format.name, func(t *testing.T) {
			out := format.render()
			outer, inner, third := listIndent(out, "글머리항목"), listIndent(out, "하위항목"), listIndent(out, "셋째항목")
			if outer < 0 || inner < 0 || third < 0 {
				t.Fatalf("항목을 찾지 못했습니다: %d %d %d", outer, inner, third)
			}
			if !(outer < inner && inner < third) {
				t.Errorf("들여쓰기가 깊어지지 않습니다: %d, %d, %d", outer, inner, third)
			}
		})
	}
	// HTML says it with structure rather than spaces: a list inside a list.
	if html := renderHTML(raw); !strings.Contains(html, "<ul>") || strings.Count(html, "<ul>") < 3 {
		t.Errorf("HTML 목록이 평평해졌습니다: <ul> %d개", strings.Count(html, "<ul>"))
	}
}

// listIndent reports how far in the line carrying a phrase is set, or -1 if
// no line carries it. The counting is the Markdown importer's own, so the two
// sides of the round trip agree on what a level of indentation is.
func listIndent(rendered, phrase string) int {
	for _, line := range strings.Split(rendered, "\n") {
		if strings.Contains(line, phrase) {
			return indentOf(line)
		}
	}
	return -1
}

// A cell's vertical alignment is read out of Word files and written back, so
// the HTML the browser prints to PDF has to carry it too — otherwise the
// document muni shows and the document muni prints disagree.
func TestCellAlignmentReachesTheHTML(t *testing.T) {
	raw := json.RawMessage(everyNode(t))
	html := renderHTML(raw)
	for _, want := range []string{"vertical-align:top", "vertical-align:bottom"} {
		if !strings.Contains(html, want) {
			t.Errorf("%q 가 HTML 에 없습니다", want)
		}
	}
	// And nothing an imported file made up reaches the stylesheet.
	odd := json.RawMessage(`{"type":"doc","content":[{"type":"table","content":[{"type":"tableRow","content":[
		{"type":"tableCell","attrs":{"verticalAlign":"expression(alert(1))"},"content":[{"type":"paragraph"}]}]}]}]}`)
	if out := renderHTML(odd); strings.Contains(out, "expression") {
		t.Errorf("정체 모를 값이 스타일에 들어갔습니다: %s", out)
	}
}

// A diagram is an ordinary code block whose language says mermaid, which is
// what lets it survive every path muni already has. In HTML it is marked for
// the browser to draw; everywhere else it stays the text it is, because what a
// diagram says is what it is.
func TestADiagramIsMarkedForDrawingInHTMLAndStaysTextElsewhere(t *testing.T) {
	raw := json.RawMessage(everyNode(t))

	rendered := renderHTML(raw)
	if !strings.Contains(rendered, `<pre class="mermaid"`) {
		t.Error("HTML 에 그릴 표시가 없습니다")
	}
	if !htmlHasDiagram(rendered) {
		t.Error("그릴 것이 있는지 묻는 검사가 자기가 쓴 표시를 못 찾습니다")
	}
	// The description survives as text, so a reader without the drawing
	// library sees what the diagram says rather than nothing.
	if !strings.Contains(rendered, "graph TD") {
		t.Error("그림의 설명이 사라졌습니다")
	}

	for name, out := range map[string]string{
		"Markdown":   renderMarkdown("문서 제목", raw),
		"plain text": renderPlainText("문서 제목", raw),
	} {
		if !strings.Contains(out, "graph TD") {
			t.Errorf("%s 에서 그림의 설명이 사라졌습니다", name)
		}
	}
}

// A document with nothing to draw must not carry three and a half megabytes of
// drawing library.
func TestADocumentWithNoDiagramCarriesNoDrawingLibrary(t *testing.T) {
	plain := renderHTML(json.RawMessage(
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"그림 없음"}]}]}`))
	if htmlHasDiagram(plain) {
		t.Error("그림이 없는 문서를 그릴 것이 있다고 봤습니다")
	}
	page := fullHTMLWithDrawing("문서", false, plain, htmlHasDiagram(plain))
	if strings.Contains(page, "muniDiagramsReady") {
		t.Error("그림이 없는데 그리기 준비 코드가 들어갔습니다")
	}
}

// muni's HTML import is built to read muni's HTML export back. A diagram
// exported without its language returns as a code block that is no longer a
// diagram.
func TestADiagramSurvivesItsOwnHTMLExport(t *testing.T) {
	rendered := renderHTML(json.RawMessage(everyNode(t)))
	content, _, err := htmlDocument([]byte(rendered))
	if err != nil {
		t.Fatal(err)
	}
	document, err := richdoc.Parse(content)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	var walk func(*richdoc.Node)
	walk = func(node *richdoc.Node) {
		if node == nil {
			return
		}
		if isDiagram(node) {
			found = true
		}
		for _, child := range node.Content {
			walk(child)
		}
	}
	walk(document)
	if !found {
		t.Errorf("내보낸 HTML 을 다시 읽으니 도형이 아닙니다: %s", content)
	}
}
