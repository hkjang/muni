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

// paragraphText is a paragraph's characters with the positions the file
// counts them by.
//
// Those positions are what a PARA_CHAR_SHAPE record refers to, and they count
// every code unit the record held — including the eight that a control mark
// occupies. Counting only the characters that survived would put every shape
// after the first table or picture on the wrong words.
type paragraphText struct {
	text string
	// runes[i] is the file's own position for text[runes[i]:].
	pieces []textPiece
	// controls counts the object marks passed, so a paragraph can be matched
	// with the records that follow it.
	controls int
}

// textPiece is a stretch of characters and where the file counts it from.
type textPiece struct {
	at   uint32 // position in code units, as the file counts them
	text string
}

func readParagraphText(raw []byte) paragraphText {
	out := paragraphText{}
	var current strings.Builder
	units := make([]uint16, 0, 8)
	position := uint32(0)
	pieceAt := uint32(0)
	flushUnits := func() {
		if len(units) > 0 {
			current.WriteString(string(utf16.Decode(units)))
			units = units[:0]
		}
	}
	flushPiece := func() {
		flushUnits()
		if current.Len() > 0 {
			out.pieces = append(out.pieces, textPiece{at: pieceAt, text: current.String()})
			current.Reset()
		}
	}
	for offset := 0; offset+2 <= len(raw); {
		code := binary.LittleEndian.Uint16(raw[offset:])
		if code >= 32 {
			if len(units) == 0 && current.Len() == 0 {
				pieceAt = position
			}
			units = append(units, code)
			offset += 2
			position++
			continue
		}
		if replacement, ok := inlineControls[code]; ok && !extendedControl(code) {
			if len(units) == 0 && current.Len() == 0 {
				pieceAt = position
			}
			flushUnits()
			current.WriteString(replacement)
			offset += 2
			position++
			continue
		}
		if extendedControl(code) {
			// Eight WCHAR including this one — the erratum that costs a reader
			// the rest of the paragraph if it is read as eight bytes.
			//
			// The piece ends here either way. A tab keeps its character, but
			// the eight positions it occupies cannot be counted from inside a
			// piece — the shape lookup walks a piece one code unit per rune,
			// so everything after a tab would be looked up seven positions
			// early and wear the wrong shape.
			replacement, keep := inlineControls[code]
			flushPiece()
			if keep {
				out.pieces = append(out.pieces, textPiece{at: position, text: replacement})
			} else {
				out.controls++
			}
			offset += controlWidth * 2
			position += controlWidth
			pieceAt = position
			continue
		}
		// A character control that says nothing muni can keep.
		offset += 2
		position++
	}
	flushPiece()
	for _, piece := range out.pieces {
		out.text += piece.text
	}
	return out
}
