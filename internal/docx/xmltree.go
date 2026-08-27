package docx

import (
	"encoding/xml"
	"io"
	"strings"
)

// xnode is a lightweight DOM for OOXML parts. Struct tags cannot express the
// "any of these elements, in order" shape WordprocessingML uses, so the
// importer walks a generic tree instead.
type xnode struct {
	Space    string
	Local    string
	Attrs    map[string]string
	Children []*xnode
	Text     string
}

var namespacePrefixes = map[string]string{
	"http://schemas.openxmlformats.org/wordprocessingml/2006/main":                  "w",
	"http://schemas.openxmlformats.org/officeDocument/2006/relationships":           "r",
	"http://schemas.openxmlformats.org/drawingml/2006/main":                         "a",
	"http://schemas.openxmlformats.org/drawingml/2006/picture":                      "pic",
	"http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing":        "wp",
	"http://schemas.openxmlformats.org/package/2006/relationships":                  "pr",
	"http://schemas.microsoft.com/office/word/2010/wordml":                          "w14",
	"urn:schemas-microsoft-com:vml":                                                 "v",
	"urn:schemas-microsoft-com:office:office":                                       "o",
	"http://schemas.openxmlformats.org/officeDocument/2006/math":                    "m",
	"http://schemas.microsoft.com/office/word/2010/wordprocessingShape":             "wps",
	"http://schemas.openxmlformats.org/officeDocument/2006/extended-properties":     "ep",
	"http://schemas.openxmlformats.org/package/2006/metadata/core-properties":       "cp",
	"http://schemas.microsoft.com/office/word/2010/wordprocessingDrawing":           "wp14",
	"http://schemas.openxmlformats.org/markup-compatibility/2006":                   "mc",
	"http://schemas.microsoft.com/office/drawing/2010/main":                         "a14",
	"http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink": "rh",
}

func prefixFor(space string) string {
	if prefix, ok := namespacePrefixes[space]; ok {
		return prefix
	}
	return ""
}

func parseXML(reader io.Reader) (*xnode, error) {
	decoder := xml.NewDecoder(reader)
	decoder.Strict = false
	decoder.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) { return input, nil }
	var root *xnode
	stack := make([]*xnode, 0, 32)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			node := &xnode{Space: typed.Name.Space, Local: typed.Name.Local, Attrs: map[string]string{}}
			for _, attribute := range typed.Attr {
				name := attribute.Name.Local
				if prefix := prefixFor(attribute.Name.Space); prefix != "" {
					node.Attrs[prefix+":"+name] = attribute.Value
				}
				if _, exists := node.Attrs[name]; !exists {
					node.Attrs[name] = attribute.Value
				}
			}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, node)
			} else if root == nil {
				root = node
			}
			stack = append(stack, node)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].Text += string(typed)
			}
		}
	}
	if root == nil {
		return &xnode{}, nil
	}
	return root, nil
}

func (n *xnode) is(prefix, local string) bool {
	if n == nil {
		return false
	}
	return n.Local == local && prefixFor(n.Space) == prefix
}

func (n *xnode) child(prefix, local string) *xnode {
	if n == nil {
		return nil
	}
	for _, child := range n.Children {
		if child.is(prefix, local) {
			return child
		}
	}
	return nil
}

// descendants finds every matching node below n, in document order. Where
// descendant answers "is there one", this answers "how many, and which".
func descendants(n *xnode, prefix, local string) []*xnode {
	var out []*xnode
	var walk func(*xnode)
	walk = func(node *xnode) {
		if node == nil {
			return
		}
		if node.is(prefix, local) {
			out = append(out, node)
			// A box inside a box is the same box's content; do not read it
			// twice.
			return
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(n)
	return out
}

// children is Children read through a possibly-nil node, so a style with no
// properties of its own reads as having none rather than needing a guard.
func (n *xnode) children() []*xnode {
	if n == nil {
		return nil
	}
	return n.Children
}

// descendant finds the first matching node anywhere below n, which keeps the
// drawing/blip lookups short without hard-coding every intermediate element.
func (n *xnode) descendant(prefix, local string) *xnode {
	if n == nil {
		return nil
	}
	for _, child := range n.Children {
		if child.is(prefix, local) {
			return child
		}
		if found := child.descendant(prefix, local); found != nil {
			return found
		}
	}
	return nil
}

func (n *xnode) attr(name string) string {
	if n == nil {
		return ""
	}
	return n.Attrs[name]
}

func (n *xnode) val() string {
	return n.attr("w:val")
}

// flag reports the value of an OOXML on/off property, where a bare element
// means "on" and w:val="0"/"false" turns it off.
func (n *xnode) flag() bool {
	if n == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(n.val())) {
	case "0", "false", "off", "none":
		return false
	default:
		return true
	}
}

func (n *xnode) allText() string {
	if n == nil {
		return ""
	}
	var out strings.Builder
	var walk func(*xnode)
	walk = func(node *xnode) {
		if node.is("w", "t") || node.is("w", "delText") {
			out.WriteString(node.Text)
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(n)
	return out.String()
}
