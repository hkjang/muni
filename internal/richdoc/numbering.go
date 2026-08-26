package richdoc

import (
	"strconv"
	"strings"
)

// The numbering schemes a document can use for its headings.
const (
	NumberingNone    = "none"
	NumberingDecimal = "decimal"
	NumberingKorean  = "korean"
)

// ValidNumbering reports whether a stored value is a scheme, treating anything
// unrecognised as no numbering rather than refusing to render the document.
func ValidNumbering(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case NumberingDecimal:
		return NumberingDecimal
	case NumberingKorean:
		return NumberingKorean
	default:
		return NumberingNone
	}
}

// koreanOrder is the sequence Korean documents number their third level with.
var koreanOrder = []rune("가나다라마바사아자차카타파하")

// romanOrder covers the first level. Written in ASCII rather than with the
// Unicode roman numerals, which not every font a reader has will carry.
var romanOrder = []string{"I", "II", "III", "IV", "V", "VI", "VII", "VIII", "IX", "X"}

// HeadingNumbers works out the label in front of each heading.
//
// A Korean report is expected to number its sections, and doing it by hand
// means renumbering everything below whenever a section is inserted — which is
// exactly the kind of work nobody does reliably. The numbers are derived from
// the outline rather than stored, so inserting a section renumbers the rest by
// itself.
//
// The depth used is the one the outline panel shows: counted from the
// shallowest heading the document actually uses, so a document written in
// 제목 2 and 제목 3 numbers from the top rather than starting at 0.1.
func HeadingNumbers(headings []Heading, scheme string) []string {
	scheme = ValidNumbering(scheme)
	out := make([]string, len(headings))
	if scheme == NumberingNone || len(headings) == 0 {
		return out
	}

	counters := make([]int, 0, 8)
	for index, heading := range headings {
		depth := heading.Depth
		if depth < 0 {
			depth = 0
		}
		// Coming back out to a shallower level ends the deeper counts, so the
		// next subsection starts at one again.
		for len(counters) > depth+1 {
			counters = counters[:len(counters)-1]
		}
		for len(counters) < depth+1 {
			counters = append(counters, 0)
		}
		counters[depth]++

		switch scheme {
		case NumberingDecimal:
			parts := make([]string, 0, len(counters))
			for _, count := range counters {
				parts = append(parts, strconv.Itoa(count))
			}
			out[index] = strings.Join(parts, ".") + "."
		case NumberingKorean:
			out[index] = koreanLabel(depth, counters[depth])
		}
	}
	return out
}

// koreanLabel numbers one level the way a Korean report does: roman numerals,
// then arabic, then 가나다, then arabic in parentheses.
func koreanLabel(depth, count int) string {
	switch depth {
	case 0:
		if count <= len(romanOrder) {
			return romanOrder[count-1] + "."
		}
		return strconv.Itoa(count) + "."
	case 1:
		return strconv.Itoa(count) + "."
	case 2:
		if count <= len(koreanOrder) {
			return string(koreanOrder[count-1]) + "."
		}
		return strconv.Itoa(count) + "."
	case 3:
		return strconv.Itoa(count) + ")"
	default:
		if count <= len(koreanOrder) {
			return string(koreanOrder[count-1]) + ")"
		}
		return strconv.Itoa(count) + ")"
	}
}

// WithHeadingNumbers returns the document with each heading's number written
// into its text, for the exports that have no other way to show it.
//
// The stored document is left alone: a number written into the document would
// be wrong the moment a section moved.
func WithHeadingNumbers(doc *Node, scheme string) *Node {
	if doc == nil || ValidNumbering(scheme) == NumberingNone {
		return doc
	}
	headings := Headings(doc)
	numbers := HeadingNumbers(headings, scheme)
	if len(numbers) == 0 {
		return doc
	}
	index := 0
	var apply func(*Node) *Node
	apply = func(node *Node) *Node {
		if node == nil {
			return nil
		}
		if node.Type == "heading" {
			if strings.TrimSpace(node.PlainText()) == "" {
				return node
			}
			label := ""
			if index < len(numbers) {
				label = numbers[index]
			}
			index++
			if label == "" {
				return node
			}
			copied := *node
			copied.Content = append([]*Node{Text(label + " ")}, node.Content...)
			return &copied
		}
		if len(node.Content) == 0 {
			return node
		}
		copied := *node
		children := make([]*Node, 0, len(node.Content))
		for _, child := range node.Content {
			children = append(children, apply(child))
		}
		copied.Content = children
		return &copied
	}
	return apply(doc)
}
