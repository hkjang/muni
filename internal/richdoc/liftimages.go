package richdoc

// Word and Markdown both write a picture inside the sentence that holds it:
// <w:drawing> lives in a run, and ![](…) lives in a line of prose. muni's
// editor draws an image as a block of its own, so a paragraph that still has
// one inside it is a shape the schema cannot represent — the editor refuses
// the whole document rather than losing the picture quietly.
//
// LiftImages rewrites that shape into the one the editor understands: the
// words before the picture, the picture on a line of its own, and the words
// after. Every importer runs it before handing a document on.

// blockContainers are the nodes whose children are blocks. Only inside one of
// these can a paragraph be replaced by the several blocks it splits into.
var blockContainers = map[string]bool{
	"doc":         true,
	"blockquote":  true,
	"listItem":    true,
	"taskItem":    true,
	"tableCell":   true,
	"tableHeader": true,
}

// LiftImages moves images out of the paragraphs and headings that hold them,
// in place, across the whole tree.
func LiftImages(node *Node) {
	if node == nil {
		return
	}
	for _, child := range node.Content {
		LiftImages(child)
	}
	if !blockContainers[node.Type] {
		return
	}
	out := make([]*Node, 0, len(node.Content))
	for _, child := range node.Content {
		out = append(out, splitAroundImages(child)...)
	}
	// A list item is "paragraph block*": whatever else it holds, it opens with
	// a paragraph. Lifting a picture that was the item's only content would
	// leave an image where that paragraph has to be.
	if paragraphFirst[node.Type] && len(out) > 0 && out[0].Type != "paragraph" {
		out = append([]*Node{Paragraph()}, out...)
	}
	node.Content = out
}

var paragraphFirst = map[string]bool{"listItem": true, "taskItem": true}

// splitAroundImages returns the blocks one paragraph becomes. A paragraph with
// no picture in it is returned unchanged, which is the usual case.
func splitAroundImages(block *Node) []*Node {
	if block == nil {
		return nil
	}
	if !holdsImage(block) {
		return []*Node{block}
	}
	if block.Type == "heading" {
		// A heading is one entry in the outline, the contents list and the
		// numbering. Splitting it around a picture would make two of them
		// where the author wrote one, so the picture goes after it instead.
		kept := block.Content[:0]
		pictures := []*Node{}
		for _, child := range block.Content {
			if child != nil && child.Type == "image" {
				pictures = append(pictures, child)
				continue
			}
			kept = append(kept, child)
		}
		block.Content = kept
		return append([]*Node{block}, pictures...)
	}
	if block.Type != "paragraph" {
		return []*Node{block}
	}
	out := make([]*Node, 0, len(block.Content)+1)
	run := make([]*Node, 0, len(block.Content))
	flush := func() {
		if len(run) == 0 || allBlank(run) {
			run = nil
			return
		}
		out = append(out, &Node{Type: block.Type, Attrs: copyAttrs(block.Attrs), Content: run})
		run = nil
	}
	for _, child := range block.Content {
		if child != nil && child.Type == "image" {
			flush()
			out = append(out, child)
			continue
		}
		run = append(run, child)
	}
	flush()
	if len(out) == 0 {
		// Cannot happen while holdsImage is true, but an empty container
		// would delete the block rather than fix it.
		return []*Node{block}
	}
	return out
}

func holdsImage(block *Node) bool {
	for _, child := range block.Content {
		if child != nil && child.Type == "image" {
			return true
		}
	}
	return false
}

// allBlank reports whether a run of inline nodes is only the spacing that sat
// next to the picture. Keeping it would leave an empty paragraph behind.
func allBlank(inline []*Node) bool {
	for _, child := range inline {
		if !child.IsBlank() {
			return false
		}
	}
	return true
}

func copyAttrs(attrs map[string]any) map[string]any {
	if len(attrs) == 0 {
		return nil
	}
	out := make(map[string]any, len(attrs))
	for key, value := range attrs {
		out[key] = value
	}
	return out
}
