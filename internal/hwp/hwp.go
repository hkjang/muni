// Package hwp reads .hwp files — the binary format Hangul Office wrote before
// .hwpx, and the one most Korean documents older than a few years are in.
//
// Where .hwpx is a zip of XML, this is an OLE2 compound file holding streams
// of compressed binary records. Three layers have to come apart before there
// is any text: the compound file (cfb.go), the compression (record.go), and
// the records themselves.
package hwp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/hkjang/muni/internal/richdoc"
)

// Meta is what the file says about itself rather than about its text.
type Meta struct {
	Version string
}

type fileHeader struct {
	version    string
	compressed bool
	encrypted  bool
}

type charShape struct {
	bold      bool
	italic    bool
	underline bool
	strike    bool
}

type importer struct {
	file       *compound
	header     fileHeader
	charShapes []charShape
	assets     []richdoc.Asset
	// assetByID keeps a picture used twice from being stored twice.
	assetByID map[string]string
}

// Parse reads a .hwp into muni's document model.
func Parse(body []byte) (*richdoc.Node, []richdoc.Asset, Meta, error) {
	file, err := openCompound(body)
	if err != nil {
		return nil, nil, Meta{}, err
	}
	imp := &importer{file: file, assetByID: map[string]string{}}
	if err := imp.readFileHeader(); err != nil {
		return nil, nil, Meta{}, err
	}
	if imp.header.encrypted {
		// Saying so beats importing a document of mojibake.
		return nil, nil, Meta{}, errors.New("암호가 걸린 HWP 문서입니다. 한글에서 암호를 풀고 다시 올려 주세요")
	}
	imp.readDocInfo()

	document := richdoc.Doc()
	for _, name := range imp.sectionNames() {
		raw, ok := imp.stream("BodyText/" + name)
		if !ok {
			continue
		}
		document.Content = append(document.Content, imp.section(raw)...)
	}
	if len(document.Content) == 0 {
		return nil, nil, Meta{}, errors.New("HWP 본문을 읽지 못했습니다")
	}
	richdoc.LiftImages(document)
	return document, imp.assets, Meta{Version: imp.header.version}, nil
}

// readFileHeader reads the 256 bytes that say what the rest of the file is.
func (imp *importer) readFileHeader() error {
	raw, ok := imp.file.stream("FileHeader")
	if !ok || len(raw) < 40 {
		return errors.New("HWP 파일 머리를 읽지 못했습니다")
	}
	if !strings.HasPrefix(string(raw[:32]), "HWP Document File") {
		return errors.New("HWP 문서가 아닙니다")
	}
	version := binary.LittleEndian.Uint32(raw[32:])
	flags := binary.LittleEndian.Uint32(raw[36:])
	imp.header = fileHeader{
		version: fmt.Sprintf("%d.%d.%d.%d",
			(version>>24)&0xFF, (version>>16)&0xFF, (version>>8)&0xFF, version&0xFF),
		compressed: flags&0x01 != 0,
		encrypted:  flags&0x02 != 0,
	}
	return nil
}

// stream reads a stream, undoing the compression the header said was applied.
func (imp *importer) stream(path string) ([]byte, bool) {
	raw, ok := imp.file.stream(path)
	if !ok {
		return nil, false
	}
	if !imp.header.compressed || strings.EqualFold(path, "FileHeader") {
		return raw, true
	}
	inflated, err := inflate(raw)
	if err != nil && len(inflated) == 0 {
		return nil, false
	}
	return inflated, true
}

// sectionNames lists the body streams in the order they are read. They are
// named Section0, Section1 … and the number carries the order.
func (imp *importer) sectionNames() []string {
	names := imp.file.names("BodyText")
	sort.Slice(names, func(a, b int) bool {
		return sectionNumber(names[a]) < sectionNumber(names[b])
	})
	return names
}

func sectionNumber(name string) int {
	digits := strings.TrimLeftFunc(name, func(r rune) bool { return r < '0' || r > '9' })
	value := 0
	for _, r := range digits {
		if r < '0' || r > '9' {
			break
		}
		value = value*10 + int(r-'0')
	}
	return value
}

// readDocInfo reads the shapes the body refers to by number.
func (imp *importer) readDocInfo() {
	raw, ok := imp.stream("DocInfo")
	if !ok {
		return
	}
	for _, item := range readRecords(raw) {
		switch item.tag {
		case tagCharShape:
			imp.charShapes = append(imp.charShapes, readCharShape(item.data))
		case tagBinData:
			// The picture streams are found by name; nothing to keep here yet.
		}
	}
}

// readCharShape reads the italic/bold/underline/strike switches out of one
// CHAR_SHAPE record.
//
// The record begins with seven font ids, then seven of each of four more
// arrays — the per-language tables — before the size and the property bits.
func readCharShape(raw []byte) charShape {
	const propertyOffset = 7*2 + 7 + 7 + 7*2 + 7 + 4 // fonts, ratios, spacings, sizes, positions, size
	if len(raw) < propertyOffset+4 {
		return charShape{}
	}
	bits := binary.LittleEndian.Uint32(raw[propertyOffset:])
	return charShape{
		italic:    bits&0x01 != 0,
		bold:      bits&0x02 != 0,
		underline: (bits>>2)&0x03 != 0,
		strike:    (bits>>18)&0x07 != 0,
	}
}

// section reads one BodyText stream into blocks.
func (imp *importer) section(raw []byte) []*richdoc.Node {
	return imp.paragraphs(tree(readRecords(raw)))
}
