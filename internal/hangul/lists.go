package hangul

import "github.com/hkjang/muni/internal/richdoc"

// A list in a Hangul document has no beginning and no end. It is a run of
// paragraphs whose shape says "bullet" or "number", each at a depth, and
// the list muni needs — an element holding items — has to be put together
// from the run. ListStack holds the list open at each depth, so a deeper
// paragraph goes inside the item above it and a shallower one closes what
// was inside. Both readers, the binary one and the XML one, build lists
// this way.
type ListStack struct {
	open []openList
}

type openList struct {
	kind  string
	level int
	node  *richdoc.Node
}

// Close ends every open list: what comes next starts afresh.
func (s *ListStack) Close() {
	s.open = nil
}

// Add puts a paragraph into the list for its kind ("bulletList" or
// "orderedList") and depth, opening one — at the top of out, or inside the
// item above — when none is, and returns out with any list it opened there.
func (s *ListStack) Add(out []*richdoc.Node, kind string, level int, block *richdoc.Node) []*richdoc.Node {
	for len(s.open) > 0 {
		top := s.open[len(s.open)-1]
		if top.level < level || (top.level == level && top.kind == kind) {
			break
		}
		s.open = s.open[:len(s.open)-1]
	}
	if len(s.open) == 0 || s.open[len(s.open)-1].level < level {
		list := &richdoc.Node{Type: kind}
		if len(s.open) == 0 {
			out = append(out, list)
		} else {
			// A list opened beneath another goes inside that list's last
			// item, which is there: a list is opened with its first item.
			parent := s.open[len(s.open)-1].node
			item := parent.Content[len(parent.Content)-1]
			item.Content = append(item.Content, list)
		}
		s.open = append(s.open, openList{kind: kind, level: level, node: list})
	}
	list := s.open[len(s.open)-1].node
	list.Content = append(list.Content, &richdoc.Node{Type: "listItem", Content: []*richdoc.Node{block}})
	return out
}
