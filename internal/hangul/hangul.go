// Package hangul holds what the two Hangul Office formats agree on.
//
// A .hwp and a .hwpx say the same things in different ways — one in binary
// records, the other in XML — but they measure in the same units and name
// their outline styles the same. Reading each of those twice is how the two
// readers would drift apart.
package hangul

import (
	"strconv"
	"strings"
)

// UnitsPerInch is the HWPUNIT: a Hangul document measures in sevenths of a
// thousandth of an inch.
const UnitsPerInch = 7200

// IndentSteps turns a left margin in HWPUNIT into the steps muni's editor
// draws, counting a step as a quarter inch — the same width the .docx reader
// settled on, so a document that passed through Word and a document that did
// not are indented alike.
func IndentSteps(units int) int {
	if units <= 0 {
		return 0
	}
	steps := (units*4 + UnitsPerInch/2) / UnitsPerInch
	if steps > 16 {
		steps = 16
	}
	return steps
}

// PixelWidth turns a width in HWPUNIT into the pixels muni's editor holds a
// table column in — ninety-six to the inch, the scale the .docx reader
// converts twips at, so a table that came through Word and a table that came
// through Hangul are held the same way.
//
// A width past a hundred inches is not a column of a real table; it is a file
// meaning something else by those bytes.
func PixelWidth(units int) int {
	if units <= 0 || units > 100*UnitsPerInch {
		return 0
	}
	return int(float64(units)/UnitsPerInch*96 + 0.5)
}

// ColumnWidths is what a cell keeps for the columns it covers: one width per
// column, and nothing at all unless every column it covers has one, because
// muni's editor reads the list as a whole.
//
// Both formats give a merged cell's width as the total across the columns it
// covers, so the columns themselves are learnt from the cells that cover one
// apiece — which is why this takes a table's columns rather than a cell's own
// width.
func ColumnWidths(pixels map[int]int, column, span int) []any {
	if len(pixels) == 0 || span < 1 {
		return nil
	}
	out := make([]any, 0, span)
	for offset := 0; offset < span; offset++ {
		width := pixels[column+offset]
		if width <= 0 {
			return nil
		}
		out = append(out, width)
	}
	return out
}

// Alignment names a paragraph's alignment the way muni does.
func Alignment(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "CENTER":
		return "center"
	case "RIGHT":
		return "right"
	case "JUSTIFY", "BOTH", "DISTRIBUTE", "DISTRIBUTE_SPACE":
		return "justify"
	}
	return ""
}

// AlignmentCode names the alignment a .hwp stores as a number. The order is
// the format's own.
func AlignmentCode(code uint32) string {
	switch code {
	case 0:
		return "justify"
	case 2:
		return "right"
	case 3:
		return "center"
	case 4, 5:
		return "justify"
	}
	// 1 is left, which is muni's own default and worth nothing to record.
	return ""
}

// OutlineLevel reads a style name as an outline level.
//
// Hangul's built-in outline styles are 개요 1..7, and a document translated
// from Word carries "Heading 1". Both say the same thing, and a style named
// neither is body text however it is formatted.
func OutlineLevel(names ...string) int {
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		// "Outline N" is what Hangul itself writes as the English name of
		// 개요 N. Real files name the level in the English name alone — "본문
		// 대제목 / Outline 1" — so without it those headings were body text.
		for _, prefix := range []string{"개요 ", "개요", "outline ", "outline", "heading ", "heading", "제목 "} {
			if !strings.HasPrefix(lower, strings.ToLower(prefix)) {
				continue
			}
			rest := strings.TrimSpace(trimmed[len(prefix):])
			// Hangul goes to 개요 7 and muni's heading goes to 6, so the
			// seventh becomes the sixth rather than becoming body text.
			if level, err := strconv.Atoi(rest); err == nil && level >= 1 && level <= 7 {
				if level > 6 {
					level = 6
				}
				return level
			}
		}
	}
	return 0
}

// FontSize turns a character height into the size muni's editor draws. Both
// formats write it in hundredths of a point — the .hwpx as an attribute, the
// .hwp as the base size in its CHAR_SHAPE — so ten point, Hangul's own
// default, is worth nothing to record.
func FontSize(hundredths int) string {
	if hundredths <= 0 {
		return ""
	}
	point := float64(hundredths) / 100
	if point == 10 {
		return ""
	}
	return strconv.FormatFloat(point, 'f', -1, 64) + "pt"
}

// Script names the vertical script a run is set in, the way muni marks it.
//
// Both formats can say raised and lowered at once — the .hwp in two property
// bits, the .hwpx in two elements — and muni's editor can draw only one of
// them, so the raised one wins rather than the two cancelling into body text.
func Script(superscript, subscript bool) string {
	switch {
	case superscript:
		return "superscript"
	case subscript:
		return "subscript"
	}
	return ""
}

// LineHeight turns a spacing percentage into the ratio muni holds. A hundred
// per cent is single spacing, which muni draws without being told.
func LineHeight(percent int) string {
	if percent <= 0 || percent == 100 {
		return ""
	}
	ratio := float64(percent) / 100
	if ratio < 0.5 || ratio > 5 {
		return ""
	}
	return strconv.FormatFloat(ratio, 'f', -1, 64)
}
