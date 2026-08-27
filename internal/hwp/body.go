package hwp

import (
	"encoding/binary"
	"strings"
	"unicode/utf16"

	"github.com/hkjang/muni/internal/richdoc"
)

// charRun is one stretch of a paragraph and the shape it wears. A
// PARA_CHAR_SHAPE record is a list of these: a position, and the shape that
// applies from there until the next one.
type charRun struct {
	at    uint32
	shape uint32
}

func readCharRuns(raw []byte) []charRun {
	out := make([]charRun, 0, len(raw)/8)
	for offset := 0; offset+8 <= len(raw); offset += 8 {
		out = append(out, charRun{
			at:    binary.LittleEndian.Uint32(raw[offset:]),
			shape: binary.LittleEndian.Uint32(raw[offset+4:]),
		})
	}
	return out
}

// shapeAt reports which shape covers a position.
func shapeAt(runs []charRun, position uint32) (uint32, bool) {
	found := uint32(0)
	ok := false
	for _, run := range runs {
		if run.at > position {
			break
		}
		found, ok = run.shape, true
	}
	return found, ok
}

// marksFor turns a shape id into the marks muni draws.
func (imp *importer) marksFor(id uint32) []richdoc.Mark {
	if int(id) >= len(imp.charShapes) {
		return nil
	}
	shape := imp.charShapes[id]
	marks := []richdoc.Mark{}
	if shape.bold {
		marks = append(marks, richdoc.Mark{Type: "bold"})
	}
	if shape.italic {
		marks = append(marks, richdoc.Mark{Type: "italic"})
	}
	if shape.underline {
		marks = append(marks, richdoc.Mark{Type: "underline"})
	}
	if shape.strike {
		marks = append(marks, richdoc.Mark{Type: "strike"})
	}
	return marks
}

// inline turns a paragraph's text and its shape list into muni's inline nodes.
//
// A stretch of text is broken wherever the shape changes inside it, which is
// what puts the bold on the word somebody bolded rather than on the paragraph.
func (imp *importer) inline(text paragraphText, runs []charRun) []*richdoc.Node {
	out := []*richdoc.Node{}
	for _, piece := range text.pieces {
		position := piece.at
		shape, _ := shapeAt(runs, position)
		var current strings.Builder
		flush := func() {
			if current.Len() > 0 {
				out = append(out, richdoc.Text(current.String(), imp.marksFor(shape)...))
				current.Reset()
			}
		}
		for _, letter := range piece.text {
			if next, _ := shapeAt(runs, position); next != shape {
				flush()
				shape = next
			}
			current.WriteRune(letter)
			// The file counts in UTF-16 code units, so a character outside the
			// basic plane moves the position by two.
			position += uint32(len(utf16.Encode([]rune{letter})))
		}
		flush()
	}
	return out
}

// paragraphs reads a run of PARA_HEADER nodes into blocks.
func (imp *importer) paragraphs(nodes []*recordNode) []*richdoc.Node {
	out := []*richdoc.Node{}
	for _, node := range nodes {
		if node.tag != tagParaHeader {
			continue
		}
		out = append(out, imp.paragraph(node)...)
	}
	return out
}

// paragraph reads one PARA_HEADER and everything under it.
func (imp *importer) paragraph(node *recordNode) []*richdoc.Node {
	textRecord := node.find(tagParaText)
	shapeRecord := node.find(tagParaCharShape)

	// Whatever the paragraph holds that is a block of its own comes out after
	// it, the way a table does in every other importer muni has.
	after := []*richdoc.Node{}
	for _, control := range node.all(tagCtrlHeader) {
		after = append(after, imp.control(control)...)
	}

	if textRecord == nil {
		if len(after) > 0 {
			return after
		}
		return nil
	}
	text := readParagraphText(textRecord.data)
	var runs []charRun
	if shapeRecord != nil {
		runs = readCharRuns(shapeRecord.data)
	}
	inline := imp.inline(text, runs)

	blocks := []*richdoc.Node{}
	for _, line := range splitLines(inline) {
		if blankInline(line) {
			continue
		}
		blocks = append(blocks, &richdoc.Node{Type: "paragraph", Content: line})
	}
	if len(blocks) == 0 && len(after) == 0 {
		return nil
	}
	return append(blocks, after...)
}

// splitLines breaks inline content at the line breaks a paragraph carries.
// muni's paragraph is one line's worth of content; HWP's holds as many as the
// writer typed.
func splitLines(inline []*richdoc.Node) [][]*richdoc.Node {
	lines := [][]*richdoc.Node{{}}
	for _, node := range inline {
		if node == nil {
			continue
		}
		if node.Type != "text" || !strings.Contains(node.Text, "\n") {
			lines[len(lines)-1] = append(lines[len(lines)-1], node)
			continue
		}
		parts := strings.Split(node.Text, "\n")
		for index, part := range parts {
			if index > 0 {
				lines = append(lines, []*richdoc.Node{})
			}
			if part != "" {
				lines[len(lines)-1] = append(lines[len(lines)-1],
					richdoc.Text(part, node.Marks...))
			}
		}
	}
	return lines
}

func blankInline(nodes []*richdoc.Node) bool {
	for _, node := range nodes {
		if node != nil && strings.TrimSpace(node.PlainText()) != "" {
			return false
		}
	}
	return true
}
