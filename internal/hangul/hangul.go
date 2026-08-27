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
		for _, prefix := range []string{"개요 ", "개요", "heading ", "heading", "제목 "} {
			if !strings.HasPrefix(lower, strings.ToLower(prefix)) {
				continue
			}
			rest := strings.TrimSpace(trimmed[len(prefix):])
			if level, err := strconv.Atoi(rest); err == nil && level >= 1 && level <= 6 {
				return level
			}
		}
	}
	return 0
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
