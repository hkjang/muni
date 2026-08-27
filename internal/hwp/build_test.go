package hwp

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

// muni does not write .hwp, so every test here builds one. That means building
// an OLE2 compound file too — there is no other way to find out what the
// reader makes of real bytes, and a reader tested only against its own output
// can only find disagreements it has with itself.

const testSectorSize = 512

type streamSpec struct {
	// path is "FileHeader" or "BodyText/Section0".
	path string
	data []byte
}

// buildCompound lays out a compound file: one FAT sector, one directory
// sector, then the streams.
func buildCompound(t *testing.T, streams []streamSpec) []byte {
	t.Helper()

	type placed struct {
		name    string
		storage string
		start   uint32
		size    uint64
	}
	// The directory is as many sectors as its entries need. Sizing it at one
	// silently drops everything past the fourth entry, which is a document
	// with one section however many it was written with.
	storageNames := map[string]bool{}
	for _, spec := range streams {
		if cut := bytes.IndexByte([]byte(spec.path), '/'); cut >= 0 {
			storageNames[spec.path[:cut]] = true
		}
	}
	entryCount := 1 + len(storageNames) + len(streams)
	directorySectors := (entryCount*128 + testSectorSize - 1) / testSectorSize

	var placedStreams []placed
	var payload []byte
	nextSector := uint32(1 + directorySectors)
	fat := []uint32{0xFFFFFFFD} // the FAT's own sector
	for index := 0; index < directorySectors; index++ {
		if index == directorySectors-1 {
			fat = append(fat, 0xFFFFFFFE)
		} else {
			fat = append(fat, uint32(index)+2)
		}
	}

	for _, spec := range streams {
		storage, name := "", spec.path
		if cut := bytes.IndexByte([]byte(spec.path), '/'); cut >= 0 {
			storage, name = spec.path[:cut], spec.path[cut+1:]
		}
		sectors := (len(spec.data) + testSectorSize - 1) / testSectorSize
		if sectors == 0 {
			sectors = 1
		}
		block := make([]byte, sectors*testSectorSize)
		copy(block, spec.data)
		payload = append(payload, block...)
		for index := 0; index < sectors; index++ {
			if index == sectors-1 {
				fat = append(fat, 0xFFFFFFFE)
			} else {
				fat = append(fat, nextSector+uint32(index)+1)
			}
		}
		placedStreams = append(placedStreams, placed{
			name: name, storage: storage, start: nextSector, size: uint64(len(spec.data)),
		})
		nextSector += uint32(sectors)
	}

	// The directory: root, then the storages, then the streams. Siblings are
	// chained through `right`, which the reader walks in full.
	entries := []dirEntry{{name: "Root Entry", kind: 5}}
	indexOf := map[string]uint32{}
	for _, stream := range placedStreams {
		if stream.storage == "" || indexOf[stream.storage] != 0 {
			continue
		}
		indexOf[stream.storage] = uint32(len(entries))
		entries = append(entries, dirEntry{name: stream.storage, kind: 1})
	}
	topLevel := []uint32{}
	for _, index := range indexOf {
		topLevel = append(topLevel, index)
	}
	childOf := map[string][]uint32{}
	for _, stream := range placedStreams {
		index := uint32(len(entries))
		entries = append(entries, dirEntry{
			name: stream.name, kind: 2, start: stream.start, size: stream.size,
		})
		if stream.storage == "" {
			topLevel = append(topLevel, index)
			continue
		}
		childOf[stream.storage] = append(childOf[stream.storage], index)
	}
	chain := func(list []uint32) uint32 {
		if len(list) == 0 {
			return 0xFFFFFFFF
		}
		for position := 0; position < len(list)-1; position++ {
			entries[list[position]].right = list[position+1]
		}
		entries[list[len(list)-1]].right = 0xFFFFFFFF
		return list[0]
	}
	for storage, children := range childOf {
		entries[indexOf[storage]].child = chain(children)
	}
	entries[0].child = chain(topLevel)
	for index := range entries {
		if entries[index].left == 0 {
			entries[index].left = 0xFFFFFFFF
		}
		if entries[index].child == 0 {
			entries[index].child = 0xFFFFFFFF
		}
	}

	directory := make([]byte, 0, len(entries)*128)
	for _, entry := range entries {
		raw := make([]byte, 128)
		units := utf16.Encode([]rune(entry.name))
		for position, unit := range units {
			if position*2+1 >= 64 {
				break
			}
			binary.LittleEndian.PutUint16(raw[position*2:], unit)
		}
		binary.LittleEndian.PutUint16(raw[0x40:], uint16(len(units)*2+2))
		raw[0x42] = entry.kind
		binary.LittleEndian.PutUint32(raw[0x44:], entry.left)
		binary.LittleEndian.PutUint32(raw[0x48:], entry.right)
		binary.LittleEndian.PutUint32(raw[0x4C:], entry.child)
		binary.LittleEndian.PutUint32(raw[0x74:], entry.start)
		binary.LittleEndian.PutUint64(raw[0x78:], entry.size)
		directory = append(directory, raw...)
	}
	directoryBlock := make([]byte, directorySectors*testSectorSize)
	copy(directoryBlock, directory)

	fatSector := make([]byte, testSectorSize)
	for index, value := range fat {
		if index*4+4 > len(fatSector) {
			break
		}
		binary.LittleEndian.PutUint32(fatSector[index*4:], value)
	}
	for index := len(fat); index*4+4 <= len(fatSector); index++ {
		binary.LittleEndian.PutUint32(fatSector[index*4:], 0xFFFFFFFF)
	}

	header := make([]byte, testSectorSize)
	copy(header, cfbSignature)
	binary.LittleEndian.PutUint16(header[0x1E:], 9)
	binary.LittleEndian.PutUint16(header[0x20:], 6)
	binary.LittleEndian.PutUint32(header[0x2C:], 1)
	binary.LittleEndian.PutUint32(header[0x30:], 1)
	binary.LittleEndian.PutUint32(header[0x38:], 0) // nothing goes in the mini stream
	binary.LittleEndian.PutUint32(header[0x3C:], 0xFFFFFFFE)
	binary.LittleEndian.PutUint32(header[0x44:], 0xFFFFFFFE)
	for offset := 0x4C; offset+4 <= testSectorSize; offset += 4 {
		binary.LittleEndian.PutUint32(header[offset:], 0xFFFFFFFF)
	}
	binary.LittleEndian.PutUint32(header[0x4C:], 0) // the FAT is in sector 0

	out := append([]byte{}, header...)
	out = append(out, fatSector...)
	out = append(out, directoryBlock...)
	return append(out, payload...)
}

// hwpFileHeader builds the 256 bytes that say what the file is.
func hwpFileHeader(compressed, encrypted bool) []byte {
	raw := make([]byte, 256)
	copy(raw, "HWP Document File")
	binary.LittleEndian.PutUint32(raw[32:], 0x05000300)
	flags := uint32(0)
	if compressed {
		flags |= 0x01
	}
	if encrypted {
		flags |= 0x02
	}
	binary.LittleEndian.PutUint32(raw[36:], flags)
	return raw
}

func recordHeader(tag uint16, level uint16, size int) []byte {
	raw := make([]byte, 4)
	if size >= sizeInHeader {
		binary.LittleEndian.PutUint32(raw, uint32(tag)|uint32(level)<<10|uint32(sizeInHeader)<<20)
		extra := make([]byte, 4)
		binary.LittleEndian.PutUint32(extra, uint32(size))
		return append(raw, extra...)
	}
	binary.LittleEndian.PutUint32(raw, uint32(tag)|uint32(level)<<10|uint32(size)<<20)
	return raw
}

// deflateRaw compresses the way a .hwp stream is compressed: raw deflate, with
// no zlib wrapper.
func deflateRaw(t *testing.T, raw []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	writer, err := flate.NewWriter(&out, flate.DefaultCompression)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func units(text string) []uint16 { return utf16.Encode([]rune(text)) }

// paragraphRecords writes a paragraph the way a real file writes one: a
// PARA_HEADER with its text and its character shapes beneath it.
//
// The reader groups records by the depth each carries, so a fixture that emits
// a bare PARA_TEXT is not a file Hangul would ever write and proves nothing
// about reading one.
func paragraphRecords(text []uint16, shapes ...charRun) []byte {
	header := make([]byte, 22)
	binary.LittleEndian.PutUint32(header[0:], uint32(len(text)))
	binary.LittleEndian.PutUint16(header[10:], uint16(len(shapes)))
	out := append(recordHeader(tagParaHeader, 0, len(header)), header...)

	payload := make([]byte, len(text)*2)
	for index, unit := range text {
		binary.LittleEndian.PutUint16(payload[index*2:], unit)
	}
	out = append(out, recordHeader(tagParaText, 1, len(payload))...)
	out = append(out, payload...)

	if len(shapes) > 0 {
		shapeData := make([]byte, len(shapes)*8)
		for index, run := range shapes {
			binary.LittleEndian.PutUint32(shapeData[index*8:], run.at)
			binary.LittleEndian.PutUint32(shapeData[index*8+4:], run.shape)
		}
		out = append(out, recordHeader(tagParaCharShape, 1, len(shapeData))...)
		out = append(out, shapeData...)
	}
	return out
}

// charShapeRecord writes one CHAR_SHAPE into DocInfo, with the switches muni
// reads set as asked.
func charShapeRecord(bold, italic, underline bool) []byte {
	// Laid out from the format, not from the reader's own constant: a fixture
	// that shares the reader's arithmetic cannot catch the reader getting it
	// wrong, which is exactly what happened here.
	const faceNames, ratios, spacings, relativeSizes, offsets, baseSize = 7 * 2, 7, 7, 7, 7, 4
	propertyOffset := faceNames + ratios + spacings + relativeSizes + offsets + baseSize
	data := make([]byte, propertyOffset+8)
	bits := uint32(0)
	if italic {
		bits |= 0x01
	}
	if bold {
		bits |= 0x02
	}
	if underline {
		bits |= 0x04
	}
	binary.LittleEndian.PutUint32(data[propertyOffset:], bits)
	return append(recordHeader(tagCharShape, 0, len(data)), data...)
}

// tableRecords writes a table the way a file writes one: a control header
// naming it, a TABLE record, and one LIST_HEADER per cell with the cell's
// paragraphs beneath it.
func tableRecords(level uint16, cells []tableCellSpec) []byte {
	control := []byte{' ', 'l', 'b', 't'} // "tbl " stored back to front
	control = append(control, make([]byte, 8)...)
	out := append(recordHeader(tagCtrlHeader, level, len(control)), control...)

	table := make([]byte, 8)
	binary.LittleEndian.PutUint16(table[4:], uint16(len(cells)))
	out = append(out, recordHeader(tagTable, level+1, len(table))...)
	out = append(out, table...)

	for _, cell := range cells {
		header := make([]byte, 8+8+16)
		binary.LittleEndian.PutUint32(header[0:], 1) // one paragraph
		binary.LittleEndian.PutUint16(header[8:], cell.column)
		binary.LittleEndian.PutUint16(header[10:], cell.row)
		binary.LittleEndian.PutUint16(header[12:], cell.span)
		binary.LittleEndian.PutUint16(header[14:], cell.rowSpan)
		out = append(out, recordHeader(tagListHeader, level+1, len(header))...)
		out = append(out, header...)
		out = append(out, shiftLevels(paragraphRecords(units(cell.text)), level+2)...)
	}
	return out
}

type tableCellSpec struct {
	row, column   uint16
	span, rowSpan uint16
	text          string
}

// shiftLevels rewrites a run of records to sit at a deeper level, which is how
// a paragraph inside a cell is written.
func shiftLevels(raw []byte, base uint16) []byte {
	out := []byte{}
	for offset := 0; offset+4 <= len(raw); {
		header := binary.LittleEndian.Uint32(raw[offset:])
		offset += 4
		tag := uint16(header & 0x3FF)
		level := uint16((header>>10)&0x3FF) + base
		size := int((header >> 20) & 0xFFF)
		if size == sizeInHeader {
			size = int(binary.LittleEndian.Uint32(raw[offset:]))
			offset += 4
		}
		if offset+size > len(raw) {
			break
		}
		out = append(out, recordHeader(tag, level, size)...)
		out = append(out, raw[offset:offset+size]...)
		offset += size
	}
	return out
}

// paragraphWithControl writes a paragraph whose text holds a control mark,
// with the control's own records beneath it.
func paragraphWithControl(text string, control []byte) []byte {
	code := units(text)
	code = append(code, 11)
	for filler := 0; filler < 6; filler++ {
		code = append(code, 11)
	}
	code = append(code, 11)
	return append(paragraphRecords(code), control...)
}

// paraShapeRecord writes one PARA_SHAPE: the alignment in bits 2-4 of the
// first word, then the margins and the first-line indent as HWPUNIT.
func paraShapeRecord(alignCode uint32, leftUnits, firstLineUnits int32) []byte {
	data := make([]byte, 54)
	binary.LittleEndian.PutUint32(data[0:], alignCode<<2)
	binary.LittleEndian.PutUint32(data[4:], uint32(leftUnits))
	binary.LittleEndian.PutUint32(data[12:], uint32(firstLineUnits))
	return append(recordHeader(tagParaShape, 0, len(data)), data...)
}

// styleRecord writes one STYLE: the Korean name, then the English one, each a
// length in code units followed by the characters.
func styleRecord(korean, english string) []byte {
	data := []byte{}
	for _, name := range []string{korean, english} {
		encoded := units(name)
		length := make([]byte, 2)
		binary.LittleEndian.PutUint16(length, uint16(len(encoded)))
		data = append(data, length...)
		for _, unit := range encoded {
			pair := make([]byte, 2)
			binary.LittleEndian.PutUint16(pair, unit)
			data = append(data, pair...)
		}
	}
	data = append(data, make([]byte, 12)...)
	return append(recordHeader(tagStyle, 0, len(data)), data...)
}

// styledParagraph writes a paragraph naming a shape and a style by number.
func styledParagraph(text []uint16, shapeID uint16, styleID uint8) []byte {
	header := make([]byte, 22)
	binary.LittleEndian.PutUint32(header[0:], uint32(len(text)))
	binary.LittleEndian.PutUint16(header[8:], shapeID)
	header[10] = styleID
	out := append(recordHeader(tagParaHeader, 0, len(header)), header...)
	payload := make([]byte, len(text)*2)
	for index, unit := range text {
		binary.LittleEndian.PutUint16(payload[index*2:], unit)
	}
	out = append(out, recordHeader(tagParaText, 1, len(payload))...)
	return append(out, payload...)
}
