package hwp

import (
	"encoding/binary"
	"strings"
	"unicode/utf16"
)

// Paragraph text is UTF-16LE, and the characters below 32 are not characters.
// Some stand for something a reader can keep — a tab, a line break — and the
// rest mark where a control sits: a table, a picture, a footnote, whose own
// records follow the paragraph.
//
// The trap the format's own documentation sets is the size of those marks. The
// spec says 8; it means 8 WCHAR — sixteen bytes, not eight — and a reader that
// skips eight bytes lands in the middle of the mark and reads its second half
// as text. What comes out is a document sprinkled with CJK ideographs that
// were never in it.
const controlWidth = 8 // WCHAR, per the spec's own erratum

// inlineControls stand for a character. Everything else below 32 that is not
// listed here marks a control object and is skipped whole.
var inlineControls = map[uint16]string{
	9:  "\t",
	10: "\n",
	13: "\n",
	24: "-", // hyphen
	30: " ", // fixed-width space
	31: " ",
}

// extendedControl reports whether a code point is one of the marks written as
// eight WCHAR rather than one.
func extendedControl(code uint16) bool {
	switch code {
	case 1, 2, 3, 11, 12, 14, 15, 16, 17, 18, 21, 22, 23:
		return true
	// The inline object marks — table, picture, equation — are the same width.
	case 4, 5, 6, 7, 8, 9, 19, 20:
		return true
	}
	return false
}

// paragraphText reads one PARA_TEXT record into the words it holds and the
// controls it points at, in the order a reader meets them.
type paragraphText struct {
	text string
	// controls counts the object marks passed, so a paragraph can be matched
	// with the records that follow it.
	controls int
}

func readParagraphText(raw []byte) paragraphText {
	var out strings.Builder
	controls := 0
	units := make([]uint16, 0, 8)
	flush := func() {
		if len(units) > 0 {
			out.WriteString(string(utf16.Decode(units)))
			units = units[:0]
		}
	}
	for offset := 0; offset+2 <= len(raw); {
		code := binary.LittleEndian.Uint16(raw[offset:])
		if code >= 32 {
			units = append(units, code)
			offset += 2
			continue
		}
		flush()
		if replacement, ok := inlineControls[code]; ok && !extendedControl(code) {
			out.WriteString(replacement)
			offset += 2
			continue
		}
		if extendedControl(code) {
			// Eight WCHAR including this one — the erratum that costs a reader
			// the rest of the paragraph if it is read as eight bytes.
			if replacement, ok := inlineControls[code]; ok {
				out.WriteString(replacement)
			} else {
				controls++
			}
			offset += controlWidth * 2
			continue
		}
		// A character control that says nothing muni can keep.
		offset += 2
	}
	flush()
	return paragraphText{text: out.String(), controls: controls}
}
