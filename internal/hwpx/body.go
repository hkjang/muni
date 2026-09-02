package hwpx

import (
	"archive/zip"
	"net/http"
	"strconv"
	"strings"

	"github.com/hkjang/muni/internal/richdoc"
)

// loadBinData keeps the bytes of every picture, by the id a run refers to.
// loadBinData notes where each picture is, without reading any of it.
//
// Decompressing every part up front turns a small zip with a few hundred
// highly compressible entries into gigabytes in memory before a single
// picture has been asked for. A document usually refers to fewer pictures
// than it carries, and one it never refers to is never read.
func (imp *importer) loadBinData(files map[string]*zip.File) {
	for name, file := range files {
		if !strings.HasPrefix(strings.ToLower(name), "bindata/") {
			continue
		}
		base := name[len("BinData/"):]
		// A picture refers to the name with and without its extension,
		// depending on which part of the file is doing the referring.
		imp.binaryParts[strings.ToLower(base)] = file
		if dot := strings.LastIndex(base, "."); dot > 0 {
			imp.binaryParts[strings.ToLower(base[:dot])] = file
		}
	}
}

// binaryFor reads one picture, once.
func (imp *importer) binaryFor(id string) ([]byte, bool) {
	key := strings.ToLower(id)
	if data, seen := imp.binary[key]; seen {
		return data, len(data) > 0
	}
	file, ok := imp.binaryParts[key]
	if !ok {
		return nil, false
	}
	data, err := readBytes(file)
	if err != nil {
		data = nil
	}
	imp.binary[key] = data
	return data, len(data) > 0
}

// sectionIsLandscape reads the paper the section asks for.
func (imp *importer) sectionIsLandscape(root *node) bool {
	page := root.descendant("pagePr")
	if page == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(page.attr("landscape")), "WIDELY") {
		return true
	}
	width, wErr := strconv.Atoi(strings.TrimSpace(page.attr("width")))
	height, hErr := strconv.Atoi(strings.TrimSpace(page.attr("height")))
	return wErr == nil && hErr == nil && width > height
}

// blocks reads a section into muni's blocks.
func (imp *importer) blocks(root *node) []*richdoc.Node {
	out := []*richdoc.Node{}
	for _, child := range root.children {
		out = append(out, imp.block(child)...)
	}
	return out
}

func (imp *importer) block(current *node) []*richdoc.Node {
	switch {
	case current.is("p"):
		return imp.paragraph(current)
	case current.is("tbl"):
		if table := imp.table(current); table != nil {
			return []*richdoc.Node{table}
		}
	default:
		// A wrapper muni does not know: its paragraphs are still paragraphs.
		out := []*richdoc.Node{}
		for _, child := range current.children {
			out = append(out, imp.block(child)...)
		}
		return out
	}
	return nil
}

// paragraph reads one <hp:p>, which is a paragraph, a heading, or the frame a
// table or a picture sits in.
func (imp *importer) paragraph(current *node) []*richdoc.Node {
	// A table is a block of its own in muni. In HWPX it lives inside the
	// paragraph that positions it, so it comes out and the paragraph keeps
	// whatever text was beside it.
	// Only the tables this paragraph holds, not every table below it: a table
	// inside a cell is built by the cell that holds it, and finding it again
	// here would put it in the document twice.
	lifted := []*richdoc.Node{}
	for _, table := range tablesIn(current) {
		if built := imp.table(table); built != nil {
			lifted = append(lifted, built)
		}
	}

	inline := imp.runs(current)
	style := imp.styles[current.attr("styleIDRef")]
	shape := imp.paraShapes[firstNonEmpty(current.attr("paraPrIDRef"), style.paraShapeID)]

	if len(inline) == 0 {
		if len(lifted) > 0 {
			return lifted
		}
		return []*richdoc.Node{richdoc.Paragraph()}
	}

	block := &richdoc.Node{Type: "paragraph", Content: inline}
	if level := style.headingLevel; level > 0 {
		block.Type = "heading"
		block.SetAttr("level", level)
	} else {
		// Alignment, indentation and line spacing describe a paragraph, not a
		// heading, whose shape muni draws itself.
		if shape.align != "" {
			block.SetAttr("textAlign", shape.align)
		}
		if shape.indent > 0 {
			block.SetAttr("indent", shape.indent)
		}
		if shape.firstLin {
			block.SetAttr("firstLine", true)
		}
		if shape.lineRate != "" {
			block.SetAttr("lineHeight", shape.lineRate)
		}
	}
	return append([]*richdoc.Node{block}, lifted...)
}

// tablesIn finds the tables a paragraph positions, stopping at a cell — what
// is inside one belongs to that cell.
func tablesIn(current *node) []*node {
	out := []*node{}
	var walk func(*node)
	walk = func(at *node) {
		for _, child := range at.children {
			switch {
			case child.is("tbl"):
				out = append(out, child)
			case child.is("tc"), child.is("subList"):
				// Someone else's.
			default:
				walk(child)
			}
		}
	}
	walk(current)
	return out
}

// runs reads the inline content of a paragraph.
func (imp *importer) runs(paragraph *node) []*richdoc.Node {
	out := []*richdoc.Node{}
	for _, run := range paragraph.children {
		if !run.is("run") {
			continue
		}
		shape := imp.charShapes[run.attr("charPrIDRef")]
		marks := shape.marks()
		for _, child := range run.children {
			switch {
			case child.is("t"):
				out = append(out, imp.text(child, marks)...)
			case child.is("pic"), child.is("picture"):
				if image := imp.picture(child); image != nil {
					out = append(out, image)
				}
			case child.is("lineBreak"):
				out = append(out, &richdoc.Node{Type: "hardBreak"})
			case child.is("footNote"), child.is("endNote"):
				// A note lives inside its reference, which is where muni
				// keeps one too. Reading only the paragraph's own text left
				// every note in every Hangul file behind — the round trip
				// through muni's own writer is what found it.
				if note := imp.note(child); note != nil {
					out = append(out, note)
				}
			case child.is("tbl"):
				// Read as a block by paragraph(); nothing to do inline.
			}
		}
	}
	return richdoc.Doc(out...).Content
}

// note reads a footnote or endnote into muni's one kind of note, as the text
// of its paragraphs joined by a space — muni's note holds text and nothing
// else.
func (imp *importer) note(current *node) *richdoc.Node {
	lines := []string{}
	current.each("p", func(paragraph *node) {
		if text := strings.TrimSpace(paragraph.allText()); text != "" {
			lines = append(lines, text)
		}
	})
	if len(lines) == 0 {
		return nil
	}
	return &richdoc.Node{Type: richdoc.FootnoteType, Content: []*richdoc.Node{richdoc.Text(strings.Join(lines, " "))}}
}

// text reads one <hp:t>, whose children are the things that interrupt a run of
// characters rather than end it.
func (imp *importer) text(current *node, marks []richdoc.Mark) []*richdoc.Node {
	out := []*richdoc.Node{}
	add := func(value string) {
		if value != "" {
			out = append(out, richdoc.Text(value, marks...))
		}
	}
	// In order: a tab written between two halves of a sentence belongs
	// between them, not after both.
	for _, child := range current.children {
		switch {
		case child.is(textName):
			add(child.text)
		case child.is("tab"):
			add("\t")
		case child.is("lineBreak"):
			out = append(out, &richdoc.Node{Type: "hardBreak"})
		default:
			add(child.allText())
		}
	}
	return out
}

func (shape charShape) marks() []richdoc.Mark {
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

// picture reads a <hp:pic> into an image node, keeping its bytes as an asset.
func (imp *importer) picture(current *node) *richdoc.Node {
	image := current.descendant("img")
	if image == nil {
		return nil
	}
	id := firstNonEmpty(image.attr("binaryItemIDRef"), image.attr("BinItem"), image.attr("id"))
	if id == "" {
		return nil
	}
	data, ok := imp.binaryFor(id)
	if !ok {
		return nil
	}
	placeholder, seen := imp.assetByID[id]
	if !seen {
		placeholder = richdoc.Placeholder(len(imp.assets) + 1)
		imp.assets = append(imp.assets, richdoc.Asset{
			Placeholder: placeholder,
			Name:        id,
			MediaType:   http.DetectContentType(data),
			Data:        data,
		})
		imp.assetByID[id] = placeholder
	}
	node := &richdoc.Node{Type: "image"}
	node.SetAttr("src", placeholder)
	if alt := strings.TrimSpace(current.attr("alt")); alt != "" {
		node.SetAttr("alt", alt)
	}
	return node
}
