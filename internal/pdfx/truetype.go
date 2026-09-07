package pdfx

// An embedded font carries its own character map, and it is the only place
// that says what a glyph really is.
//
// A subset font is addressed by glyph number, and the producer is supposed to
// declare what each glyph means in a /ToUnicode map. Producers that cannot
// name a glyph write the glyph's own number there instead — the space in a
// Chromium PDF arrives as U+0001, and in a font whose space happens to sit at
// glyph 36 it arrives as "$". Reading the font's cmap answers the question
// properly: glyph 36 is the space, whatever the producer claimed.

// glyphMap is a font's own answer: glyph number to the text it draws.
type glyphMap map[uint32]string

func read16(data []byte, at int) (uint32, bool) {
	if at < 0 || at+2 > len(data) {
		return 0, false
	}
	return uint32(data[at])<<8 | uint32(data[at+1]), true
}

func read32(data []byte, at int) (uint32, bool) {
	if at < 0 || at+4 > len(data) {
		return 0, false
	}
	return uint32(data[at])<<24 | uint32(data[at+1])<<16 | uint32(data[at+2])<<8 | uint32(data[at+3]), true
}

// parseGlyphMap reads the cmap of an embedded TrueType or OpenType font and
// turns it inside out: the file maps characters to glyphs, and a reader of a
// subset font needs the other direction.
func parseGlyphMap(data []byte) glyphMap {
	table, ok := findTable(data, "cmap")
	if !ok {
		return nil
	}
	subtable, ok := bestSubtable(data, table)
	if !ok {
		return nil
	}
	pairs := readSubtable(data, subtable)
	if len(pairs) == 0 {
		return nil
	}
	out := make(glyphMap, len(pairs))
	for _, pair := range pairs {
		// The first character to claim a glyph keeps it: a font often maps
		// several characters to one glyph, and the lowest is the plain one.
		if _, taken := out[pair.glyph]; taken {
			continue
		}
		if !printableRune(rune(pair.code)) {
			continue
		}
		out[pair.glyph] = string(rune(pair.code))
	}
	return out
}

// findTable locates one sfnt table by its four-letter tag.
func findTable(data []byte, tag string) (int, bool) {
	if len(data) < 12 {
		return 0, false
	}
	offset := 0
	// A font collection holds several fonts; the first one will do.
	if string(data[0:4]) == "ttcf" {
		first, ok := read32(data, 12)
		if !ok || int(first) >= len(data) {
			return 0, false
		}
		offset = int(first)
	}
	count, ok := read16(data, offset+4)
	if !ok || count > 512 {
		return 0, false
	}
	for index := 0; index < int(count); index++ {
		record := offset + 12 + index*16
		if record+16 > len(data) {
			return 0, false
		}
		if string(data[record:record+4]) != tag {
			continue
		}
		start, ok := read32(data, record+8)
		if !ok || int(start) >= len(data) {
			return 0, false
		}
		return int(start), true
	}
	return 0, false
}

// bestSubtable picks the cmap a text reader should believe: full Unicode
// first, then the basic-plane Windows table, then anything that is left.
func bestSubtable(data []byte, table int) (int, bool) {
	count, ok := read16(data, table+2)
	if !ok || count > 128 {
		return 0, false
	}
	best, bestRank := 0, -1
	for index := 0; index < int(count); index++ {
		record := table + 4 + index*8
		platform, ok1 := read16(data, record)
		encoding, ok2 := read16(data, record+2)
		offset, ok3 := read32(data, record+4)
		if !ok1 || !ok2 || !ok3 {
			break
		}
		at := table + int(offset)
		if at < 0 || at >= len(data) {
			continue
		}
		rank := -1
		switch {
		case platform == 3 && encoding == 10:
			rank = 5
		case platform == 0 && encoding >= 4:
			rank = 4
		case platform == 3 && encoding == 1:
			rank = 3
		case platform == 0:
			rank = 2
		case platform == 3 && encoding == 0:
			rank = 1
		case platform == 1 && encoding == 0:
			rank = 0
		}
		if rank > bestRank {
			best, bestRank = at, rank
		}
	}
	return best, bestRank >= 0
}

type codeGlyph struct {
	code  uint32
	glyph uint32
}

// maxGlyphPairs bounds the work a crafted font can cause.
const maxGlyphPairs = 200000

func readSubtable(data []byte, at int) []codeGlyph {
	format, ok := read16(data, at)
	if !ok {
		return nil
	}
	switch format {
	case 0:
		return readFormat0(data, at)
	case 4:
		return readFormat4(data, at)
	case 6:
		return readFormat6(data, at)
	case 12:
		return readFormat12(data, at)
	}
	return nil
}

func readFormat0(data []byte, at int) []codeGlyph {
	if at+6+256 > len(data) {
		return nil
	}
	out := make([]codeGlyph, 0, 256)
	for code := 0; code < 256; code++ {
		if glyph := uint32(data[at+6+code]); glyph != 0 {
			out = append(out, codeGlyph{uint32(code), glyph})
		}
	}
	return out
}

func readFormat4(data []byte, at int) []codeGlyph {
	doubled, ok := read16(data, at+6)
	if !ok || doubled == 0 || doubled%2 == 1 {
		return nil
	}
	segments := int(doubled / 2)
	endAt := at + 14
	startAt := endAt + segments*2 + 2
	deltaAt := startAt + segments*2
	rangeAt := deltaAt + segments*2
	if rangeAt+segments*2 > len(data) {
		return nil
	}
	out := make([]codeGlyph, 0, segments*4)
	for index := 0; index < segments; index++ {
		end, _ := read16(data, endAt+index*2)
		start, _ := read16(data, startAt+index*2)
		delta, _ := read16(data, deltaAt+index*2)
		rangeOffset, _ := read16(data, rangeAt+index*2)
		if start > end || end == 0xFFFF && start == 0xFFFF {
			continue
		}
		for code := start; code <= end; code++ {
			var glyph uint32
			if rangeOffset == 0 {
				glyph = (code + delta) & 0xFFFF
			} else {
				// The offset is counted in bytes from the entry itself, the
				// one piece of this format that catches every reader out.
				entry := rangeAt + index*2 + int(rangeOffset) + int(code-start)*2
				value, ok := read16(data, entry)
				if !ok || value == 0 {
					continue
				}
				glyph = (value + delta) & 0xFFFF
			}
			if glyph != 0 {
				out = append(out, codeGlyph{code, glyph})
			}
			if len(out) >= maxGlyphPairs || code == 0xFFFF {
				break
			}
		}
		if len(out) >= maxGlyphPairs {
			break
		}
	}
	return out
}

func readFormat6(data []byte, at int) []codeGlyph {
	first, ok1 := read16(data, at+6)
	count, ok2 := read16(data, at+8)
	if !ok1 || !ok2 || count > maxGlyphPairs {
		return nil
	}
	out := make([]codeGlyph, 0, count)
	for index := 0; index < int(count); index++ {
		glyph, ok := read16(data, at+10+index*2)
		if !ok {
			break
		}
		if glyph != 0 {
			out = append(out, codeGlyph{first + uint32(index), glyph})
		}
	}
	return out
}

func readFormat12(data []byte, at int) []codeGlyph {
	groups, ok := read32(data, at+12)
	if !ok || groups > 100000 {
		return nil
	}
	out := make([]codeGlyph, 0, groups)
	for index := 0; index < int(groups); index++ {
		record := at + 16 + index*12
		start, ok1 := read32(data, record)
		end, ok2 := read32(data, record+4)
		glyph, ok3 := read32(data, record+8)
		if !ok1 || !ok2 || !ok3 || end < start || end-start > maxGlyphPairs {
			break
		}
		for code := start; code <= end; code++ {
			out = append(out, codeGlyph{code, glyph + (code - start)})
			if len(out) >= maxGlyphPairs {
				return out
			}
		}
	}
	return out
}
