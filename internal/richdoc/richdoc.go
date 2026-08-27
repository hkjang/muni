// Package richdoc holds the ProseMirror/TipTap document model shared by the
// import and export pipelines. Working with typed nodes instead of raw maps
// keeps the DOCX and PDF converters readable and lets them agree on exactly
// which attributes survive a round trip.
package richdoc

import (
	"encoding/json"
	"strconv"
	"strings"
)

type Mark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

type Node struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Content []*Node        `json:"content,omitempty"`
	Marks   []Mark         `json:"marks,omitempty"`
	Text    string         `json:"text,omitempty"`
}

// AssetPlaceholderPrefix marks an image source that still points at an
// imported asset rather than at stored bytes.
const AssetPlaceholderPrefix = "muni-import-image:"

// Placeholder builds the source an importer puts on an image node.
func Placeholder(index int) string {
	return AssetPlaceholderPrefix + strconv.Itoa(index)
}

// Asset is a binary resource discovered while importing (an embedded image).
// The importer references it from an image node through Placeholder; the caller
// stores the bytes and rewrites the placeholder to a real attachment URL.
type Asset struct {
	Placeholder string
	Name        string
	MediaType   string
	Data        []byte
}

func Parse(raw json.RawMessage) (*Node, error) {
	var node Node
	if err := json.Unmarshal(raw, &node); err != nil {
		return nil, err
	}
	return &node, nil
}

func (n *Node) JSON() (json.RawMessage, error) {
	if n == nil {
		n = Doc()
	}
	if n.Type == "doc" && len(n.Content) == 0 {
		n.Content = []*Node{Paragraph()}
	}
	return json.Marshal(n)
}

func Doc(children ...*Node) *Node { return &Node{Type: "doc", Content: children} }

func Paragraph(children ...*Node) *Node {
	return &Node{Type: "paragraph", Content: children}
}

func Text(value string, marks ...Mark) *Node {
	return &Node{Type: "text", Text: value, Marks: marks}
}

func (n *Node) Append(children ...*Node) *Node {
	for _, child := range children {
		if child != nil {
			n.Content = append(n.Content, child)
		}
	}
	return n
}

func (n *Node) SetAttr(key string, value any) *Node {
	if value == nil {
		return n
	}
	if n.Attrs == nil {
		n.Attrs = map[string]any{}
	}
	n.Attrs[key] = value
	return n
}

func (n *Node) Attr(key string) any {
	if n == nil || n.Attrs == nil {
		return nil
	}
	return n.Attrs[key]
}

func (n *Node) AttrString(key string) string {
	switch value := n.Attr(key).(type) {
	case string:
		return value
	case nil:
		return ""
	default:
		return ""
	}
}

func (n *Node) AttrInt(key string, fallback int) int {
	switch value := n.Attr(key).(type) {
	case float64:
		return int(value)
	case int:
		return value
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			return int(parsed)
		}
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(value), "px")); err == nil {
			return parsed
		}
	}
	return fallback
}

func (n *Node) AttrBool(key string) bool {
	value, _ := n.Attr(key).(bool)
	return value
}

func (m Mark) AttrString(key string) string {
	if m.Attrs == nil {
		return ""
	}
	value, _ := m.Attrs[key].(string)
	return value
}

func (n *Node) HasMark(kind string) bool {
	for _, mark := range n.Marks {
		if mark.Type == kind {
			return true
		}
	}
	return false
}

func (n *Node) Mark(kind string) (Mark, bool) {
	for _, mark := range n.Marks {
		if mark.Type == kind {
			return mark, true
		}
	}
	return Mark{}, false
}

// PlainText flattens the node for search indexing and previews.
func (n *Node) PlainText() string {
	var out strings.Builder
	var walk func(*Node)
	walk = func(node *Node) {
		if node == nil {
			return
		}
		if node.Type == "text" {
			out.WriteString(node.Text)
		}
		for _, child := range node.Content {
			walk(child)
		}
		switch node.Type {
		case "paragraph", "heading", "listItem", "taskItem", "codeBlock", "tableRow", "blockquote":
			out.WriteString("\n")
		case "hardBreak":
			// A line break is a line break. Without this the lines either side
			// of one run together — in the search index, in a preview, and in
			// a footnote written as two lines.
			out.WriteString("\n")
		}
	}
	walk(n)
	return strings.TrimSpace(out.String())
}

// IsBlank reports whether a block carries no visible text or embedded content.
func (n *Node) IsBlank() bool {
	if n == nil {
		return true
	}
	if n.Type == "image" || n.Type == "horizontalRule" || n.Type == "pageBreak" || n.Type == "hardBreak" {
		return false
	}
	if strings.TrimSpace(n.Text) != "" {
		return false
	}
	for _, child := range n.Content {
		if !child.IsBlank() {
			return false
		}
	}
	return true
}
