package pdfx

import (
	"strings"
	"unicode"

	"github.com/hkjang/muni/internal/richdoc"
)

// A line of a PDF is drawn one run at a time, and the runs do not all look
// alike: half a sentence may be bold, and three words of it may be a link.
// Until now a line was one string with one set of marks, so a line was either
// all bold or none of it, and a link had nowhere to attach at all.
//
// spanText keeps a line as the pieces it was drawn in. Everything that reads a
// line to decide what it is — a heading, a bullet, a table row — asks for the
// plain string; only the moment a line becomes text in the document needs the
// pieces, and that is where they are spent.

type textSpan struct {
	text   string
	bold   bool
	italic bool
	mono   bool
	href   string
}

type spanText []textSpan

func (s spanText) String() string {
	if len(s) == 1 {
		return s[0].text
	}
	var out strings.Builder
	for _, span := range s {
		out.WriteString(span.text)
	}
	return out.String()
}

func (s spanText) empty() bool {
	for _, span := range s {
		if span.text != "" {
			return false
		}
	}
	return true
}

// slice takes the piece of a line between two byte offsets of its plain text.
func (s spanText) slice(from, to int) spanText {
	if from < 0 {
		from = 0
	}
	out := make(spanText, 0, len(s))
	at := 0
	for _, span := range s {
		end := at + len(span.text)
		if end > from && at < to {
			low, high := from-at, to-at
			if low < 0 {
				low = 0
			}
			if high > len(span.text) {
				high = len(span.text)
			}
			if low < high {
				piece := span
				piece.text = span.text[low:high]
				out = append(out, piece)
			}
		}
		at = end
	}
	return out
}

// trimSpace drops the spaces at either end without losing what the rest of
// the line was drawn as.
func (s spanText) trimSpace() spanText {
	whole := s.String()
	trimmed := strings.TrimSpace(whole)
	if trimmed == whole {
		return s
	}
	if trimmed == "" {
		return nil
	}
	start := strings.Index(whole, trimmed)
	return s.slice(start, start+len(trimmed))
}

// join adds a piece of text that carries no marks of its own.
func (s spanText) join(text string) spanText {
	if text == "" {
		return s
	}
	return append(append(spanText{}, s...), textSpan{text: text})
}

// concat puts two lines together, which is what wrapping a paragraph does.
func (s spanText) concat(other spanText) spanText {
	return append(append(spanText{}, s...), other...)
}

// allBold reports whether every piece of the line is bold, which is what a
// heading looks like.
func (s spanText) allBold() bool {
	seen := false
	for _, span := range s {
		if strings.TrimSpace(span.text) == "" {
			continue
		}
		if !span.bold {
			return false
		}
		seen = true
	}
	return seen
}

// nodes turns a line into the text of a document, one node per run, with the
// runs that look alike put back together.
func (s spanText) nodes(extra ...richdoc.Mark) []*richdoc.Node {
	out := make([]*richdoc.Node, 0, len(s))
	merged := make(spanText, 0, len(s))
	for _, span := range s {
		if span.text == "" {
			continue
		}
		if last := len(merged) - 1; last >= 0 && merged[last].sameLook(span) {
			merged[last].text += span.text
			continue
		}
		merged = append(merged, span)
	}
	for _, span := range merged {
		marks := append([]richdoc.Mark{}, extra...)
		if span.bold && !hasMark(marks, "bold") {
			marks = append(marks, richdoc.Mark{Type: "bold"})
		}
		if span.italic {
			marks = append(marks, richdoc.Mark{Type: "italic"})
		}
		if span.href != "" {
			marks = append(marks, richdoc.Mark{Type: "link", Attrs: map[string]any{"href": span.href}})
		}
		out = append(out, richdoc.Text(span.text, marks...))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (s textSpan) sameLook(other textSpan) bool {
	return s.bold == other.bold && s.italic == other.italic &&
		s.mono == other.mono && s.href == other.href
}

func hasMark(marks []richdoc.Mark, kind string) bool {
	for _, mark := range marks {
		if mark.Type == kind {
			return true
		}
	}
	return false
}

// joinSpans wraps one line onto the end of another, the way joinWrapped does
// for plain text.
func joinSpans(left, right spanText) spanText {
	if left.empty() {
		return right
	}
	leftText, rightText := left.String(), right.String()
	leftRunes, rightRunes := []rune(leftText), []rune(rightText)
	if len(leftRunes) == 0 {
		return right
	}
	last := leftRunes[len(leftRunes)-1]
	if last == '-' && len(rightRunes) > 0 && unicode.IsLower(rightRunes[0]) {
		return left.slice(0, len(leftText)-len(string(last))).concat(right)
	}
	if isCJK(last) || (len(rightRunes) > 0 && isCJK(rightRunes[0])) {
		return left.concat(right)
	}
	return left.join(" ").concat(right)
}
