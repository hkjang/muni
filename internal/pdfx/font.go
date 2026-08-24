package pdfx

import (
	"strconv"
	"strings"
	"unicode/utf16"
)

type codespace struct {
	low    uint32
	high   uint32
	length int
}

type fontInfo struct {
	name         string
	twoByte      bool
	codespaces   []codespace
	toUnicode    map[uint32]string
	encoding     map[byte]string // character code -> Unicode text
	widths       map[uint32]float64
	defaultWidth float64
	bold         bool
	italic       bool
	mono         bool
	ucs2CMap     bool
	identityCID  bool
	spaceCode    uint32
	hasSpaceCode bool
}

func (d *Document) loadFont(dict Dict) *fontInfo {
	font := &fontInfo{
		toUnicode:    map[uint32]string{},
		encoding:     map[byte]string{},
		widths:       map[uint32]float64{},
		defaultWidth: 0.5,
		spaceCode:    32,
		hasSpaceCode: true,
	}
	if dict == nil {
		return font
	}
	subtype := d.name(dict.get("Subtype"))
	base := d.name(dict.get("BaseFont"))
	font.name = base
	lower := strings.ToLower(base)
	font.bold = strings.Contains(lower, "bold") || strings.Contains(lower, "black") || strings.Contains(lower, "heavy")
	font.italic = strings.Contains(lower, "italic") || strings.Contains(lower, "oblique")
	font.mono = isMonospaceName(lower)

	if subtype == "Type0" {
		font.twoByte = true
		font.hasSpaceCode = false
		switch encoding := d.resolve(dict.get("Encoding")).(type) {
		case Name:
			name := string(encoding)
			font.identityCID = strings.HasPrefix(name, "Identity")
			font.ucs2CMap = strings.Contains(name, "UCS2")
		case *Stream:
			if data, err := d.StreamData(encoding); err == nil {
				font.readCMapCodespaces(data)
			}
		}
		descendants := d.array(dict.get("DescendantFonts"))
		if len(descendants) > 0 {
			descendant := d.dict(descendants[0])
			font.defaultWidth = d.number(descendant.get("DW"), 1000) / 1000
			font.readCIDWidths(d, d.array(descendant.get("W")))
		}
	} else {
		font.readSimpleEncoding(d, dict)
		font.readSimpleWidths(d, dict)
		if strings.Contains(lower, "symbol") || strings.Contains(lower, "dingbat") {
			font.hasSpaceCode = false
		}
	}

	if unicodeStream, ok := d.resolve(dict.get("ToUnicode")).(*Stream); ok {
		if data, err := d.StreamData(unicodeStream); err == nil {
			font.readToUnicode(data)
		}
	}
	return font
}

func (f *fontInfo) readSimpleEncoding(d *Document, dict Dict) {
	baseTable := winAnsiEncoding
	switch encoding := d.resolve(dict.get("Encoding")).(type) {
	case Name:
		baseTable = encodingTable(string(encoding))
	case Dict:
		if name := d.name(encoding.get("BaseEncoding")); name != "" {
			baseTable = encodingTable(name)
		}
		differences := d.array(encoding.get("Differences"))
		code := 0
		for _, item := range differences {
			switch typed := d.resolve(item).(type) {
			case int64:
				code = int(typed)
			case float64:
				code = int(typed)
			case Name:
				if code >= 0 && code < 256 {
					if value := glyphToUnicode(string(typed)); value != "" {
						f.encoding[byte(code)] = value
					}
				}
				code++
			}
		}
	}
	for code, value := range baseTable {
		if value == "" {
			continue
		}
		if _, ok := f.encoding[byte(code)]; !ok {
			f.encoding[byte(code)] = value
		}
	}
}

func (f *fontInfo) readSimpleWidths(d *Document, dict Dict) {
	firstChar, _ := toInt(d.resolve(dict.get("FirstChar")))
	widths := d.array(dict.get("Widths"))
	for index, item := range widths {
		if value, ok := toFloat(d.resolve(item)); ok && value > 0 {
			f.widths[uint32(firstChar+index)] = value / 1000
		}
	}
	if descriptor := d.dict(dict.get("FontDescriptor")); descriptor != nil {
		if missing := d.number(descriptor.get("MissingWidth"), 0); missing > 0 {
			f.defaultWidth = missing / 1000
		}
		if flags, ok := toInt(d.resolve(descriptor.get("Flags"))); ok {
			if flags&(1<<18) != 0 {
				f.bold = true
			}
			if flags&(1<<6) != 0 {
				f.italic = true
			}
		}
	}
	if len(f.widths) == 0 {
		f.defaultWidth = 0.5
	}
}

func (f *fontInfo) readCIDWidths(d *Document, widths Array) {
	index := 0
	for index < len(widths) {
		start, ok := toInt(d.resolve(widths[index]))
		if !ok {
			index++
			continue
		}
		if index+1 >= len(widths) {
			break
		}
		switch next := d.resolve(widths[index+1]).(type) {
		case Array:
			for offset, item := range next {
				if value, ok := toFloat(d.resolve(item)); ok {
					f.widths[uint32(start+offset)] = value / 1000
				}
			}
			index += 2
		default:
			end, _ := toInt(next)
			if index+2 >= len(widths) {
				return
			}
			value, _ := toFloat(d.resolve(widths[index+2]))
			if end >= start && end-start < 65536 {
				for code := start; code <= end; code++ {
					f.widths[uint32(code)] = value / 1000
				}
			}
			index += 3
		}
	}
}

// readCMapCodespaces learns the byte lengths an embedded CMap uses so mixed
// one- and two-byte encodings decode correctly.
func (f *fontInfo) readCMapCodespaces(data []byte) {
	parser := newParser(data, 0)
	pending := make([]token, 0, 2)
	for {
		current := parser.read()
		if current.kind == tokenEOF {
			return
		}
		if current.kind == tokenKeyword && current.text == "endcodespacerange" {
			for index := 0; index+1 < len(pending); index += 2 {
				low, lowLength := codeFromBytes(pending[index].bytes)
				high, _ := codeFromBytes(pending[index+1].bytes)
				f.codespaces = append(f.codespaces, codespace{low: low, high: high, length: lowLength})
			}
			pending = pending[:0]
			continue
		}
		if current.kind == tokenKeyword && current.text == "begincodespacerange" {
			pending = pending[:0]
			continue
		}
		if current.kind == tokenString {
			pending = append(pending, current)
			if len(pending) > 128 {
				pending = pending[:0]
			}
		}
	}
}

func codeFromBytes(data []byte) (uint32, int) {
	value := uint32(0)
	for _, item := range data {
		value = value<<8 | uint32(item)
	}
	return value, len(data)
}

// readToUnicode parses the bfchar/bfrange sections of a ToUnicode CMap.
func (f *fontInfo) readToUnicode(data []byte) {
	parser := newParser(data, 0)
	mode := ""
	pending := make([]Object, 0, 3)
	flush := func() {
		switch mode {
		case "bfchar":
			for index := 0; index+1 < len(pending); index += 2 {
				source, ok := pending[index].(String)
				if !ok {
					continue
				}
				code, _ := codeFromBytes(source)
				f.toUnicode[code] = decodeUTF16BE(pending[index+1])
			}
		case "bfrange":
			for index := 0; index+2 < len(pending); index += 3 {
				low, ok1 := pending[index].(String)
				high, ok2 := pending[index+1].(String)
				if !ok1 || !ok2 {
					continue
				}
				start, _ := codeFromBytes(low)
				end, _ := codeFromBytes(high)
				if end < start || end-start > 65535 {
					continue
				}
				switch target := pending[index+2].(type) {
				case Array:
					for offset := uint32(0); offset <= end-start && int(offset) < len(target); offset++ {
						f.toUnicode[start+offset] = decodeUTF16BE(target[offset])
					}
				case String:
					value := decodeUTF16BE(target)
					runes := []rune(value)
					if len(runes) == 0 {
						continue
					}
					for offset := uint32(0); offset <= end-start; offset++ {
						next := make([]rune, len(runes))
						copy(next, runes)
						next[len(next)-1] += rune(offset)
						f.toUnicode[start+offset] = string(next)
					}
				}
			}
		}
		pending = pending[:0]
	}
	for {
		current := parser.read()
		if current.kind == tokenEOF {
			return
		}
		if current.kind == tokenKeyword {
			switch current.text {
			case "beginbfchar":
				mode, pending = "bfchar", pending[:0]
			case "beginbfrange":
				mode, pending = "bfrange", pending[:0]
			case "endbfchar", "endbfrange":
				flush()
				mode = ""
			case "begincodespacerange":
				mode, pending = "codespace", pending[:0]
			case "endcodespacerange":
				for index := 0; index+1 < len(pending); index += 2 {
					low, ok := pending[index].(String)
					if !ok {
						continue
					}
					value, length := codeFromBytes(low)
					high := value
					if item, ok := pending[index+1].(String); ok {
						high, _ = codeFromBytes(item)
					}
					f.codespaces = append(f.codespaces, codespace{low: value, high: high, length: length})
				}
				mode, pending = "", pending[:0]
			}
			continue
		}
		if mode == "" {
			continue
		}
		parser.unread(current)
		value, err := parser.object(0)
		if err != nil {
			return
		}
		pending = append(pending, value)
		if len(pending) > 3000 {
			flush()
		}
	}
}

func decodeUTF16BE(value Object) string {
	data, ok := value.(String)
	if !ok || len(data) == 0 {
		return ""
	}
	if len(data)%2 == 1 {
		data = append(data, 0)
	}
	units := make([]uint16, 0, len(data)/2)
	for index := 0; index+1 < len(data); index += 2 {
		units = append(units, uint16(data[index])<<8|uint16(data[index+1]))
	}
	return strings.TrimRight(string(utf16.Decode(units)), "\x00")
}

// decode splits a PDF string into character codes using the font's codespace.
func (f *fontInfo) decode(data []byte) []uint32 {
	out := make([]uint32, 0, len(data))
	if len(f.codespaces) > 0 {
		for index := 0; index < len(data); {
			matched := false
			for length := 1; length <= 4 && index+length <= len(data); length++ {
				value, _ := codeFromBytes(data[index : index+length])
				for _, space := range f.codespaces {
					if space.length == length && value >= space.low && value <= space.high {
						out = append(out, value)
						index += length
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
			if !matched {
				if f.twoByte && index+1 < len(data) {
					value, _ := codeFromBytes(data[index : index+2])
					out = append(out, value)
					index += 2
				} else {
					out = append(out, uint32(data[index]))
					index++
				}
			}
		}
		return out
	}
	if f.twoByte {
		for index := 0; index+1 < len(data); index += 2 {
			out = append(out, uint32(data[index])<<8|uint32(data[index+1]))
		}
		if len(data)%2 == 1 {
			out = append(out, uint32(data[len(data)-1]))
		}
		return out
	}
	for _, item := range data {
		out = append(out, uint32(item))
	}
	return out
}

// text maps a character code to its Unicode string; ok is false when the font
// carries no usable mapping (typical of subset fonts without /ToUnicode).
func (f *fontInfo) text(code uint32) (string, bool) {
	if value, ok := f.toUnicode[code]; ok {
		return value, true
	}
	if f.twoByte {
		if f.ucs2CMap && code > 0 {
			return string(rune(code)), true
		}
		if f.identityCID {
			return "", false
		}
		if code >= 32 && code < 0xd800 {
			return string(rune(code)), true
		}
		return "", false
	}
	if value, ok := f.encoding[byte(code)]; ok && value != "" {
		return value, true
	}
	if code >= 32 && code < 127 {
		return string(rune(code)), true
	}
	return "", false
}

func (f *fontInfo) width(code uint32) float64 {
	if value, ok := f.widths[code]; ok {
		return value
	}
	return f.defaultWidth
}

func (f *fontInfo) isSpace(code uint32) bool {
	if f.hasSpaceCode && code == f.spaceCode {
		return true
	}
	value, ok := f.text(code)
	return ok && value == " "
}

func glyphToUnicode(glyph string) string {
	if glyph == "" {
		return ""
	}
	if value, ok := glyphNames[glyph]; ok {
		return value
	}
	if strings.HasPrefix(glyph, "uni") && len(glyph) >= 7 {
		if value, err := strconv.ParseUint(glyph[3:7], 16, 32); err == nil {
			return string(rune(value))
		}
	}
	if strings.HasPrefix(glyph, "u") && len(glyph) >= 5 && len(glyph) <= 7 {
		if value, err := strconv.ParseUint(glyph[1:], 16, 32); err == nil {
			return string(rune(value))
		}
	}
	// Subset fonts frequently use gNN / cidNN names that carry no meaning.
	if index := strings.IndexAny(glyph, "."); index > 0 {
		return glyphToUnicode(glyph[:index])
	}
	return ""
}

// isMonospaceName recognises the fixed-pitch families that mark code in
// exported documents.
func isMonospaceName(lowerName string) bool {
	for _, candidate := range []string{"mono", "courier", "consolas", "menlo", "d2coding", "inconsolata", "hack", "typewriter", "cousine", "sourcecodepro"} {
		if strings.Contains(lowerName, candidate) {
			return true
		}
	}
	return false
}
