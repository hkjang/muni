package richdoc

import (
	"crypto/rand"
	"strconv"
	"strings"
	"time"
)

// BlockIDAttr is the attribute that carries a block's stable identity.
const BlockIDAttr = "blockId"

// anchorableTypes mirrors the editor's list: the blocks a comment, citation,
// AI patch or deep link can point at. Containers are left out because an
// anchor is only useful on something a reader can be taken to.
var anchorableTypes = map[string]bool{
	"paragraph":      true,
	"heading":        true,
	"blockquote":     true,
	"codeBlock":      true,
	"horizontalRule": true,
	"image":          true,
	"listItem":       true,
	"taskItem":       true,
	"table":          true,
}

const blockIDAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

// NewBlockID returns an identifier in the same shape the editor produces: a
// base36 timestamp followed by randomness, so ids sort by creation time and
// stay unique across collaborators.
func NewBlockID(now time.Time) string {
	stamp := strconv.FormatInt(now.UnixMilli(), 36)
	for len(stamp) < 9 {
		stamp = "0" + stamp
	}
	suffix := make([]byte, 10)
	if _, err := rand.Read(suffix); err != nil {
		// crypto/rand does not fail in practice; fall back to the timestamp so
		// the document still gets a usable identity.
		return "blk_" + stamp + strconv.FormatInt(now.UnixNano(), 36)
	}
	var out strings.Builder
	out.WriteString("blk_")
	out.WriteString(stamp)
	for _, value := range suffix {
		out.WriteByte(blockIDAlphabet[int(value)%len(blockIDAlphabet)])
	}
	return out.String()
}

// AssignBlockIDs gives every anchorable block a stable id, replacing ids that
// another block already claimed. Documents that arrive through import or the
// API have never been through the editor, so nothing has stamped them yet.
// The walk is idempotent: ids that are already present and unique are kept.
func AssignBlockIDs(node *Node, now time.Time) int {
	seen := map[string]bool{}
	assigned := 0
	var walk func(*Node)
	walk = func(current *Node) {
		if current == nil {
			return
		}
		if anchorableTypes[current.Type] {
			existing, _ := current.Attr(BlockIDAttr).(string)
			if existing == "" || seen[existing] {
				id := NewBlockID(now)
				for seen[id] {
					id = NewBlockID(now)
				}
				current.SetAttr(BlockIDAttr, id)
				seen[id] = true
				assigned++
			} else {
				seen[existing] = true
			}
		}
		for _, child := range current.Content {
			walk(child)
		}
	}
	walk(node)
	return assigned
}
