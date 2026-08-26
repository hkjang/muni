package richdoc

import "testing"

func numbersFor(t *testing.T, raw, scheme string) []string {
	t.Helper()
	doc := parseDoc(t, raw)
	return HeadingNumbers(Headings(doc), scheme)
}

const nestedHeadings = `{"type":"doc","content":[
	{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"추진 배경"}]},
	{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"현황"}]},
	{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"문제점"}]},
	{"type":"heading","attrs":{"level":3},"content":[{"type":"text","text":"세부"}]},
	{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"추진 계획"}]}
]}`

func TestDecimalNumbering(t *testing.T) {
	got := numbersFor(t, nestedHeadings, NumberingDecimal)
	want := []string{"1.", "1.1.", "1.2.", "1.2.1.", "2."}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("numbers = %v, want %v", got, want)
		}
	}
}

func TestKoreanNumbering(t *testing.T) {
	got := numbersFor(t, nestedHeadings, NumberingKorean)
	want := []string{"I.", "1.", "2.", "가.", "II."}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("numbers = %v, want %v", got, want)
		}
	}
}

func TestNumberingRestartsAfterComingBackOut(t *testing.T) {
	// The second chapter's first section must be 2.1, not 2.3.
	got := numbersFor(t, nestedHeadings, NumberingDecimal)
	if got[4] != "2." {
		t.Fatalf("chapter number = %q", got[4])
	}
	raw := `{"type":"doc","content":[
		{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"1장"}]},
		{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"1.1"}]},
		{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"2장"}]},
		{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"2.1"}]}
	]}`
	again := numbersFor(t, raw, NumberingDecimal)
	if again[3] != "2.1." {
		t.Fatalf("section number after returning to the top level = %q", again[3])
	}
}

func TestNumberingCountsFromTheShallowestLevelUsed(t *testing.T) {
	raw := `{"type":"doc","content":[
		{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"첫 절"}]},
		{"type":"heading","attrs":{"level":3},"content":[{"type":"text","text":"세부"}]}
	]}`
	got := numbersFor(t, raw, NumberingDecimal)
	if got[0] != "1." || got[1] != "1.1." {
		t.Fatalf("a document written in h2 and h3 should number from the top: %v", got)
	}
}

func TestNoNumberingLeavesEveryLabelEmpty(t *testing.T) {
	for _, scheme := range []string{NumberingNone, "", "nonsense"} {
		got := numbersFor(t, nestedHeadings, scheme)
		for _, label := range got {
			if label != "" {
				t.Fatalf("scheme %q produced %q", scheme, label)
			}
		}
	}
}

func TestWithHeadingNumbersWritesTheLabelIntoTheExport(t *testing.T) {
	doc := parseDoc(t, nestedHeadings)
	numbered := WithHeadingNumbers(doc, NumberingKorean)
	if got := numbered.Content[0].PlainText(); got != "I. 추진 배경" {
		t.Fatalf("first heading = %q", got)
	}
	if got := numbered.Content[3].PlainText(); got != "가. 세부" {
		t.Fatalf("third level heading = %q", got)
	}
}

func TestWithHeadingNumbersLeavesTheStoredDocumentAlone(t *testing.T) {
	doc := parseDoc(t, nestedHeadings)
	_ = WithHeadingNumbers(doc, NumberingDecimal)
	// A number written into the document would be wrong the moment a section
	// moved, so the stored copy must be untouched.
	if got := doc.Content[0].PlainText(); got != "추진 배경" {
		t.Fatalf("the stored heading was modified: %q", got)
	}
}

func TestNumberedHeadingsReachTheContentsList(t *testing.T) {
	raw := `{"type":"doc","content":[
		{"type":"tableOfContents"},
		{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"개요"}]}
	]}`
	doc := WithTableOfContents(WithHeadingNumbers(parseDoc(t, raw), NumberingDecimal))
	if got := doc.Content[0].PlainText(); got != "1. 개요" {
		t.Fatalf("contents entry = %q", got)
	}
}

func TestNumberingSkipsAnEmptyHeading(t *testing.T) {
	raw := `{"type":"doc","content":[
		{"type":"heading","attrs":{"level":1},"content":[]},
		{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"실제 제목"}]}
	]}`
	doc := WithHeadingNumbers(parseDoc(t, raw), NumberingDecimal)
	if got := doc.Content[1].PlainText(); got != "1. 실제 제목" {
		t.Fatalf("an empty heading should not take a number: %q", got)
	}
}
