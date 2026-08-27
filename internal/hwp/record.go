package hwp

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"io"
)

// Inside the compound file, a .hwp stream is a run of records. Each begins
// with one uint32 holding three things at once: what the record is, how deep
// it sits, and how long it is.
//
//	bits 0-9   tag
//	bits 10-19 level
//	bits 20-31 size
//
// A size of 0xFFF means "too big to fit here", and the real size is the next
// uint32. Reading that as part of the payload is the classic way to lose the
// rest of a stream.
type record struct {
	tag   uint16
	level uint16
	data  []byte
}

const sizeInHeader = 0xFFF

// tags used by the body. The numbers are the format's own.
const (
	tagDocumentProperties = 0x010 // HWPTAG_BEGIN
	tagCharShape          = 0x015
	tagParaShape          = 0x019
	tagStyle              = 0x01A
	tagBinData            = 0x012
	tagParaHeader         = 0x042
	tagParaText           = 0x043
	tagParaCharShape      = 0x044
	tagCtrlHeader         = 0x047
	tagListHeader         = 0x048
	tagTable              = 0x04C
)

func readRecords(raw []byte) []record {
	out := []record{}
	for offset := 0; offset+4 <= len(raw); {
		header := binary.LittleEndian.Uint32(raw[offset:])
		offset += 4
		tag := uint16(header & 0x3FF)
		level := uint16((header >> 10) & 0x3FF)
		size := int((header >> 20) & 0xFFF)
		if size == sizeInHeader {
			if offset+4 > len(raw) {
				break
			}
			size = int(binary.LittleEndian.Uint32(raw[offset:]))
			offset += 4
		}
		if size < 0 || offset+size > len(raw) {
			// A truncated file still has everything before the truncation.
			break
		}
		out = append(out, record{tag: tag, level: level, data: raw[offset : offset+size]})
		offset += size
	}
	return out
}

// inflate undoes the compression a .hwp applies to its streams.
//
// The bytes are raw deflate with no zlib wrapper, which is why a reader that
// reaches for zlib gets "invalid header" on a perfectly good file.
func inflate(raw []byte) ([]byte, error) {
	reader := flate.NewReader(bytes.NewReader(raw))
	defer reader.Close()
	// A document that decompresses to more than this is not a document.
	return io.ReadAll(io.LimitReader(reader, 256<<20))
}
