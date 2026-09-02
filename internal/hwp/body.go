package hwp

import (
	"encoding/binary"
	"github.com/hkjang/muni/internal/hangul"
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
	attrs := map[string]any{}
	if shape.color != "" {
		attrs["color"] = shape.color
	}
	if shape.sizePoint != "" {
		attrs["fontSize"] = shape.sizePoint
	}
	if shape.family != "" {
		attrs["fontFamily"] = shape.family
	}
	if len(attrs) > 0 {
		marks = append(marks, richdoc.Mark{Type: "textStyle", Attrs: attrs})
	}
	return marks
}

// inline turns a paragraph's text and its shape list into muni's inline nodes.
//
// A stretch of text is broken wherever the shape changes inside it, which is
// what puts the bold on the word somebody bolded rather than on the paragraph.
func (imp *importer) inline(text paragraphText, runs []charRun, links []linkSpan) []*richdoc.Node {
	out := []*richdoc.Node{}
	for _, piece := range text.pieces {
		position := piece.at
		shape, _ := shapeAt(runs, position)
		href := linkAt(links, position)
		var current strings.Builder
		flush := func() {
			if current.Len() > 0 {
				marks := imp.marksFor(shape)
				if href != "" {
					marks = append(marks, richdoc.Mark{Type: "link", Attrs: map[string]any{"href": href}})
				}
				out = append(out, richdoc.Text(current.String(), marks...))
				current.Reset()
			}
		}
		for _, letter := range piece.text {
			next, _ := shapeAt(runs, position)
			if nextHref := linkAt(links, position); next != shape || nextHref != href {
				flush()
				shape, href = next, nextHref
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

// linkSpan is a stretch of a paragraph that is a hyperlink, in code units.
type linkSpan struct {
	begin, end uint32
	href       string
}

// linkAt says which address, if any, the character at a position links to.
func linkAt(links []linkSpan, position uint32) string {
	for _, link := range links {
		if position >= link.begin && position < link.end {
			return link.href
		}
	}
	return ""
}

// paragraphs reads a run of PARA_HEADER nodes into blocks.
func (imp *importer) paragraphs(nodes []*recordNode) []*richdoc.Node {
	out := []*richdoc.Node{}
	var lists hangul.ListStack
	for _, node := range nodes {
		if node.tag != tagParaHeader {
			continue
		}
		kind, level := imp.listShape(node)
		for _, block := range imp.paragraph(node) {
			if kind != "" && block.Type == "paragraph" {
				out = lists.Add(out, kind, level, block)
				continue
			}
			// Anything else — a heading, a table, a plain paragraph — ends
			// the list that was open.
			lists.Close()
			out = append(out, block)
		}
	}
	return out
}

// listShape says what list a paragraph's shape puts it in, if any.
func (imp *importer) listShape(node *recordNode) (kind string, level int) {
	refs := readParagraphRefs(node.data)
	if int(refs.shape) < len(imp.paraShapes) {
		shape := imp.paraShapes[refs.shape]
		return shape.list, shape.level
	}
	return "", 0
}

// paragraph reads one PARA_HEADER and everything under it.
func (imp *importer) paragraph(node *recordNode) []*richdoc.Node {
	textRecord := node.find(tagParaText)
	shapeRecord := node.find(tagParaCharShape)

	// Whatever the paragraph holds that is a block of its own comes out after
	// it, the way a table does in every other importer muni has.
	after := []*richdoc.Node{}
	notes := []*richdoc.Node{}
	for _, control := range node.all(tagCtrlHeader) {
		blocks, inline := imp.control(control)
		after = append(after, blocks...)
		notes = append(notes, inline...)
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
	// A field's text is between its marks in the paragraph; which field it
	// is comes from the field controls, in the same order.
	links := []linkSpan{}
	fields := 0
	for _, control := range node.all(tagCtrlHeader) {
		id := controlID(control.data)
		if id == "" || id[0] != '%' {
			continue
		}
		if href := fieldLink(control); href != "" && fields < len(text.fields) {
			span := text.fields[fields]
			if span.end != openField && span.end > span.begin {
				links = append(links, linkSpan{begin: span.begin, end: span.end, href: href})
			}
		}
		fields++
	}
	inline := imp.inline(text, runs, links)
	// A note goes at the end of the paragraph that referred to it. The mark
	// in the text says where in the sentence, and keeping that would mean
	// interleaving pieces and marks by position; the paragraph is right and
	// the words are all there, which is the part that was missing.
	inline = append(inline, notes...)

	refs := readParagraphRefs(node.data)
	level := 0
	if int(refs.style) < len(imp.styles) {
		level = imp.styles[refs.style].headingLevel
	}
	var shape paraShape
	if int(refs.shape) < len(imp.paraShapes) {
		shape = imp.paraShapes[refs.shape]
	}

	blocks := []*richdoc.Node{}
	for _, line := range splitLines(inline) {
		if blankInline(line) {
			continue
		}
		block := &richdoc.Node{Type: "paragraph", Content: line}
		if level > 0 {
			// A heading draws its own weight and spacing; the shape it names
			// describes a paragraph, and applying it would fight what muni
			// already does with a heading.
			block.Type = "heading"
			block.SetAttr("level", level)
		} else {
			if shape.align != "" {
				block.SetAttr("textAlign", shape.align)
			}
			if shape.indent > 0 {
				block.SetAttr("indent", shape.indent)
			}
			if shape.firstLine {
				block.SetAttr("firstLine", true)
			}
			if shape.lineRate != "" {
				block.SetAttr("lineHeight", shape.lineRate)
			}
		}
		blocks = append(blocks, block)
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
