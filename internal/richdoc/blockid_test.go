package richdoc

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func parseDoc(t *testing.T, raw string) *Node {
	t.Helper()
	node, err := Parse(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	return node
}

func blockIDs(node *Node) []string {
	out := []string{}
	var walk func(*Node)
	walk = func(current *Node) {
		if value, ok := current.Attr(BlockIDAttr).(string); ok && value != "" {
			out = append(out, value)
		}
		for _, child := range current.Content {
			walk(child)
		}
	}
	walk(node)
	return out
}

func TestNewBlockIDIsUniqueAndSortable(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	seen := map[string]bool{}
	for index := 0; index < 500; index++ {
		id := NewBlockID(now)
		if !strings.HasPrefix(id, "blk_") {
			t.Fatalf("unexpected id shape: %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate id: %q", id)
		}
		seen[id] = true
	}
	earlier := NewBlockID(now)
	later := NewBlockID(now.Add(time.Hour))
	if earlier[:13] >= later[:13] {
		t.Fatalf("ids are not time ordered: %q vs %q", earlier, later)
	}
}

func TestAssignBlockIDsStampsAnchorableBlocks(t *testing.T) {
	document := parseDoc(t, `{"type":"doc","content":[
		{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"제목"}]},
		{"type":"paragraph","content":[{"type":"text","text":"본문"}]},
		{"type":"bulletList","content":[
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"항목"}]}]}]}
	]}`)
	assigned := AssignBlockIDs(document, time.Now())
	// heading, paragraph, listItem and the paragraph inside it.
	if assigned != 4 {
		t.Fatalf("assigned = %d, want 4", assigned)
	}
	ids := blockIDs(document)
	if len(ids) != 4 {
		t.Fatalf("ids = %v", ids)
	}
	unique := map[string]bool{}
	for _, id := range ids {
		unique[id] = true
	}
	if len(unique) != len(ids) {
		t.Fatalf("ids are not unique: %v", ids)
	}
	// The container itself is not anchorable.
	if value := document.Content[2].Attr(BlockIDAttr); value != nil {
		t.Errorf("bulletList should not be stamped, got %v", value)
	}
}

func TestAssignBlockIDsIsIdempotent(t *testing.T) {
	document := parseDoc(t, `{"type":"doc","content":[
		{"type":"paragraph","content":[{"type":"text","text":"본문"}]}]}`)
	AssignBlockIDs(document, time.Now())
	before := blockIDs(document)
	if assigned := AssignBlockIDs(document, time.Now()); assigned != 0 {
		t.Fatalf("second pass reassigned %d ids", assigned)
	}
	if after := blockIDs(document); after[0] != before[0] {
		t.Fatalf("id changed on the second pass: %q -> %q", before[0], after[0])
	}
}

func TestAssignBlockIDsReplacesDuplicates(t *testing.T) {
	document := parseDoc(t, `{"type":"doc","content":[
		{"type":"paragraph","attrs":{"blockId":"blk_same"},"content":[{"type":"text","text":"앞"}]},
		{"type":"paragraph","attrs":{"blockId":"blk_same"},"content":[{"type":"text","text":"뒤"}]}]}`)
	if assigned := AssignBlockIDs(document, time.Now()); assigned != 1 {
		t.Fatalf("assigned = %d, want 1", assigned)
	}
	ids := blockIDs(document)
	if ids[0] != "blk_same" {
		t.Errorf("the first block should keep its identity, got %q", ids[0])
	}
	if ids[1] == "blk_same" {
		t.Errorf("the duplicate was not replaced")
	}
}
