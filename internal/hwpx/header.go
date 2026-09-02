package hwpx

import (
	"archive/zip"
	"strconv"
	"strings"

	"github.com/hkjang/muni/internal/hangul"
)

// loadHeader reads Contents/header.xml, where the formatting a run or a
// paragraph refers to by id actually lives.
//
// A run in HWPX says charPrIDRef="7" and nothing else. Reading only the run
// finds no formatting at all — the same shape that made muni lose every Word
// style, one format over.
func (imp *importer) loadHeader(files map[string]*zip.File) {
	file := findPart(files, "contents/header.xml")
	if file == nil {
		return
	}
	root, err := readPart(file)
	if err != nil {
		return
	}
	root.each("charPr", func(current *node) {
		id := current.attr("id")
		if id == "" {
			return
		}
		imp.charShapes[id] = readCharShape(current)
	})
	root.each("paraPr", func(current *node) {
		id := current.attr("id")
		if id == "" {
			return
		}
		imp.paraShapes[id] = readParaShape(current)
	})
	root.each("style", func(current *node) {
		id := current.attr("id")
		if id == "" {
			return
		}
		info := styleInfo{
			name:        current.attr("name"),
			englishName: current.attr("engName"),
			paraShapeID: current.attr("paraPrIDRef"),
			charShapeID: current.attr("charPrIDRef"),
		}
		info.headingLevel = hangul.OutlineLevel(info.name, info.englishName)
		imp.styles[id] = info
	})
}

// readCharShape reads one charPr. A property is on when its element is there
// and does not say otherwise — Hangul writes <hh:bold/> for bold and leaves it
// out for not-bold.
func readCharShape(current *node) charShape {
	shape := charShape{
		bold:      current.child("bold") != nil,
		italic:    current.child("italic") != nil,
		strike:    current.child("strikeout") != nil,
		underline: underlineIsDrawn(current.child("underline")),
	}
	if color := normalizeColor(current.attr("textColor")); color != "" {
		shape.color = color
	}
	// height is in 1/100 pt.
	if height, err := strconv.Atoi(strings.TrimSpace(current.attr("height"))); err == nil && height > 0 {
		point := float64(height) / 100
		if point != 10 {
			shape.sizePoint = strconv.FormatFloat(point, 'f', -1, 64) + "pt"
		}
	}
	if font := current.descendant("fontRef"); font != nil {
		// hangul first: muni's documents are Korean, and that attribute names
		// the face the Hangul is set in.
		shape.family = firstNonEmpty(font.attr("hangul"), font.attr("latin"))
	}
	return shape
}

// underlineIsDrawn reports whether an underline element actually draws one.
// Hangul writes type="NONE" rather than leaving the element out.
func underlineIsDrawn(current *node) bool {
	if current == nil {
		return false
	}
	switch strings.ToUpper(strings.TrimSpace(current.attr("type"))) {
	case "", "NONE":
		return false
	}
	return true
}

func readParaShape(current *node) paraShape {
	shape := paraShape{}
	if align := current.child("align"); align != nil {
		shape.align = hangul.Alignment(align.attr("horizontal"))
	} else {
		shape.align = hangul.Alignment(current.attr("align"))
	}
	if margin := current.child("margin"); margin != nil {
		if left := margin.child("left"); left != nil {
			shape.indent = indentSteps(left.attr("value"))
		}
		// The format spells the first-line indent "intent" — Hancom's own
		// spelling, kept for as long as the format has existed. Looking for
		// "indent" finds it in no real file. Both are accepted, because a
		// fixture written to muni's reader rather than to the format is how
		// this went unnoticed.
		for _, name := range []string{"intent", "indent"} {
			if first := margin.child(name); first != nil {
				if value, err := strconv.Atoi(strings.TrimSpace(first.attr("value"))); err == nil && value > 0 {
					shape.firstLin = true
				}
			}
		}
	}
	if spacing := current.child("lineSpacing"); spacing != nil {
		shape.lineRate = lineHeightOf(spacing)
	}
	if heading := current.child("heading"); heading != nil &&
		strings.EqualFold(strings.TrimSpace(heading.attr("type")), "OUTLINE") {
		// Zero-based in the file; muni's heading levels start at one.
		if level, err := strconv.Atoi(strings.TrimSpace(heading.attr("level"))); err == nil && level >= 0 && level < 6 {
			shape.outline = level + 1
		}
	}
	return shape
}

// indentSteps turns a left margin in HWPUNIT (1/7200 inch) into the steps
// muni's editor draws. Two steps is one Word indent level, which is what the
// .docx importer settled on.
func indentSteps(value string) int {
	units, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return hangul.IndentSteps(units)
}

// lineHeightOf reads a line spacing muni can hold. Hangul writes PERCENT with
// a value of 160 for 1.6 lines.
func lineHeightOf(spacing *node) string {
	if !strings.EqualFold(strings.TrimSpace(spacing.attr("type")), "PERCENT") {
		return ""
	}
	percent, err := strconv.Atoi(strings.TrimSpace(spacing.attr("value")))
	if err != nil {
		return ""
	}
	return hangul.LineHeight(percent)
}

func normalizeColor(value string) string {
	trimmed := strings.ToUpper(strings.TrimSpace(value))
	trimmed = strings.TrimPrefix(trimmed, "#")
	if len(trimmed) != 6 {
		return ""
	}
	for _, r := range trimmed {
		if !((r >= '0' && r <= '9') || (r >= 'A' && r <= 'F')) {
			return ""
		}
	}
	if trimmed == "000000" {
		return ""
	}
	return "#" + trimmed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// findPart looks a part up without caring how the writer cased its name.
func findPart(files map[string]*zip.File, want string) *zip.File {
	for name, file := range files {
		if strings.EqualFold(name, want) {
			return file
		}
	}
	return nil
}
