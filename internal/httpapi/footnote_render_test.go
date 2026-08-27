package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
)

const footnoteDocument = `{"type":"doc","content":[
	{"type":"paragraph","content":[
		{"type":"text","text":"집행률은 92%였다"},
		{"type":"footnote","content":[{"type":"text","text":"기획조정실 집계."}]},
		{"type":"text","text":". 전년보다 높다"},
		{"type":"footnote","content":[{"type":"text","text":"2025년 87%."}]},
		{"type":"text","text":"."}]}]}`

// muni prints PDFs by rendering this HTML in a browser, and that browser does
// not implement the CSS that would place a note on the page its reference
// landed on. The notes are collected at the end instead, with a link back —
// which is what the document says it is doing, rather than silently dropping
// them or pretending they are per-page.
func TestFootnotesAreCollectedAtTheEndWithLinksBothWays(t *testing.T) {
	out := renderHTML(json.RawMessage(footnoteDocument))

	if !strings.Contains(out, `id="muni-note-ref-1"`) || !strings.Contains(out, `href="#muni-note-1"`) {
		t.Errorf("첫 각주의 참조와 링크가 없습니다:\n%s", out)
	}
	if !strings.Contains(out, `id="muni-note-1"`) || !strings.Contains(out, `href="#muni-note-ref-1"`) {
		t.Errorf("첫 각주 항목과 되돌아가는 링크가 없습니다:\n%s", out)
	}
	// The note's own words belong at the bottom, not in the sentence.
	sentenceEnd := strings.Index(out, `<hr class="muni-footnote-rule">`)
	if sentenceEnd < 0 {
		t.Fatalf("각주 구분선이 없습니다:\n%s", out)
	}
	if strings.Contains(out[:sentenceEnd], "기획조정실 집계") {
		t.Error("각주 내용이 본문 안에 있습니다")
	}
	if !strings.Contains(out[sentenceEnd:], "기획조정실 집계") {
		t.Error("각주 내용이 목록에 없습니다")
	}
}

func TestNotesAreNumberedInReadingOrder(t *testing.T) {
	out := renderHTML(json.RawMessage(footnoteDocument))
	first := strings.Index(out, `id="muni-note-ref-1"`)
	second := strings.Index(out, `id="muni-note-ref-2"`)
	if first < 0 || second < 0 || second < first {
		t.Fatalf("번호가 읽는 순서대로 붙지 않았습니다:\n%s", out)
	}
}

func TestADocumentWithoutNotesGetsNoFootnoteSection(t *testing.T) {
	// An empty rule and an empty list at the bottom of every document would
	// read as a bug.
	out := renderHTML(json.RawMessage(
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"본문"}]}]}`))
	if strings.Contains(out, "muni-footnote") {
		t.Errorf("각주가 없는데 각주 구역이 생겼습니다:\n%s", out)
	}
}

func TestRenderingTwiceGivesTheSameNumbers(t *testing.T) {
	// The numbers used to come from a counter the renderer held. Shared across
	// concurrent exports, that is a number belonging to whichever document got
	// there first.
	first := renderHTML(json.RawMessage(footnoteDocument))
	second := renderHTML(json.RawMessage(footnoteDocument))
	if first != second {
		t.Error("같은 문서를 두 번 렌더링했는데 결과가 다릅니다")
	}
}

// Every export path had to be taught what a footnote is. The one that had not
// been spliced the note's words into the middle of the sentence it annotated —
// "집행률은 92%였다기획조정실 집계.." — which is what an exporter does with a
// node it does not recognise.
func TestEveryFormatKeepsTheNoteOutOfTheSentence(t *testing.T) {
	raw := json.RawMessage(footnoteDocument)
	for _, format := range []struct {
		name   string
		render func() string
		marker string
	}{
		{"markdown", func() string { return renderMarkdown("각주 문서", raw) }, "[^1]"},
		{"plain text", func() string { return renderPlainText("각주 문서", raw) }, "[1]"},
	} {
		t.Run(format.name, func(t *testing.T) {
			out := format.render()
			if !strings.Contains(out, "집행률은 92%였다"+format.marker) {
				t.Errorf("참조가 문장 뒤에 붙지 않았습니다:\n%s", out)
			}
			// The note's words appear once, after the body, not inside it.
			body := out[:strings.Index(out, format.marker)]
			if strings.Contains(body, "기획조정실 집계") {
				t.Errorf("각주 내용이 본문에 섞였습니다:\n%s", out)
			}
			if !strings.Contains(out, "기획조정실 집계") {
				t.Errorf("각주 내용이 어디에도 없습니다:\n%s", out)
			}
		})
	}
}

func TestMarkdownUsesTheFormOtherReadersUnderstand(t *testing.T) {
	out := renderMarkdown("각주 문서", json.RawMessage(footnoteDocument))
	if !strings.Contains(out, "\n[^1]: 기획조정실 집계.") {
		t.Errorf("GFM 각주 정의 형식이 아닙니다:\n%s", out)
	}
}
