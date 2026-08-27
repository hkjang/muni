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

// paraTextRecord wraps UTF-16 code units in a PARA_TEXT record.
func paraTextRecord(units []uint16) []byte {
	payload := make([]byte, len(units)*2)
	for index, unit := range units {
		binary.LittleEndian.PutUint16(payload[index*2:], unit)
	}
	return append(recordHeader(tagParaText, 1, len(payload)), payload...)
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
