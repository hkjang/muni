package richdoc

import "strconv"

// FootnoteType is the note an author attaches to a word.
//
// The note's text lives inside the node, at the point in the sentence it
// belongs to. Nothing about it is numbered in storage: a footnote's number is
// its position among the others, and a stored number is wrong the moment a
// paragraph moves. The number is worked out when the document is rendered, the
// same way heading numbers and the contents list already are.
const FootnoteType = "footnote"

// Footnote is one note, in the order it appears in the document.
type Footnote struct {
	// Number counts from one, in reading order.
	Number int
	// Content is what the note says.
	Content []*Node
}

// Footnotes reads a document's notes in the order a reader meets them.
func Footnotes(doc *Node) []Footnote {
	notes := make([]Footnote, 0, 8)
	var walk func(*Node)
	walk = func(node *Node) {
		if node == nil {
			return
		}
		if node.Type == FootnoteType {
			notes = append(notes, Footnote{Number: len(notes) + 1, Content: node.Content})
			// A note inside a note would number itself in the middle of its
			// own numbering. Nothing creates one; not descending makes sure
			// nothing can.
			return
		}
		for _, child := range node.Content {
			walk(child)
		}
	}
	walk(doc)
	return notes
}

// FootnoteText flattens a note to the single line the .docx and the PDF put at
// the foot of the page. A note is a sentence or two of prose; anything that
// would need paragraphs of its own belongs in the document.
func FootnoteText(note Footnote) string {
	// Walked as one node rather than child by child: a note written as two
	// lines has a hardBreak between them, and PlainText trims a break that is
	// all a node contains — so asking each child separately loses exactly the
	// thing that keeps the lines apart.
	return collapse((&Node{Type: FootnoteType, Content: note.Content}).PlainText())
}

// FootnoteMarker is what the reader sees in the sentence.
func FootnoteMarker(number int) string {
	return strconv.Itoa(number)
}

func collapse(value string) string {
	out := make([]rune, 0, len(value))
	space := false
	for _, r := range value {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			space = true
			continue
		}
		if space && len(out) > 0 {
			out = append(out, ' ')
		}
		space = false
		out = append(out, r)
	}
	return string(out)
}

// WithFootnoteNumbers returns a copy of the document with each note stamped
// with the number a reader will see.
//
// Rendering needs the number, and a renderer that counted as it went would
// need a counter of its own — which, shared across concurrent exports, is a
// number that belongs to whichever document got there first. Numbering the
// tree once, before anything renders it, keeps every renderer stateless.
//
// The numbers are never stored. This copy exists for the length of one export,
// because a note's number is its position and a stored one is wrong as soon as
// a paragraph moves.
func WithFootnoteNumbers(doc *Node) *Node {
	if doc == nil {
		return nil
	}
	counter := 0
	var stamp func(*Node) *Node
	stamp = func(node *Node) *Node {
		if node == nil {
			return nil
		}
		copied := *node
		if node.Type == FootnoteType {
			counter++
			copied.Attrs = map[string]any{"number": counter}
			return &copied
		}
		if len(node.Content) > 0 {
			children := make([]*Node, 0, len(node.Content))
			for _, child := range node.Content {
				children = append(children, stamp(child))
			}
			copied.Content = children
		}
		return &copied
	}
	return stamp(doc)
}
