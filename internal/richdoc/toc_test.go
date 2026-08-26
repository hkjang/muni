package richdoc

import "testing"

func TestHeadingsIndentFromTheShallowestLevelUsed(t *testing.T) {
	doc := parseDoc(t, `{"type":"doc","content":[
		{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"첫 절"}]},
		{"type":"heading","attrs":{"level":3},"content":[{"type":"text","text":"세부"}]}
	]}`)
	headings := Headings(doc)
	if len(headings) != 2 {
		t.Fatalf("headings = %+v", headings)
	}
	// A document written in h2 and h3 must not be listed pushed to the right.
	if headings[0].Depth != 0 || headings[1].Depth != 1 {
		t.Fatalf("depths = %d, %d", headings[0].Depth, headings[1].Depth)
	}
}

func TestHeadingsIndentASkippedLevelByOneStep(t *testing.T) {
	doc := parseDoc(t, `{"type":"doc","content":[
		{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"제목"}]},
		{"type":"heading","attrs":{"level":4},"content":[{"type":"text","text":"깊은 제목"}]},
		{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"둘째 장"}]}
	]}`)
	depths := []int{}
	for _, heading := range Headings(doc) {
		depths = append(depths, heading.Depth)
	}
	if len(depths) != 3 || depths[0] != 0 || depths[1] != 1 || depths[2] != 0 {
		t.Fatalf("depths = %v", depths)
	}
}

func TestHeadingsLeaveOutEmptyOnes(t *testing.T) {
	doc := parseDoc(t, `{"type":"doc","content":[
		{"type":"heading","attrs":{"level":1},"content":[]},
		{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"실제 제목"}]}
	]}`)
	if headings := Headings(doc); len(headings) != 1 || headings[0].Text != "실제 제목" {
		t.Fatalf("headings = %+v", headings)
	}
}

func TestWithTableOfContentsFillsTheNode(t *testing.T) {
	doc := parseDoc(t, `{"type":"doc","content":[
		{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"개요"}]},
		{"type":"tableOfContents"},
		{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"배경"}]}
	]}`)
	filled := WithTableOfContents(doc)
	toc := filled.Content[1]
	if toc.Type != TableOfContentsType {
		t.Fatalf("node type = %q", toc.Type)
	}
	if len(toc.Content) != 2 {
		t.Fatalf("entries = %d", len(toc.Content))
	}
	if got := toc.Content[0].PlainText(); got != "개요" {
		t.Fatalf("first entry = %q", got)
	}
	// The second heading is a level deeper, so its entry is indented.
	if toc.Content[1].AttrInt("indent", 0) != 1 {
		t.Fatalf("second entry was not indented: %+v", toc.Content[1])
	}
}

func TestWithTableOfContentsLeavesTheStoredDocumentAlone(t *testing.T) {
	doc := parseDoc(t, `{"type":"doc","content":[
		{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"개요"}]},
		{"type":"tableOfContents"}
	]}`)
	_ = WithTableOfContents(doc)
	// Generating entries must not write them back: a stored list would go
	// stale the moment a heading changed.
	if len(doc.Content[1].Content) != 0 {
		t.Fatalf("the stored node was modified: %+v", doc.Content[1])
	}
}

func TestWithTableOfContentsSaysSoWhenThereAreNoHeadings(t *testing.T) {
	doc := parseDoc(t, `{"type":"doc","content":[
		{"type":"paragraph","content":[{"type":"text","text":"본문뿐입니다"}]},
		{"type":"tableOfContents"}
	]}`)
	filled := WithTableOfContents(doc)
	if text := filled.Content[1].PlainText(); text == "" {
		t.Fatal("an empty contents list should explain itself rather than vanish")
	}
}

func TestWithTableOfContentsIsUntouchedWithoutOne(t *testing.T) {
	doc := parseDoc(t, `{"type":"doc","content":[
		{"type":"paragraph","content":[{"type":"text","text":"본문"}]}
	]}`)
	if WithTableOfContents(doc) != doc {
		t.Fatal("a document with no contents node should be returned as it is")
	}
}
