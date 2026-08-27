package hwpx

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

// HWPX is XML in a zip, like .docx, but muni reads it with a reader of its own
// rather than the one in internal/docx.
//
// The two formats name things differently enough that sharing would mean
// carrying both namespace tables everywhere, and HWPX does not need the
// namespaces at all: within an HWPX part the local names — p, run, t, tbl, tc
// — do not collide, so matching on the local name alone is both simpler and
// tolerant of the prefix a writer happened to choose.
type node struct {
	name     string
	attrs    map[string]string
	children []*node
	text     string
}

// textName is the name given to a run of characters, so that characters and
// elements keep the order they were written in.
//
// Holding a node's characters in one field instead loses where they sat: for
// <hp:t>가나<hp:tab/>다라</hp:t> it gives "가나다라" and a tab afterwards,
// rather than a tab between the two halves.
const textName = "#text"

// maxDepth bounds how deep a part may nest. Walking a tree recursively is the
// natural way to read one, and a file nested ten million deep would overflow
// the stack — which in Go is a fatal error that takes the server with it, not
// a panic one request can recover from.
const maxDepth = 512

func parse(reader io.Reader) (*node, error) {
	decoder := xml.NewDecoder(reader)
	decoder.Strict = false
	var root *node
	stack := []*node{}
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
			if len(stack) >= maxDepth {
				return nil, errors.New("HWPX 파일이 너무 깊게 중첩되어 있습니다")
			}
			current := &node{name: typed.Name.Local, attrs: map[string]string{}}
			for _, attribute := range typed.Attr {
				// The local name again: a writer may call the same attribute
				// hp:id or just id.
				current.attrs[attribute.Name.Local] = attribute.Value
			}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, current)
			} else if root == nil {
				root = current
			}
			stack = append(stack, current)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.text += string(typed)
				parent.children = append(parent.children,
					&node{name: textName, text: string(typed)})
			}
		}
	}
	if root == nil {
		return nil, io.ErrUnexpectedEOF
	}
	return root, nil
}

func (n *node) is(name string) bool { return n != nil && n.name == name }

func (n *node) attr(name string) string {
	if n == nil {
		return ""
	}
	return n.attrs[name]
}

func (n *node) child(name string) *node {
	if n == nil {
		return nil
	}
	for _, child := range n.children {
		if child.name == name {
			return child
		}
	}
	return nil
}

// descendant finds the first matching node anywhere below n.
func (n *node) descendant(name string) *node {
	if n == nil {
		return nil
	}
	for _, child := range n.children {
		if child.name == name {
			return child
		}
		if found := child.descendant(name); found != nil {
			return found
		}
	}
	return nil
}

// each walks every descendant with the given name, in document order.
func (n *node) each(name string, visit func(*node)) {
	if n == nil {
		return
	}
	for _, child := range n.children {
		if child.name == name {
			visit(child)
		}
		child.each(name, visit)
	}
}

// allText is every character below a node, which is what a cell or a note
// amounts to when its structure is not being kept.
func (n *node) allText() string {
	if n == nil {
		return ""
	}
	var out strings.Builder
	var walk func(*node)
	walk = func(current *node) {
		if current.is(textName) {
			out.WriteString(current.text)
			return
		}
		for _, child := range current.children {
			walk(child)
		}
	}
	walk(n)
	return out.String()
}
