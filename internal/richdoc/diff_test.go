package richdoc

import (
	"encoding/json"
	"strings"
	"testing"
)

func doc(t *testing.T, raw string) *Node {
	t.Helper()
	node, err := Parse(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	return node
}

func paragraphJSON(id, text string) string {
	attrs := ""
	if id != "" {
		attrs = `"attrs":{"blockId":"` + id + `"},`
	}
	return `{"type":"paragraph",` + attrs + `"content":[{"type":"text","text":"` + text + `"}]}`
}

func statuses(result DiffResult) []string {
	out := make([]string, 0, len(result.Blocks))
	for _, block := range result.Blocks {
		out = append(out, block.Status)
	}
	return out
}

func TestDiffReportsAnUnchangedDocument(t *testing.T) {
	raw := `{"type":"doc","content":[` + paragraphJSON("blk_a", "첫 문단") + `,` + paragraphJSON("blk_b", "둘째 문단") + `]}`
	result := Diff(doc(t, raw), doc(t, raw))
	if result.Summary != (DiffSummary{Unchanged: 2}) {
		t.Fatalf("summary = %+v", result.Summary)
	}
}

// The whole point of stable ids: inserting a paragraph must not make every
// block below it look changed.
func TestInsertingAParagraphLeavesTheRestUnchanged(t *testing.T) {
	before := `{"type":"doc","content":[` + paragraphJSON("blk_a", "첫 문단") + `,` + paragraphJSON("blk_b", "둘째 문단") + `]}`
	after := `{"type":"doc","content":[` + paragraphJSON("blk_a", "첫 문단") + `,` +
		paragraphJSON("blk_new", "새 문단") + `,` + paragraphJSON("blk_b", "둘째 문단") + `]}`
	result := Diff(doc(t, before), doc(t, after))
	if result.Summary.Added != 1 || result.Summary.Unchanged != 2 {
		t.Fatalf("summary = %+v, statuses = %v", result.Summary, statuses(result))
	}
	if result.Summary.Changed != 0 || result.Summary.Moved != 0 {
		t.Fatalf("an insert should not disturb its neighbours: %+v", result.Summary)
	}
}

func TestDiffDetectsAChangedBlockWithInlineDetail(t *testing.T) {
	before := `{"type":"doc","content":[` + paragraphJSON("blk_a", "예산은 2억원입니다") + `]}`
	after := `{"type":"doc","content":[` + paragraphJSON("blk_a", "예산은 2.5억원입니다") + `]}`
	result := Diff(doc(t, before), doc(t, after))
	if result.Summary.Changed != 1 {
		t.Fatalf("summary = %+v", result.Summary)
	}
	block := result.Blocks[0]
	if len(block.Inline) == 0 {
		t.Fatal("a changed block should carry an inline breakdown")
	}
	var equal, inserted, deleted strings.Builder
	for _, op := range block.Inline {
		switch op.Op {
		case "equal":
			equal.WriteString(op.Text)
		case "insert":
			inserted.WriteString(op.Text)
		case "delete":
			deleted.WriteString(op.Text)
		}
	}
	if !strings.Contains(equal.String(), "예산은") {
		t.Errorf("the unchanged words were not recognised: %+v", block.Inline)
	}
	if !strings.Contains(inserted.String(), ".5") && !strings.Contains(inserted.String(), "2.5") {
		t.Errorf("the inserted text was not reported: %+v", block.Inline)
	}
	// "2억원" to "2.5억원" is a pure insertion, so there is nothing to delete;
	// reporting one would be noise.
	if deleted.String() != "" {
		t.Errorf("nothing was replaced here: %+v", block.Inline)
	}
}

func TestInlineDiffReportsReplacedText(t *testing.T) {
	before := `{"type":"doc","content":[` + paragraphJSON("blk_a", "일정은 3월에 시작합니다") + `]}`
	after := `{"type":"doc","content":[` + paragraphJSON("blk_a", "일정은 5월에 시작합니다") + `]}`
	result := Diff(doc(t, before), doc(t, after))
	var deleted, inserted strings.Builder
	for _, op := range result.Blocks[0].Inline {
		switch op.Op {
		case "delete":
			deleted.WriteString(op.Text)
		case "insert":
			inserted.WriteString(op.Text)
		}
	}
	if !strings.Contains(deleted.String(), "3") {
		t.Errorf("the replaced text was not reported: %+v", result.Blocks[0].Inline)
	}
	if !strings.Contains(inserted.String(), "5") {
		t.Errorf("the new text was not reported: %+v", result.Blocks[0].Inline)
	}
}

func TestDiffDetectsRemoval(t *testing.T) {
	before := `{"type":"doc","content":[` + paragraphJSON("blk_a", "남는 문단") + `,` + paragraphJSON("blk_b", "지울 문단") + `]}`
	after := `{"type":"doc","content":[` + paragraphJSON("blk_a", "남는 문단") + `]}`
	result := Diff(doc(t, before), doc(t, after))
	if result.Summary.Removed != 1 || result.Summary.Unchanged != 1 {
		t.Fatalf("summary = %+v, statuses = %v", result.Summary, statuses(result))
	}
	removed := result.Blocks[len(result.Blocks)-1]
	if removed.Status != BlockRemoved || removed.Before != "지울 문단" || removed.ToIndex != -1 {
		t.Fatalf("unexpected removal entry: %+v", removed)
	}
}

func TestDiffDetectsAMove(t *testing.T) {
	before := `{"type":"doc","content":[` + paragraphJSON("blk_a", "하나") + `,` +
		paragraphJSON("blk_b", "둘") + `,` + paragraphJSON("blk_c", "셋") + `]}`
	after := `{"type":"doc","content":[` + paragraphJSON("blk_c", "셋") + `,` +
		paragraphJSON("blk_a", "하나") + `,` + paragraphJSON("blk_b", "둘") + `]}`
	result := Diff(doc(t, before), doc(t, after))
	if result.Summary.Moved != 1 {
		t.Fatalf("expected exactly one block to be reported as moved: %+v %v", result.Summary, statuses(result))
	}
	if result.Summary.Changed != 0 || result.Summary.Added != 0 || result.Summary.Removed != 0 {
		t.Fatalf("a reorder is not an edit: %+v", result.Summary)
	}
}

// Documents written before block ids existed still have to diff usefully.
func TestDiffFallsBackToSimilarityWithoutIDs(t *testing.T) {
	before := `{"type":"doc","content":[` + paragraphJSON("", "이 문단은 예산 2억원을 설명합니다") + `]}`
	after := `{"type":"doc","content":[` + paragraphJSON("", "이 문단은 예산 3억원을 설명합니다") + `]}`
	result := Diff(doc(t, before), doc(t, after))
	if result.Summary.Changed != 1 {
		t.Fatalf("an edited paragraph should read as a change, not a delete and an insert: %+v", result.Summary)
	}
}

func TestDiffKeepsUnrelatedBlocksApart(t *testing.T) {
	before := `{"type":"doc","content":[` + paragraphJSON("", "완전히 다른 내용입니다") + `]}`
	after := `{"type":"doc","content":[` + paragraphJSON("", "전혀 관계없는 문장") + `]}`
	result := Diff(doc(t, before), doc(t, after))
	if result.Summary.Changed != 0 || result.Summary.Added != 1 || result.Summary.Removed != 1 {
		t.Fatalf("unrelated blocks should not be paired: %+v", result.Summary)
	}
}

func TestDiffComparesHeadingsTablesAndImages(t *testing.T) {
	before := `{"type":"doc","content":[
		{"type":"heading","attrs":{"level":1,"blockId":"blk_h"},"content":[{"type":"text","text":"제목"}]},
		{"type":"table","attrs":{"blockId":"blk_t"},"content":[
			{"type":"tableRow","content":[
				{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"속도"}]}]},
				{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"12.5"}]}]}]}]}
	]}`
	after := `{"type":"doc","content":[
		{"type":"heading","attrs":{"level":1,"blockId":"blk_h"},"content":[{"type":"text","text":"새 제목"}]},
		{"type":"table","attrs":{"blockId":"blk_t"},"content":[
			{"type":"tableRow","content":[
				{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"속도"}]}]},
				{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"13.1"}]}]}]}]}
	]}`
	result := Diff(doc(t, before), doc(t, after))
	if result.Summary.Changed != 2 {
		t.Fatalf("both the heading and the table changed: %+v %v", result.Summary, statuses(result))
	}
	for _, block := range result.Blocks {
		if block.Type == "table" && !strings.Contains(block.After, "13.1") {
			t.Errorf("the table cell change was not captured: %+v", block)
		}
	}
}

func TestInlineDiffOnKoreanTextWorksCharacterByCharacter(t *testing.T) {
	ops := InlineDiff("계획을 수립한다", "계획을 검토한다")
	var equal strings.Builder
	for _, op := range ops {
		if op.Op == "equal" {
			equal.WriteString(op.Text)
		}
	}
	if !strings.Contains(equal.String(), "계획을") || !strings.Contains(equal.String(), "한다") {
		t.Fatalf("Korean text should share its unchanged characters: %+v", ops)
	}
}

func TestInlineDiffOfIdenticalTextIsAllEqual(t *testing.T) {
	ops := InlineDiff("같은 문장", "같은 문장")
	if len(ops) != 1 || ops[0].Op != "equal" {
		t.Fatalf("identical text should produce a single equal run: %+v", ops)
	}
}

func TestDiffHandlesAnEmptyDocument(t *testing.T) {
	empty := `{"type":"doc","content":[]}`
	filled := `{"type":"doc","content":[` + paragraphJSON("blk_a", "내용") + `]}`
	if result := Diff(doc(t, empty), doc(t, filled)); result.Summary.Added != 1 {
		t.Fatalf("summary = %+v", result.Summary)
	}
	if result := Diff(doc(t, filled), doc(t, empty)); result.Summary.Removed != 1 {
		t.Fatalf("summary = %+v", result.Summary)
	}
}
