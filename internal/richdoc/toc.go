package richdoc

import "strings"

// TableOfContentsType is the node an author inserts where they want a
// contents list. It is stored empty: the entries are generated when the
// document is exported, so they can never be stale.
const TableOfContentsType = "tableOfContents"

// Heading is one line of a document's outline.
type Heading struct {
	Level int
	Text  string
	// Depth is how far the entry is indented, counting from the shallowest
	// heading the document actually uses.
	Depth int
}

// Headings reads the outline of a document.
//
// Indentation counts from the shallowest heading present rather than from h1,
// so a document written entirely in h2 and h3 is not listed pushed to the
// right, and a heading never indents more than one step past the one above it,
// which keeps a document that skips from h1 to h4 readable. The editor's
// outline panel follows the same rule, so what is exported matches what the
// author was looking at.
func Headings(doc *Node) []Heading {
	raw := make([]Heading, 0, 16)
	var walk func(*Node)
	walk = func(node *Node) {
		if node == nil {
			return
		}
		if node.Type == "heading" {
			text := strings.TrimSpace(node.PlainText())
			if text != "" {
				raw = append(raw, Heading{Level: clampLevel(node.AttrInt("level", 1)), Text: text})
			}
			return
		}
		for _, child := range node.Content {
			walk(child)
		}
	}
	walk(doc)
	if len(raw) == 0 {
		return nil
	}

	shallowest := raw[0].Level
	for _, heading := range raw {
		if heading.Level < shallowest {
			shallowest = heading.Level
		}
	}

	previousDepth := -1
	previousLevel := shallowest
	for index := range raw {
		level := raw[index].Level
		var depth int
		switch {
		case level <= shallowest:
			depth = 0
		case level > previousLevel:
			depth = previousDepth + 1
		case level == previousLevel:
			depth = previousDepth
		default:
			depth = previousDepth - (previousLevel - level)
			if depth < 0 {
				depth = 0
			}
		}
		raw[index].Depth = depth
		previousDepth = depth
		previousLevel = level
	}
	return raw
}

func clampLevel(level int) int {
	if level < 1 {
		return 1
	}
	if level > 6 {
		return 6
	}
	return level
}

// WithTableOfContents fills in every contents node, leaving the document
// otherwise untouched.
//
// The entries are generated here rather than stored so that a contents list
// can never disagree with the headings above it. Every writer already renders
// the children of a node type it does not recognise, so filling the node is
// all it takes for the list to reach HTML, PDF, DOCX, Markdown and text alike.
func WithTableOfContents(doc *Node) *Node {
	if doc == nil || !containsTOC(doc) {
		return doc
	}
	headings := Headings(doc)
	return fillTOC(doc, headings)
}

func containsTOC(node *Node) bool {
	if node == nil {
		return false
	}
	if node.Type == TableOfContentsType {
		return true
	}
	for _, child := range node.Content {
		if containsTOC(child) {
			return true
		}
	}
	return false
}

func fillTOC(node *Node, headings []Heading) *Node {
	if node == nil {
		return nil
	}
	if node.Type == TableOfContentsType {
		return tocNode(node, headings)
	}
	copied := *node
	if len(node.Content) > 0 {
		children := make([]*Node, 0, len(node.Content))
		for _, child := range node.Content {
			children = append(children, fillTOC(child, headings))
		}
		copied.Content = children
	}
	return &copied
}

func tocNode(original *Node, headings []Heading) *Node {
	copied := *original
	if len(headings) == 0 {
		// An empty list would export as nothing at all, which reads as a bug
		// rather than as a document without headings.
		copied.Content = []*Node{Paragraph(Text("(제목이 없어 목차를 만들 수 없습니다)"))}
		return &copied
	}
	entries := make([]*Node, 0, len(headings))
	for _, heading := range headings {
		entry := Paragraph(Text(heading.Text))
		if heading.Depth > 0 {
			entry.SetAttr("indent", heading.Depth)
		}
		entries = append(entries, entry)
	}
	copied.Content = entries
	return &copied
}
