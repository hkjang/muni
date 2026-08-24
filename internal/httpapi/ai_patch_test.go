package httpapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

func patchDoc(t *testing.T, raw string) *richdoc.Node {
	t.Helper()
	node, err := richdoc.Parse(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	return node
}

func TestPatchableBlocksNeedAStableAnchor(t *testing.T) {
	document := patchDoc(t, `{"type":"doc","content":[
		{"type":"heading","attrs":{"level":1,"blockId":"blk_h"},"content":[{"type":"text","text":"제목"}]},
		{"type":"paragraph","attrs":{"blockId":"blk_a"},"content":[{"type":"text","text":"본문"}]},
		{"type":"paragraph","content":[{"type":"text","text":"식별자 없는 문단"}]},
		{"type":"paragraph","attrs":{"blockId":"blk_empty"},"content":[]},
		{"type":"table","attrs":{"blockId":"blk_t"},"content":[]}
	]}`)
	blocks := patchableBlocks(document, nil)

	ids := make([]string, 0, len(blocks))
	for _, block := range blocks {
		ids = append(ids, block.id)
	}
	// Without an id there is nothing to anchor a proposal to; an empty block
	// has nothing to rewrite; a table is not rewritten as plain text.
	if len(ids) != 2 || ids[0] != "blk_h" || ids[1] != "blk_a" {
		t.Fatalf("unexpected blocks: %v", ids)
	}
}

func TestPatchableBlocksCanBeNarrowedToASelection(t *testing.T) {
	document := patchDoc(t, `{"type":"doc","content":[
		{"type":"paragraph","attrs":{"blockId":"blk_a"},"content":[{"type":"text","text":"하나"}]},
		{"type":"paragraph","attrs":{"blockId":"blk_b"},"content":[{"type":"text","text":"둘"}]}
	]}`)
	blocks := patchableBlocks(document, []string{"blk_b"})
	if len(blocks) != 1 || blocks[0].id != "blk_b" {
		t.Fatalf("selection was not honoured: %+v", blocks)
	}
}

func TestPatchableBlocksReachesInsideContainers(t *testing.T) {
	document := patchDoc(t, `{"type":"doc","content":[
		{"type":"bulletList","content":[
			{"type":"listItem","attrs":{"blockId":"blk_item"},"content":[
				{"type":"paragraph","attrs":{"blockId":"blk_p"},"content":[{"type":"text","text":"항목"}]}]}]},
		{"type":"blockquote","content":[
			{"type":"paragraph","attrs":{"blockId":"blk_q"},"content":[{"type":"text","text":"인용"}]}]}
	]}`)
	blocks := patchableBlocks(document, nil)
	ids := map[string]bool{}
	for _, block := range blocks {
		ids[block.id] = true
	}
	if !ids["blk_p"] || !ids["blk_q"] {
		t.Fatalf("blocks inside containers were skipped: %+v", blocks)
	}
}

func TestParsePatchEdits(t *testing.T) {
	cases := []struct {
		name   string
		answer string
		want   int
	}{
		{"plain json", `{"edits":[{"blockId":"blk_a","newText":"고침","reason":"명확하게"}]}`, 1},
		{"fenced json", "```json\n{\"edits\":[{\"blockId\":\"blk_a\",\"newText\":\"고침\"}]}\n```", 1},
		{"prose around it", "다음과 같이 제안합니다.\n{\"edits\":[{\"blockId\":\"blk_a\",\"newText\":\"고침\"}]}\n이상입니다.", 1},
		{"nothing to change", `{"edits":[]}`, 0},
	}
	for _, item := range cases {
		edits, err := parsePatchEdits(item.answer)
		if err != nil {
			t.Errorf("%s: %v", item.name, err)
			continue
		}
		if len(edits) != item.want {
			t.Errorf("%s: got %d edits, want %d", item.name, len(edits), item.want)
		}
	}
}

func TestParsePatchEditsRejectsANonAnswer(t *testing.T) {
	for _, answer := range []string{"", "죄송하지만 도와드릴 수 없습니다", "```\nnot json\n```"} {
		if _, err := parsePatchEdits(answer); err == nil {
			t.Errorf("expected an error for %q", answer)
		}
	}
}

func TestParsePatchEditsKeepsTheReason(t *testing.T) {
	edits, err := parsePatchEdits(`{"edits":[{"blockId":"blk_a","newText":"새 문장","reason":"근거가 없어 보강"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if edits[0].Reason != "근거가 없어 보강" {
		t.Fatalf("the reason was dropped: %+v", edits[0])
	}
	if !strings.Contains(edits[0].NewText, "새 문장") {
		t.Fatalf("the replacement was dropped: %+v", edits[0])
	}
}
