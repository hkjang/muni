package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
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
const everyNode = `{"type":"doc","content":[
	{"type":"tableOfContents"},
	{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"제목입니다"}]},
	{"type":"paragraph","content":[
		{"type":"text","text":"평문과 "},
		{"type":"text","marks":[{"type":"bold"}],"text":"굵은글씨"},
		{"type":"text","text":"와 "},
		{"type":"text","marks":[{"type":"italic"}],"text":"기울임"},
		{"type":"text","text":"와 "},
		{"type":"text","marks":[{"type":"underline"}],"text":"밑줄"},
		{"type":"text","text":"와 "},
		{"type":"text","marks":[{"type":"strike"}],"text":"취소선"},
		{"type":"text","text":"와 "},
		{"type":"text","marks":[{"type":"code"}],"text":"코드조각"},
		{"type":"text","text":"와 "},
		{"type":"text","marks":[{"type":"link","attrs":{"href":"https://example.com"}}],"text":"링크글자"},
		{"type":"text","text":"와 제곱미터"},
		{"type":"text","marks":[{"type":"superscript"}],"text":"위첨자표시"},
		{"type":"text","text":"와 화학식"},
		{"type":"text","marks":[{"type":"subscript"}],"text":"아래첨자표시"},
		{"type":"footnote","content":[{"type":"text","text":"각주내용입니다"}]},
		{"type":"text","text":"."},
		{"type":"hardBreak"},
		{"type":"text","text":"줄바꿈뒤문장"}]},
	{"type":"blockquote","content":[{"type":"paragraph","content":[{"type":"text","text":"인용문입니다"}]}]},
	{"type":"bulletList","content":[
		{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"글머리항목"}]}]}]},
	{"type":"orderedList","content":[
		{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"번호항목"}]}]}]},
	{"type":"taskList","content":[
		{"type":"taskItem","attrs":{"checked":true},"content":[{"type":"paragraph","content":[{"type":"text","text":"할일항목"}]}]}]},
	{"type":"codeBlock","content":[{"type":"text","text":"코드블록내용"}]},
	{"type":"table","content":[
		{"type":"tableRow","content":[
			{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"표머리글"}]}]},
			{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"표셀내용"}]}]}]}]},
	{"type":"horizontalRule"},
	{"type":"pageBreak"},
	{"type":"paragraph","content":[{"type":"text","text":"마지막문단"}]}]}`

// phrases every format has to carry through, whatever it does with the markup
// around them.
var carriedPhrases = []string{
	"제목입니다", "평문과", "굵은글씨", "기울임", "밑줄", "취소선", "코드조각",
	"링크글자", "위첨자표시", "아래첨자표시", "각주내용입니다", "줄바꿈뒤문장",
	"인용문입니다", "글머리항목", "번호항목", "할일항목", "코드블록내용",
	"표머리글", "표셀내용", "마지막문단",
}

func TestEveryExportFormatCarriesEveryKindOfContent(t *testing.T) {
	raw := json.RawMessage(everyNode)
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
	raw := json.RawMessage(everyNode)
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
