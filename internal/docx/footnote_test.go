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

// muni has one kind of note. An endnote and a footnote differ only in where
// they are printed, and muni already prints its notes at the end of a PDF.
// Reading 미주 as 각주 keeps the words and the place in the sentence they were
// attached to; dropping them, which is what muni did, keeps neither.
func TestAnEndnoteIsReadAsANote(t *testing.T) {
	body := `<w:p><w:r><w:t>문장입니다</w:t></w:r>` +
		`<w:r><w:endnoteReference w:id="2"/></w:r><w:r><w:t>.</w:t></w:r></w:p>`
	notes := `<w:endnote w:type="separator" w:id="0"><w:p><w:r><w:separator/></w:r></w:p></w:endnote>` +
		`<w:endnote w:id="2"><w:p><w:r><w:t>미주 내용입니다</w:t></w:r></w:p></w:endnote>`
	document, _, _, err := Parse(wordPackageWithNotes(t, body, "", notes))
	if err != nil {
		t.Fatal(err)
	}
	found := richdoc.Footnotes(document)
	if len(found) != 1 {
		t.Fatalf("주석 = %d개: %s", len(found), document.PlainText())
	}
	if text := richdoc.FootnoteText(found[0]); text != "미주 내용입니다" {
		t.Errorf("주석 내용 = %q", text)
	}
}

// The two files number themselves separately, so a footnote 2 and an endnote 2
// must not become each other.
func TestAFootnoteAndAnEndnoteWithTheSameNumberStayApart(t *testing.T) {
	body := `<w:p><w:r><w:t>앞</w:t></w:r><w:r><w:footnoteReference w:id="2"/></w:r>` +
		`<w:r><w:t>뒤</w:t></w:r><w:r><w:endnoteReference w:id="2"/></w:r></w:p>`
	footnotes := `<w:footnote w:id="2"><w:p><w:r><w:t>각주 쪽</w:t></w:r></w:p></w:footnote>`
	endnotes := `<w:endnote w:id="2"><w:p><w:r><w:t>미주 쪽</w:t></w:r></w:p></w:endnote>`
	document, _, _, err := Parse(wordPackageWithNotes(t, body, footnotes, endnotes))
	if err != nil {
		t.Fatal(err)
	}
	found := richdoc.Footnotes(document)
	if len(found) != 2 {
		t.Fatalf("주석 = %d개", len(found))
	}
	if got := richdoc.FootnoteText(found[0]); got != "각주 쪽" {
		t.Errorf("첫 주석 = %q", got)
	}
	if got := richdoc.FootnoteText(found[1]); got != "미주 쪽" {
		t.Errorf("둘째 주석 = %q", got)
	}
}

// A note is often more than one paragraph — a citation and then a remark about
// it. Reading the whole part as one string ran them together, so all the words
// were there and the sentence was gone.
func TestANoteKeepsItsLines(t *testing.T) {
	body := `<w:p><w:r><w:t>문장</w:t></w:r><w:r><w:footnoteReference w:id="2"/></w:r></w:p>`
	notes := `<w:footnote w:id="2">` +
		`<w:p><w:r><w:t>기획조정실(2026)</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>같은 자료 12쪽</w:t></w:r></w:p></w:footnote>`
	document, _, _, err := Parse(wordPackageWithNotes(t, body, notes, ""))
	if err != nil {
		t.Fatal(err)
	}
	found := richdoc.Footnotes(document)
	if len(found) != 1 {
		t.Fatalf("주석 = %d개", len(found))
	}
	breaks := 0
	for _, child := range found[0].Content {
		if child.Type == "hardBreak" {
			breaks++
		}
	}
	if breaks != 1 {
		t.Errorf("줄바꿈 = %d개: %q", breaks, richdoc.FootnoteText(found[0]))
	}
	// And the two lines have not run into each other.
	if text := richdoc.FootnoteText(found[0]); strings.Contains(text, ")같은") {
		t.Errorf("두 줄이 붙었습니다: %q", text)
	}
}

// And the two lines survive a round trip through Word, which writes the break
// as w:br and reads it back the same way.
func TestATwoLineNoteSurvivesTheRoundTrip(t *testing.T) {
	source := `{"type":"doc","content":[{"type":"paragraph","content":[
		{"type":"text","text":"문장"},
		{"type":"footnote","content":[{"type":"text","text":"기획조정실(2026)"},{"type":"hardBreak"},{"type":"text","text":"같은 자료 12쪽"}]},
		{"type":"text","text":"."}]}]}`
	node, err := richdoc.Parse(json.RawMessage(source))
	if err != nil {
		t.Fatal(err)
	}
	built, err := Build(node, Options{Title: "두 줄 주석"})
	if err != nil {
		t.Fatal(err)
	}
	back, _, _, err := Parse(built)
	if err != nil {
		t.Fatal(err)
	}
	found := richdoc.Footnotes(back)
	if len(found) != 1 {
		t.Fatalf("주석 = %d개", len(found))
	}
	if text := richdoc.FootnoteText(found[0]); strings.Contains(text, ")같은") {
		t.Errorf("왕복에서 두 줄이 붙었습니다: %q", text)
	}
}
