package docx

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

func TestTheWordFileCarriesEveryKindOfContent(t *testing.T) {
	node, err := richdoc.Parse(json.RawMessage(everyNode(t)))
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
		"링크글자", "위첨자표시", "아래첨자표시", "형광펜표시", "글자서식표시",
		"줄바꿈뒤문장",
		"인용문입니다", "글머리항목", "하위항목", "셋째항목", "번호항목",
		"할일항목", "코드블록내용", "graph TD", "표머리글", "표셀내용", "마지막문단",
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
	node, err := richdoc.Parse(json.RawMessage(everyNode(t)))
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
		"위첨자표시", "아래첨자표시", "형광펜표시", "글자서식표시",
		"각주내용입니다", "줄바꿈뒤문장",
		"인용문입니다", "글머리항목", "하위항목", "셋째항목", "번호항목",
		"할일항목", "코드블록내용", "graph TD", "표머리글", "표셀내용", "마지막문단",
	} {
		if !strings.Contains(text, phrase) {
			t.Errorf("%q 가 왕복에서 사라졌습니다", phrase)
		}
	}
}

// Carrying the words is the floor. A round trip that keeps "형광펜표시" and
// drops the highlight has not kept the sentence — it has kept the text and
// thrown away what the writer meant by it.
func TestTheMarksComeBackWithTheWordsTheyWereOn(t *testing.T) {
	node, err := richdoc.Parse(json.RawMessage(everyNode(t)))
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
	for _, want := range []struct {
		phrase string
		mark   string
	}{
		{"굵은글씨", "bold"},
		{"기울임", "italic"},
		{"밑줄", "underline"},
		{"취소선", "strike"},
		{"링크글자", "link"},
		{"위첨자표시", "superscript"},
		{"아래첨자표시", "subscript"},
		{"형광펜표시", "highlight"},
		{"글자서식표시", "textStyle"},
	} {
		if !hasMarkOn(back, want.phrase, want.mark) {
			t.Errorf("%q 가 %s 없이 돌아왔습니다", want.phrase, want.mark)
		}
	}
}

// hasMarkOn reports whether the text carrying a phrase still wears a mark.
func hasMarkOn(node *richdoc.Node, phrase, mark string) bool {
	if node == nil {
		return false
	}
	if node.Type == "text" && strings.Contains(node.Text, phrase) {
		for _, m := range node.Marks {
			if m.Type == mark {
				return true
			}
		}
	}
	for _, child := range node.Content {
		if hasMarkOn(child, phrase, mark) {
			return true
		}
	}
	return false
}
