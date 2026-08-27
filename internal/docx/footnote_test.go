package docx

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

const footnoteDoc = `{"type":"doc","content":[
	{"type":"paragraph","content":[
		{"type":"text","text":"예산 집행률은 92%였다"},
		{"type":"footnote","content":[{"type":"text","text":"기획조정실 자체 집계."}]},
		{"type":"text","text":". 전년보다 높다"},
		{"type":"footnote","content":[{"type":"text","text":"2025년 87%."}]},
		{"type":"text","text":"."}]}]}`

func footnoteRoundTrip(t *testing.T) *richdoc.Node {
	t.Helper()
	node, err := richdoc.Parse(json.RawMessage(footnoteDoc))
	if err != nil {
		t.Fatal(err)
	}
	built, err := Build(node, Options{Title: "각주"})
	if err != nil {
		t.Fatal(err)
	}
	back, _, _, err := Parse(built)
	if err != nil {
		t.Fatal(err)
	}
	return back
}

// A footnote is not part of the sentence it annotates. Before the exporter
// knew what one was, the default branch walked into the note and spliced its
// text into the middle of the sentence — which is exactly how a footnote reads
// when nobody has taught the exporter about it.
func TestAFootnoteStaysOutOfTheSentence(t *testing.T) {
	node, err := richdoc.Parse(json.RawMessage(footnoteDoc))
	if err != nil {
		t.Fatal(err)
	}
	built, err := Build(node, Options{Title: "각주"})
	if err != nil {
		t.Fatal(err)
	}
	body := documentXMLOf(t, built)
	if strings.Contains(body, "기획조정실 자체 집계") {
		t.Error("각주 내용이 본문에 섞였습니다")
	}
	if !strings.Contains(body, "w:footnoteReference") {
		t.Error("본문에 각주 참조가 없습니다")
	}
	if !packageHas(t, built, "word/footnotes.xml") {
		t.Error("word/footnotes.xml 이 없습니다")
	}
}

func TestFootnotesSurviveTheRoundTrip(t *testing.T) {
	notes := richdoc.Footnotes(footnoteRoundTrip(t))
	if len(notes) != 2 {
		t.Fatalf("각주 %d개가 돌아왔습니다", len(notes))
	}
	if got := richdoc.FootnoteText(notes[0]); got != "기획조정실 자체 집계." {
		t.Errorf("첫 각주 = %q", got)
	}
	if notes[1].Number != 2 {
		t.Errorf("둘째 각주 번호 = %d", notes[1].Number)
	}
}

func TestNotesAreNumberedByPositionNotByWhatTheFileSaid(t *testing.T) {
	// Word's ids start at 2 and are not the reader's numbers. A number carried
	// in from the file would be the old document's number, and wrong the
	// moment a paragraph moved.
	notes := richdoc.Footnotes(footnoteRoundTrip(t))
	for index, note := range notes {
		if note.Number != index+1 {
			t.Errorf("%d번째 각주의 번호가 %d입니다", index+1, note.Number)
		}
	}
}

func TestADocumentWithoutNotesHasNoFootnotePart(t *testing.T) {
	// A part nothing points at is one Word can call corrupt, and every
	// document muni has ever exported would otherwise carry an empty one.
	node, err := richdoc.Parse(json.RawMessage(
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"본문"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	built, err := Build(node, Options{Title: "각주 없음"})
	if err != nil {
		t.Fatal(err)
	}
	if packageHas(t, built, "word/footnotes.xml") {
		t.Error("각주가 없는데 파트가 쓰였습니다")
	}
	if strings.Contains(documentXMLOf(t, built), "footnoteReference") {
		t.Error("각주가 없는데 참조가 있습니다")
	}
}

func TestTheSeparatorEntriesAreNotReadAsNotes(t *testing.T) {
	// Ids 0 and 1 are the rules Word draws above the notes. Reading them as
	// notes would put two empty footnotes at the top of every imported
	// document.
	notes := richdoc.Footnotes(footnoteRoundTrip(t))
	for _, note := range notes {
		if strings.TrimSpace(richdoc.FootnoteText(note)) == "" {
			t.Error("빈 각주가 들어왔습니다")
		}
	}
}
