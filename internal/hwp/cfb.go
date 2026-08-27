package hwp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"unicode/utf16"
)

// A .hwp is an OLE2 compound file: a little filesystem inside one file, with a
// sector table, a directory tree and streams whose sectors are chained through
// it. Go has no reader for that, so this is one — only as much of it as
// reading a document needs, which is the directory and the streams it names.

const (
	sectorFree       = 0xFFFFFFFF
	sectorEndOfChain = 0xFFFFFFFE
	maxSectors       = 1 << 22 // a bound on a malformed chain, not on real files
)

var cfbSignature = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}

type compound struct {
	data       []byte
	sectorSize int
	miniSize   int
	fat        []uint32
	miniFAT    []uint32
	miniStream []byte
	cutoff     uint32
	entries    []dirEntry
}

type dirEntry struct {
	name  string
	kind  byte // 1 storage, 2 stream, 5 root
	start uint32
	size  uint64
	child uint32
	left  uint32
	right uint32
}

func openCompound(data []byte) (*compound, error) {
	if len(data) < 512 || string(data[:8]) != string(cfbSignature) {
		return nil, errors.New("HWP 파일이 아닙니다")
	}
	sectorShift := binary.LittleEndian.Uint16(data[0x1E:])
	miniShift := binary.LittleEndian.Uint16(data[0x20:])
	if sectorShift < 7 || sectorShift > 20 || miniShift < 2 || miniShift > sectorShift {
		return nil, errors.New("HWP 파일의 섹터 크기를 읽지 못했습니다")
	}
	file := &compound{
		data:       data,
		sectorSize: 1 << sectorShift,
		miniSize:   1 << miniShift,
		cutoff:     binary.LittleEndian.Uint32(data[0x38:]),
	}
	if err := file.readFAT(); err != nil {
		return nil, err
	}
	if err := file.readDirectory(binary.LittleEndian.Uint32(data[0x30:])); err != nil {
		return nil, err
	}
	file.readMiniFAT()
	return file, nil
}

// sector returns one sector's bytes, or nil when the file stops short of it.
func (c *compound) sector(index uint32) []byte {
	start := int64(c.sectorSize) + int64(index)*int64(c.sectorSize)
	end := start + int64(c.sectorSize)
	if start < 0 || end > int64(len(c.data)) {
		return nil
	}
	return c.data[start:end]
}

// readFAT walks the DIFAT — the table of tables — and reads the sector chain
// table it points at.
func (c *compound) readFAT() error {
	count := binary.LittleEndian.Uint32(c.data[0x2C:])
	if count > maxSectors {
		return errors.New("HWP 파일의 섹터 표가 너무 큽니다")
	}
	locations := make([]uint32, 0, count)
	for offset := 0x4C; offset+4 <= 512 && len(locations) < int(count); offset += 4 {
		if value := binary.LittleEndian.Uint32(c.data[offset:]); value < sectorEndOfChain {
			locations = append(locations, value)
		}
	}
	// Beyond 109 entries the DIFAT continues in sectors of its own.
	next := binary.LittleEndian.Uint32(c.data[0x44:])
	for guard := 0; next < sectorEndOfChain && len(locations) < int(count) && guard < maxSectors; guard++ {
		block := c.sector(next)
		if block == nil {
			break
		}
		for offset := 0; offset+4 <= len(block)-4 && len(locations) < int(count); offset += 4 {
			if value := binary.LittleEndian.Uint32(block[offset:]); value < sectorEndOfChain {
				locations = append(locations, value)
			}
		}
		next = binary.LittleEndian.Uint32(block[len(block)-4:])
	}
	for _, location := range locations {
		block := c.sector(location)
		if block == nil {
			continue
		}
		for offset := 0; offset+4 <= len(block); offset += 4 {
			c.fat = append(c.fat, binary.LittleEndian.Uint32(block[offset:]))
		}
	}
	if len(c.fat) == 0 {
		return errors.New("HWP 파일의 섹터 표가 비어 있습니다")
	}
	return nil
}

// chain reads a stream by following its sectors through a table.
func (c *compound) chain(table []uint32, start uint32, size uint64, read func(uint32) []byte) []byte {
	out := make([]byte, 0, size)
	index := start
	for guard := 0; index < uint32(len(table)) && guard < maxSectors; guard++ {
		block := read(index)
		if block == nil {
			break
		}
		out = append(out, block...)
		if uint64(len(out)) >= size {
			break
		}
		index = table[index]
		if index >= sectorEndOfChain {
			break
		}
	}
	if uint64(len(out)) > size {
		out = out[:size]
	}
	return out
}

func (c *compound) readDirectory(start uint32) error {
	raw := c.chain(c.fat, start, uint64(len(c.data)), c.sector)
	for offset := 0; offset+128 <= len(raw); offset += 128 {
		entry := raw[offset : offset+128]
		nameLen := int(binary.LittleEndian.Uint16(entry[0x40:]))
		if nameLen > 64 {
			nameLen = 64
		}
		units := make([]uint16, 0, nameLen/2)
		for index := 0; index+1 < nameLen; index += 2 {
			unit := binary.LittleEndian.Uint16(entry[index:])
			if unit == 0 {
				break
			}
			units = append(units, unit)
		}
		c.entries = append(c.entries, dirEntry{
			name:  string(utf16.Decode(units)),
			kind:  entry[0x42],
			left:  binary.LittleEndian.Uint32(entry[0x44:]),
			right: binary.LittleEndian.Uint32(entry[0x48:]),
			child: binary.LittleEndian.Uint32(entry[0x4C:]),
			start: binary.LittleEndian.Uint32(entry[0x74:]),
			size:  binary.LittleEndian.Uint64(entry[0x78:]),
		})
	}
	if len(c.entries) == 0 {
		return errors.New("HWP 파일의 디렉터리를 읽지 못했습니다")
	}
	return nil
}

// readMiniFAT prepares the small-stream area. Streams under the cutoff — most
// of a document's pictures are over it, most of its records under — live
// packed inside the root entry's own stream rather than in sectors.
func (c *compound) readMiniFAT() {
	first := binary.LittleEndian.Uint32(c.data[0x3C:])
	if first < sectorEndOfChain {
		raw := c.chain(c.fat, first, uint64(len(c.data)), c.sector)
		for offset := 0; offset+4 <= len(raw); offset += 4 {
			c.miniFAT = append(c.miniFAT, binary.LittleEndian.Uint32(raw[offset:]))
		}
	}
	root := c.entries[0]
	if root.kind == 5 && root.size > 0 {
		c.miniStream = c.chain(c.fat, root.start, root.size, c.sector)
	}
}

func (c *compound) miniSector(index uint32) []byte {
	start := int(index) * c.miniSize
	if start < 0 || start+c.miniSize > len(c.miniStream) {
		return nil
	}
	return c.miniStream[start : start+c.miniSize]
}

// stream returns the bytes of a named stream. A name inside a storage is
// written "BodyText/Section0", the way a path is.
func (c *compound) stream(path string) ([]byte, bool) {
	entry, ok := c.find(path)
	if !ok || entry.kind != 2 {
		return nil, false
	}
	if entry.size < uint64(c.cutoff) && len(c.miniStream) > 0 {
		return c.chain(c.miniFAT, entry.start, entry.size, c.miniSector), true
	}
	return c.chain(c.fat, entry.start, entry.size, c.sector), true
}

// find walks the directory's red-black tree by name. The tree's order is not
// the one a reader would guess, so every sibling is visited rather than
// compared.
func (c *compound) find(path string) (dirEntry, bool) {
	parts := strings.Split(path, "/")
	current := c.entries[0].child
	var found dirEntry
	for depth, part := range parts {
		var match *dirEntry
		c.walk(current, func(entry dirEntry) {
			if match == nil && strings.EqualFold(entry.name, part) {
				copied := entry
				match = &copied
			}
		})
		if match == nil {
			return dirEntry{}, false
		}
		found = *match
		if depth < len(parts)-1 {
			current = match.child
		}
	}
	return found, true
}

// walk visits an entry and its siblings, bounded against a cycle.
func (c *compound) walk(index uint32, visit func(dirEntry)) {
	seen := map[uint32]bool{}
	var step func(uint32)
	step = func(at uint32) {
		if at == sectorFree || int(at) >= len(c.entries) || seen[at] {
			return
		}
		seen[at] = true
		entry := c.entries[at]
		visit(entry)
		step(entry.left)
		step(entry.right)
	}
	step(index)
}

// names lists every stream under a storage, which is how the sections and the
// pictures are found without guessing how many there are.
func (c *compound) names(storage string) []string {
	entry, ok := c.find(storage)
	if !ok {
		return nil
	}
	var out []string
	c.walk(entry.child, func(child dirEntry) {
		if child.kind == 2 {
			out = append(out, child.name)
		}
	})
	return out
}

func (c *compound) describe() string {
	return fmt.Sprintf("sectors=%d entries=%d", c.sectorSize, len(c.entries))
}
