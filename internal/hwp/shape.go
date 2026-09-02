package hwp

import (
	"encoding/binary"
	"unicode/utf16"

	"github.com/hkjang/muni/internal/hangul"
)

// A paragraph in a .hwp says which shape and which style it wears, by number.
// Both live in DocInfo, and reading only the body finds a document with no
// alignment, no indentation and no headings — which is most of what a Korean
// report's layout is.

type paraShape struct {
	align     string
	indent    int
	firstLine bool
	// list is the kind of list a paragraph with this shape is an item of —
	// "bulletList" or "orderedList" — and level how deep; "" is no list.
	list  string
	level int
}

// readParaShape reads one PARA_SHAPE record.
//
// The first word carries the alignment in bits 2 to 4; the margins and the
// first-line indent follow it as signed HWPUNIT.
func readParaShape(raw []byte) paraShape {
	if len(raw) < 16 {
		return paraShape{}
	}
	property := binary.LittleEndian.Uint32(raw[0:])
	shape := paraShape{align: hangul.AlignmentCode((property >> 2) & 0x07)}
	shape.indent = hangul.IndentSteps(int(int32(binary.LittleEndian.Uint32(raw[4:]))))
	// A positive indent is a first line set in from the margin; a negative one
	// is a hanging indent, which muni draws as an indented paragraph instead.
	if first := int32(binary.LittleEndian.Uint32(raw[12:])); first > 0 {
		shape.firstLine = true
	}
	// Bits 23 and 24 say what heads the paragraph — nothing, an outline
	// number, a list number or a bullet — and 25 to 27 how deep it sits. A
	// .hwp has no list element: a list is a run of paragraphs that say so.
	switch (property >> 23) & 0x03 {
	case 2:
		shape.list = "orderedList"
	case 3:
		shape.list = "bulletList"
	}
	shape.level = int((property >> 25) & 0x07)
	return shape
}

type styleInfo struct {
	name         string
	englishName  string
	headingLevel int
}

// readStyle reads one STYLE record, which begins with the style's two names.
//
// Each is a length in code units followed by the characters, so the English
// name cannot be found without reading past the Korean one first.
func readStyle(raw []byte) styleInfo {
	korean, offset := readWideString(raw, 0)
	english, _ := readWideString(raw, offset)
	info := styleInfo{name: korean, englishName: english}
	info.headingLevel = hangul.OutlineLevel(korean, english)
	return info
}

// readWideString reads a length-prefixed UTF-16 string and reports where it
// ended.
func readWideString(raw []byte, offset int) (string, int) {
	if offset+2 > len(raw) {
		return "", offset
	}
	length := int(binary.LittleEndian.Uint16(raw[offset:]))
	offset += 2
	if length < 0 || offset+length*2 > len(raw) {
		return "", offset
	}
	units := make([]uint16, 0, length)
	for index := 0; index < length; index++ {
		units = append(units, binary.LittleEndian.Uint16(raw[offset+index*2:]))
	}
	return string(utf16.Decode(units)), offset + length*2
}

// paragraphRefs is which shape and style a PARA_HEADER names.
//
// The layout is a character count, a control mask, then the two numbers.
type paragraphRefs struct {
	shape uint16
	style uint8
}

func readParagraphRefs(raw []byte) paragraphRefs {
	if len(raw) < 11 {
		return paragraphRefs{}
	}
	return paragraphRefs{
		shape: binary.LittleEndian.Uint16(raw[8:]),
		style: raw[10],
	}
}
