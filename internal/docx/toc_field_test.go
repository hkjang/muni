package docx

import (
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

// Word writes a table of contents as a field wrapped around the entries it
// last calculated. Those entries carry page numbers, and muni has no pages
// until a document is printed — importing them as prose gives a document a
// frozen list that will never be right again. muni's contents node is built
// from the headings instead.

func blockTypes(doc *richdoc.Node) []string {
	types := make([]string, 0, len(doc.Content))
	for _, block := range doc.Content {
		types = append(types, block.Type)
	}
	return types
}

// The shape Word actually writes: one paragraph opens the field, one paragraph
// per cached entry follows, and one closes it.
func TestAWordContentsFieldBecomesAContentsNode(t *testing.T) {
	body := wordParagraph(wordFieldChar("begin"), wordInstruction(` TOC \o "1-3" \h \z \u `), wordFieldChar("separate")) +
		wordParagraph(wordText("제1장 총칙\t1")) +
		wordParagraph(wordText("제1절 목적\t1")) +
		wordParagraph(wordFieldChar("end")) +
		wordParagraph(wordText("본문입니다"))

	document, _, _, err := Parse(wordPackage(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if got := blockTypes(document); len(got) != 2 || got[0] != richdoc.TableOfContentsType || got[1] != "paragraph" {
		t.Fatalf("블록 = %v", got)
	}
	// The cached entries and their page numbers are gone.
	if text := document.PlainText(); strings.Contains(text, "제1장 총칙") || strings.Contains(text, "제1절 목적") {
		t.Errorf("굳어버린 목차가 본문에 남았습니다: %q", text)
	}
	if text := document.PlainText(); !strings.Contains(text, "본문입니다") {
		t.Errorf("목차 뒤 본문이 사라졌습니다: %q", text)
	}
}

// Some producers put the whole field in one paragraph. Nothing follows it to
// skip, so the node must not swallow the rest of the document.
func TestAContentsFieldInOneParagraphEndsThere(t *testing.T) {
	body := wordParagraph(
		wordFieldChar("begin"), wordInstruction(" TOC "), wordFieldChar("separate"),
		wordText("제1장 총칙\t1"), wordFieldChar("end")) +
		wordParagraph(wordText("본문입니다"))

	document, _, _, err := Parse(wordPackage(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if got := blockTypes(document); len(got) != 2 || got[0] != richdoc.TableOfContentsType || got[1] != "paragraph" {
		t.Fatalf("블록 = %v", got)
	}
	if text := document.PlainText(); !strings.Contains(text, "본문입니다") {
		t.Errorf("본문이 사라졌습니다: %q", text)
	}
}

// Every other field stays what it was. A PAGEREF or a HYPERLINK inside a
// sentence must not take the sentence with it.
func TestOtherFieldsAreLeftAlone(t *testing.T) {
	body := wordParagraph(
		wordText("앞말 "),
		wordFieldChar("begin"), wordInstruction(" PAGEREF _Ref1 \\h "), wordFieldChar("separate"),
		wordText("3"), wordFieldChar("end"),
		wordText(" 뒷말"))

	document, _, _, err := Parse(wordPackage(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if got := blockTypes(document); len(got) != 1 || got[0] != "paragraph" {
		t.Fatalf("블록 = %v", got)
	}
	text := document.PlainText()
	for _, want := range []string{"앞말", "뒷말"} {
		if !strings.Contains(text, want) {
			t.Errorf("%q 가 사라졌습니다: %q", want, text)
		}
	}
}

// A contents field with nothing after it must not leave the reader inside a
// field that never closes.
func TestAnUnclosedContentsFieldDoesNotEatTheDocument(t *testing.T) {
	body := wordParagraph(wordFieldChar("begin"), wordInstruction(" TOC \\h "), wordFieldChar("separate")) +
		wordParagraph(wordText("제1장 총칙\t1")) +
		wordParagraph(wordText("본문입니다"))

	document, _, _, err := Parse(wordPackage(t, body))
	if err != nil {
		t.Fatal(err)
	}
	// Word would close the field; a file that does not is malformed, and the
	// entries after it are indistinguishable from the cache. What matters is
	// that Parse returns rather than losing itself.
	if len(document.Content) == 0 {
		t.Fatal("문서가 비었습니다")
	}
	if document.Content[0].Type != richdoc.TableOfContentsType {
		t.Errorf("블록 = %v", blockTypes(document))
	}
}

// A table of contents laid out in a table — not rare in Korean report
// templates — closes its field inside a cell, which the block walker descends
// into separately. A running "am I still inside it" flag never sees that end
// and drops every remaining paragraph: the import succeeds, and the document
// stops at its table of contents.
func TestAContentsFieldClosingElsewhereDoesNotEatTheDocument(t *testing.T) {
	body := wordParagraph(wordFieldChar("begin"), wordInstruction(" TOC \\h "), wordFieldChar("separate")) +
		`<w:tbl><w:tblPr/><w:tr><w:tc><w:tcPr/>` + wordParagraph(wordFieldChar("end")) + `</w:tc></w:tr></w:tbl>` +
		wordParagraph(wordText("목차 뒤 본문입니다")) +
		wordParagraph(wordText("그리고 더 많은 본문"))

	document, _, _, err := Parse(wordPackage(t, body))
	if err != nil {
		t.Fatal(err)
	}
	text := document.PlainText()
	for _, want := range []string{"목차 뒤 본문입니다", "그리고 더 많은 본문"} {
		if !strings.Contains(text, want) {
			t.Errorf("%q 가 사라졌습니다: %q", want, text)
		}
	}
	if document.Content[0].Type != richdoc.TableOfContentsType {
		t.Errorf("블록 = %v", blockTypes(document))
	}
}
