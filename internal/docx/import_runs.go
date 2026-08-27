package docx

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hkjang/muni/internal/richdoc"
)

// runs converts the inline children of a paragraph, threading an optional
// link mark down through hyperlink and field wrappers.
func (imp *importer) runs(nodes []*xnode, link richdoc.Mark) []*richdoc.Node {
	out := make([]*richdoc.Node, 0, len(nodes))
	for _, node := range nodes {
		switch {
		case node.is("w", "r"):
			out = append(out, imp.run(node, link)...)
		case node.is("w", "hyperlink"):
			target := link
			if relID := node.attr("r:id"); relID != "" {
				if rel, ok := imp.rels[relID]; ok && rel.external && safeHref(rel.target) {
					target = richdoc.Mark{Type: "link", Attrs: map[string]any{"href": rel.target, "target": "_blank"}}
				}
			}
			out = append(out, imp.runs(node.Children, target)...)
		case node.is("w", "fldSimple"):
			target := link
			if href, ok := hyperlinkField(node.attr("w:instr")); ok {
				target = richdoc.Mark{Type: "link", Attrs: map[string]any{"href": href, "target": "_blank"}}
			}
			out = append(out, imp.runs(node.Children, target)...)
		case node.is("w", "ins"), node.is("w", "smartTag"), node.is("w", "bdo"), node.is("w", "dir"):
			out = append(out, imp.runs(node.Children, link)...)
		case node.is("mc", "AlternateContent"):
			out = append(out, imp.shapes(alternateContent(node), nil)...)
		case node.is("w", "sdt"):
			if content := node.child("w", "sdtContent"); content != nil {
				out = append(out, imp.runs(content.Children, link)...)
			}
		case node.is("m", "oMath"), node.is("m", "oMathPara"):
			if text := strings.TrimSpace(node.allText()); text != "" {
				out = append(out, richdoc.Text(text))
			}
		}
	}
	return mergeAdjacentText(out)
}

func hyperlinkField(instruction string) (string, bool) {
	fields := strings.Fields(instruction)
	for index, field := range fields {
		if strings.EqualFold(field, "HYPERLINK") && index+1 < len(fields) {
			href := strings.Trim(fields[index+1], `"`)
			if safeHref(href) {
				return href, true
			}
		}
	}
	return "", false
}

func safeHref(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "mailto:")
}

func (imp *importer) run(node *xnode, link richdoc.Mark) []*richdoc.Node {
	properties := node.child("w", "rPr")
	if properties.child("w", "vanish") != nil && properties.child("w", "vanish").flag() {
		return nil
	}
	marks := imp.marks(properties)
	if link.Type != "" {
		marks = append(marks, link)
	}
	out := make([]*richdoc.Node, 0, 4)
	for _, child := range node.Children {
		switch {
		case child.is("w", "t"):
			if child.Text != "" {
				out = append(out, richdoc.Text(child.Text, marks...))
			}
		case child.is("w", "tab"):
			out = append(out, richdoc.Text("\t", marks...))
		case child.is("w", "br"):
			if strings.EqualFold(child.attr("w:type"), "page") {
				// Marked here and lifted out of the paragraph by the block
				// assembler, which is where a page break belongs.
				out = append(out, &richdoc.Node{Type: "pageBreak"})
				continue
			}
			out = append(out, &richdoc.Node{Type: "hardBreak"})
		case child.is("w", "cr"):
			out = append(out, &richdoc.Node{Type: "hardBreak"})
		case child.is("w", "noBreakHyphen"):
			out = append(out, richdoc.Text("-", marks...))
		case child.is("w", "footnoteReference"), child.is("w", "endnoteReference"):
			// The little number in the sentence. What it points at lives in
			// word/footnotes.xml or word/endnotes.xml, both read before the
			// body was walked.
			key := strings.TrimSuffix(child.Local, "Reference") + ":" + child.attr("w:id")
			if note := imp.footnotes[key]; len(note) > 0 {
				out = append(out, &richdoc.Node{Type: richdoc.FootnoteType, Content: note})
			}
		case child.is("w", "softHyphen"):
			// Rendering hint only; it carries no textual content.
		case child.is("w", "sym"):
			if glyph := symbolRune(child.attr("w:char"), child.attr("w:font")); glyph != "" {
				out = append(out, richdoc.Text(glyph, marks...))
			}
		case child.is("mc", "AlternateContent"):
			out = append(out, imp.shapes(alternateContent(child), marks)...)
		case child.is("w", "drawing"), child.is("w", "pict"), child.is("w", "object"):
			out = append(out, imp.shapes([]*xnode{child}, marks)...)
		}
	}
	return out
}

// symbolRune maps a w:sym code point, undoing the Symbol/Wingdings private-use
// offset so bullets and check boxes survive as real characters.
func symbolRune(code, font string) string {
	value, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(code), "0x"), 16, 32)
	if err != nil || value == 0 {
		return ""
	}
	rune32 := rune(value)
	lowerFont := strings.ToLower(font)
	if rune32 >= 0xf000 && rune32 <= 0xf0ff {
		plain := rune32 - 0xf000
		switch {
		case strings.Contains(lowerFont, "wingdings"):
			switch plain {
			case 0xa8:
				return "☐"
			case 0xfe:
				return "☒"
			case 0x6f:
				return "☐"
			case 0xfc:
				return "✔"
			}
			return "•"
		case strings.Contains(lowerFont, "symbol"):
			switch plain {
			case 0xb7:
				return "•"
			case 0xa7:
				return "▪"
			case 0x2d:
				return "-"
			}
		}
		return string(plain)
	}
	return string(rune32)
}

// styledRunProperties returns the run's properties with the ones its character
// style supplies filled in underneath.
//
// Word's own "Strong" and "Emphasis" are character styles: the run says only
// which style it wears, and the bold or the italic lives in styles.xml. muni
// read the run alone, so text formatted through the styles gallery — which is
// how most templates do it — arrived as plain text.
//
// The run always wins. A style is what a run starts from, not what it is.
func (imp *importer) styledRunProperties(properties *xnode) *xnode {
	if properties == nil {
		return nil
	}
	id := properties.child("w", "rStyle").val()
	if id == "" {
		return properties
	}
	merged := &xnode{Space: properties.Space, Local: properties.Local, Attrs: properties.Attrs}
	// basedOn chains: "Strong" may sit on a style that sets the font. The
	// bound stops a file whose styles refer to each other in a circle.
	chain := make([][]*xnode, 0, 4)
	hyperlink := false
	for depth := 0; id != "" && depth < 16; depth++ {
		style, ok := imp.styles[id]
		if !ok {
			break
		}
		if strings.Contains(strings.ToLower(id), "hyperlink") {
			hyperlink = true
		}
		chain = append(chain, style.runProperties.children())
		id = style.basedOn
	}
	// w:rStyle stays: marks() reads it to recognise a code style.
	var skip []string
	if hyperlink {
		// Word's Hyperlink style is its blue and its underline. muni draws a
		// link its own way, and a colour taken from the style would freeze
		// every imported link to whatever blue that file happened to use.
		skip = append(skip, "w:color")
	}
	merged.Children = mergeProperties(properties.children(), chain, skip...)
	return merged
}

func (imp *importer) marks(properties *xnode) []richdoc.Mark {
	if properties == nil {
		return nil
	}
	properties = imp.styledRunProperties(properties)
	marks := make([]richdoc.Mark, 0, 4)
	styleAttrs := map[string]any{}

	if properties.child("w", "b").flag() && properties.child("w", "b") != nil {
		marks = append(marks, richdoc.Mark{Type: "bold"})
	}
	if properties.child("w", "i").flag() && properties.child("w", "i") != nil {
		marks = append(marks, richdoc.Mark{Type: "italic"})
	}
	if underline := properties.child("w", "u"); underline != nil {
		if value := strings.ToLower(underline.val()); value != "none" && value != "0" {
			marks = append(marks, richdoc.Mark{Type: "underline"})
		}
	}
	if properties.child("w", "strike").flag() && properties.child("w", "strike") != nil {
		marks = append(marks, richdoc.Mark{Type: "strike"})
	}
	if properties.child("w", "dstrike").flag() && properties.child("w", "dstrike") != nil {
		marks = append(marks, richdoc.Mark{Type: "strike"})
	}
	if vertical := properties.child("w", "vertAlign"); vertical != nil {
		switch strings.ToLower(vertical.val()) {
		case "superscript":
			marks = append(marks, richdoc.Mark{Type: "superscript"})
		case "subscript":
			marks = append(marks, richdoc.Mark{Type: "subscript"})
		}
	}

	monospaced := false
	if fonts := properties.child("w", "rFonts"); fonts != nil {
		family := fonts.attr("w:ascii")
		if family == "" {
			family = fonts.attr("w:eastAsia")
		}
		if family != "" {
			if isMonospaceFont(family) {
				monospaced = true
			} else {
				styleAttrs["fontFamily"] = family
			}
		}
	}
	if style := properties.child("w", "rStyle").val(); style != "" {
		key := strings.ToLower(style)
		if strings.Contains(key, "code") || strings.Contains(key, "verbatim") {
			monospaced = true
		}
		if strings.Contains(key, "hyperlink") {
			// The link mark itself comes from the relationship, not the style.
			delete(styleAttrs, "color")
		}
	}
	if monospaced {
		marks = append(marks, richdoc.Mark{Type: "code"})
	}

	if color := properties.child("w", "color"); color != nil {
		value := strings.ToUpper(strings.TrimSpace(color.val()))
		if value != "" && value != "AUTO" && value != "000000" && value != "202124" {
			styleAttrs["color"] = "#" + value
		}
	}
	if size := properties.child("w", "sz"); size != nil {
		if halfPoints, err := strconv.Atoi(strings.TrimSpace(size.val())); err == nil && halfPoints > 0 && halfPoints != 22 {
			styleAttrs["fontSize"] = fmt.Sprintf("%gpt", float64(halfPoints)/2)
		}
	}
	if len(styleAttrs) > 0 {
		marks = append(marks, richdoc.Mark{Type: "textStyle", Attrs: styleAttrs})
	}

	if highlight := properties.child("w", "highlight"); highlight != nil {
		if color := highlightColor(highlight.val()); color != "" {
			marks = append(marks, richdoc.Mark{Type: "highlight", Attrs: map[string]any{"color": color}})
		}
	} else if shading := properties.child("w", "shd"); shading != nil {
		fill := strings.ToUpper(strings.TrimSpace(shading.attr("w:fill")))
		if fill != "" && fill != "AUTO" && fill != "FFFFFF" {
			marks = append(marks, richdoc.Mark{Type: "highlight", Attrs: map[string]any{"color": "#" + fill}})
		}
	}
	return marks
}

func isMonospaceFont(name string) bool {
	lower := strings.ToLower(name)
	for _, candidate := range []string{"consolas", "courier", "monaco", "menlo", "mono", "d2coding", "나눔고딕코딩"} {
		if strings.Contains(lower, candidate) {
			return true
		}
	}
	return false
}

func highlightColor(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "none":
		return ""
	case "yellow":
		return "#FFFF00"
	case "green":
		return "#00FF00"
	case "cyan":
		return "#00FFFF"
	case "magenta":
		return "#FF00FF"
	case "blue":
		return "#0000FF"
	case "red":
		return "#FF0000"
	case "darkblue":
		return "#000080"
	case "darkcyan":
		return "#008080"
	case "darkgreen":
		return "#008000"
	case "darkmagenta":
		return "#800080"
	case "darkred":
		return "#800000"
	case "darkyellow":
		return "#808000"
	case "darkgray", "darkgrey":
		return "#808080"
	case "lightgray", "lightgrey":
		return "#C0C0C0"
	case "black":
		return "#000000"
	case "white":
		return "#FFFFFF"
	default:
		return "#FFF3A3"
	}
}

func (imp *importer) image(node *xnode) *richdoc.Node {
	relID := ""
	if blip := node.descendant("a", "blip"); blip != nil {
		relID = blip.attr("r:embed")
		if relID == "" {
			relID = blip.attr("r:link")
		}
	}
	if relID == "" {
		if data := node.descendant("v", "imagedata"); data != nil {
			relID = data.attr("r:id")
		}
	}
	if relID == "" {
		return nil
	}
	data, name, ok := imp.mediaFor(relID)
	if !ok {
		return nil
	}
	placeholder, seen := imp.assetByID[relID]
	if !seen {
		placeholder = richdoc.Placeholder(len(imp.assets) + 1)
		imp.assets = append(imp.assets, richdoc.Asset{
			Placeholder: placeholder,
			Name:        name,
			MediaType:   mediaContentType(strings.TrimPrefix(strings.ToLower(pathExt(name)), ".")),
			Data:        data,
		})
		imp.assetByID[relID] = placeholder
	}
	image := &richdoc.Node{Type: "image"}
	image.SetAttr("src", placeholder)
	alt := ""
	if docPr := node.descendant("wp", "docPr"); docPr != nil {
		alt = docPr.attr("descr")
		if alt == "" {
			alt = docPr.attr("name")
		}
	}
	if alt != "" {
		image.SetAttr("alt", alt)
	}
	if extent := node.descendant("wp", "extent"); extent != nil {
		if cx, err := strconv.Atoi(extent.attr("cx")); err == nil && cx > 0 {
			image.SetAttr("width", cx/emuPerPixel)
		}
	}
	return image
}

func pathExt(name string) string {
	if index := strings.LastIndex(name, "."); index >= 0 {
		return name[index:]
	}
	return ""
}

// mergeAdjacentText collapses the many small runs Word emits (it splits on
// spell-check and revision boundaries) back into readable text nodes.
func mergeAdjacentText(nodes []*richdoc.Node) []*richdoc.Node {
	out := make([]*richdoc.Node, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if len(out) > 0 && node.Type == "text" && out[len(out)-1].Type == "text" && sameMarks(out[len(out)-1].Marks, node.Marks) {
			out[len(out)-1].Text += node.Text
			continue
		}
		out = append(out, node)
	}
	return out
}

func sameMarks(left, right []richdoc.Mark) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Type != right[index].Type || len(left[index].Attrs) != len(right[index].Attrs) {
			return false
		}
		for key, value := range left[index].Attrs {
			if fmt.Sprint(right[index].Attrs[key]) != fmt.Sprint(value) {
				return false
			}
		}
	}
	return true
}

// alternateContent picks the branch of an mc:AlternateContent to read.
//
// The element offers the same thing twice: an mc:Choice for readers that
// understand some extension, and an mc:Fallback for the rest. muni is one of
// the rest. Reading both would put every shape's words in the document twice.
func alternateContent(node *xnode) []*xnode {
	// Fallback first, but only if there is anything in it. Some producers
	// write <mc:Fallback/> empty, and preferring nothing over the Choice
	// throws away the only copy of the shape's words.
	if fallback := node.child("mc", "Fallback"); len(fallback.children()) > 0 {
		return fallback.Children
	}
	if choice := node.child("mc", "Choice"); choice != nil {
		return choice.Children
	}
	return nil
}

// shapes reads what muni can hold out of the things Word draws: a picture, or
// the words in a text box. A shape is often wrapped one layer deeper than the
// run — an mc:Fallback holding a w:pict — so this takes a list of candidates
// rather than one node.
func (imp *importer) shapes(nodes []*xnode, marks []richdoc.Mark) []*richdoc.Node {
	out := []*richdoc.Node{}
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if image := imp.image(node); image != nil {
			out = append(out, image)
			continue
		}
		out = append(out, textBoxWords(node, marks)...)
	}
	return out
}

// textBoxWords pulls the words out of a text box.
//
// A Korean office document keeps things in text boxes that the page cannot do
// without: the 붙임 label, the stamp beside a signature, a note in the margin.
// muni has no box to put them in, and the drawing they arrive in is not a
// picture — so muni found no image, kept nothing, and left an empty paragraph
// where the words had been.
//
// The words come across as words. A box holding several paragraphs keeps its
// lines, because a two-line stamp read as one line is a different stamp.
func textBoxWords(node *xnode, marks []richdoc.Mark) []*richdoc.Node {
	out := []*richdoc.Node{}
	// Every box, not the first. A 직인 grouped with a 붙임 label is one drawing
	// holding two of them, and keeping one is keeping half the stamp.
	for _, content := range descendants(node, "w", "txbxContent") {
		for _, child := range content.Children {
			if !child.is("w", "p") {
				continue
			}
			text := collapseSpaces(child.allText())
			if text == "" {
				continue
			}
			if len(out) > 0 {
				out = append(out, &richdoc.Node{Type: "hardBreak"})
			}
			out = append(out, richdoc.Text(text, marks...))
		}
	}
	return out
}
