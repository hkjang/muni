package docx

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

// One document carrying every kind of block and inline the editor can make,
// each with a distinctive phrase, so a node the exporter does not recognise is
// obvious rather than quietly missing.
//
// The same mistake kept arriving in different clothes: an exporter meets a
// node it has not been taught about, walks into its children, and splices them
// where they do not belong. Footnotes did it, and it took four separate
// findings to notice. One document through the whole path finds the next one
// in a single run.
const everyNode = `{"type":"doc","content":[
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

func TestTheWordFileCarriesEveryKindOfContent(t *testing.T) {
	node, err := richdoc.Parse(json.RawMessage(everyNode))
	if err != nil {
		t.Fatal(err)
	}
	built, err := Build(node, Options{Title: "문서 제목"})
	if err != nil {
		t.Fatal(err)
	}
	body := documentXMLOf(t, built)

	// Everything except the footnote belongs in the body.
	for _, phrase := range []string{
		"제목입니다", "평문과", "굵은글씨", "기울임", "밑줄", "취소선",
		"링크글자", "위첨자표시", "아래첨자표시", "줄바꿈뒤문장",
		"인용문입니다", "글머리항목", "번호항목", "할일항목", "코드블록내용",
		"표머리글", "표셀내용", "마지막문단",
	} {
		if !strings.Contains(body, phrase) {
			t.Errorf("%q 가 .docx 본문에서 사라졌습니다", phrase)
		}
	}
	// And the footnote belongs in its own part, not in the sentence.
	if strings.Contains(body, "각주내용입니다") {
		t.Error("각주 내용이 본문에 섞였습니다")
	}
	notes := partOf(t, built, "word/footnotes.xml")
	if !strings.Contains(notes, "각주내용입니다") {
		t.Error("각주 내용이 word/footnotes.xml 에 없습니다")
	}
}

func TestEverythingComesBackWhenTheWordFileIsReadAgain(t *testing.T) {
	node, err := richdoc.Parse(json.RawMessage(everyNode))
	if err != nil {
		t.Fatal(err)
	}
	built, err := Build(node, Options{Title: "문서 제목"})
	if err != nil {
		t.Fatal(err)
	}
	back, _, _, err := Parse(built)
	if err != nil {
		t.Fatal(err)
	}
	text := back.PlainText()
	for _, phrase := range []string{
		"제목입니다", "굵은글씨", "기울임", "밑줄", "취소선", "링크글자",
		"위첨자표시", "아래첨자표시", "각주내용입니다", "줄바꿈뒤문장",
		"인용문입니다", "글머리항목", "번호항목", "할일항목", "코드블록내용",
		"표머리글", "표셀내용", "마지막문단",
	} {
		if !strings.Contains(text, phrase) {
			t.Errorf("%q 가 왕복에서 사라졌습니다", phrase)
		}
	}
}
