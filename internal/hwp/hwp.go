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

	"github.com/hkjang/muni/internal/hangul"
	"github.com/hkjang/muni/internal/richdoc"
)

// Meta is what the file says about itself rather than about its text.
type Meta struct {
	Version string
	// Header and Footer are the words of the first header and footer, the
	// one line of each muni keeps.
	Header string
	Footer string
	// Landscape is set when the first section's paper is wider than it is
	// tall.
	Landscape bool
}

type fileHeader struct {
	version    string
	compressed bool
	encrypted  bool
}

// charShapeBaseSizeOffset is where a CHAR_SHAPE's base size sits, and
// charShapePropertyOffset where its switches begin: UINT16[7] face names,
// UINT8[7] ratios, INT8[7] spacings, UINT8[7] relative sizes, INT8[7]
// offsets, INT32 base size.
//
// Counting the relative sizes as two bytes each puts this seven bytes too far
// and reads the switches out of the base size, which is formatting arrived at
// by accident.
const (
	charShapeBaseSizeOffset = 7*2 + 7 + 7 + 7 + 7
	charShapePropertyOffset = charShapeBaseSizeOffset + 4
	// After the switches come two INT8 shadow offsets, and only then the
	// text colour — the underline, shade and shadow colours follow it.
	charShapeColorOffset = charShapePropertyOffset + 4 + 2
)

type charShape struct {
	bold      bool
	italic    bool
	underline bool
	strike    bool
	color     string
	sizePoint string
	// fontID is the face's number in the FACE_NAME list; family is the name
	// it resolves to, once DocInfo has been read through.
	fontID uint16
	family string
}

type importer struct {
	file       *compound
	header     fileHeader
	charShapes []charShape
	paraShapes []paraShape
	styles     []styleInfo
	// faceNames is the document's font table in the order the FACE_NAME
	// records came, which is the order a CHAR_SHAPE's face numbers index.
	faceNames []string
	assets    []richdoc.Asset
	// assetByID keeps a picture used twice from being stored twice, and
	// binaryCache keeps it from being decompressed twice.
	assetByID   map[string]string
	binaryCache map[string][]byte
	// headerText and footerText are the first header and footer met.
	headerText string
	footerText string
	// landscape is the first section's paper, once seen.
	landscape bool
	pageSeen  bool
}

// Parse reads a .hwp into muni's document model.
func Parse(body []byte) (*richdoc.Node, []richdoc.Asset, Meta, error) {
	file, err := openCompound(body)
	if err != nil {
		return nil, nil, Meta{}, err
	}
	imp := &importer{file: file, assetByID: map[string]string{}, binaryCache: map[string][]byte{}}
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
		// A document with no words is still a document. Real files have one
		// paragraph holding only the section and column definitions — that
		// is what a blank page saved from Hangul looks like — and refusing
		// it says the file is broken when it is merely empty.
		document.Content = []*richdoc.Node{richdoc.Paragraph()}
	}
	richdoc.LiftImages(document)
	return document, imp.assets, Meta{Version: imp.header.version, Header: imp.headerText, Footer: imp.footerText, Landscape: imp.landscape}, nil
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
		case tagFaceName:
			imp.faceNames = append(imp.faceNames, readFaceName(item.data))
		case tagCharShape:
			imp.charShapes = append(imp.charShapes, readCharShape(item.data))
		case tagParaShape:
			imp.paraShapes = append(imp.paraShapes, readParaShape(item.data))
		case tagStyle:
			imp.styles = append(imp.styles, readStyle(item.data))
		case tagBinData:
			// The picture streams are found by name; nothing to keep here yet.
		}
	}
	// A CHAR_SHAPE names its face by number and nothing else, so a number
	// taken for a name would put a font called "3" on every run. The names
	// are resolved once the whole of DocInfo has been read rather than inside
	// the loop, which would depend on FACE_NAME coming first.
	for index := range imp.charShapes {
		imp.charShapes[index].family = imp.faceName(imp.charShapes[index].fontID)
	}
}

// readFaceName reads the font's name out of one FACE_NAME record.
//
// The record opens with a property byte saying which of the optional
// tails — a substitute font, panose type information, a default font —
// follow the name. muni wants only the name, which comes first.
func readFaceName(raw []byte) string {
	if len(raw) < 1 {
		return ""
	}
	name, _ := readWideString(raw, 1)
	return name
}

// faceName is the face a CHAR_SHAPE's Hangul font number points at.
//
// The FACE_NAME records are written one language after another, Hangul
// first, so a Hangul font number indexes them from the start — which is the
// face a Korean document is actually set in.
func (imp *importer) faceName(id uint16) string {
	if int(id) >= len(imp.faceNames) {
		return ""
	}
	return imp.faceNames[id]
}

// readCharShape reads one CHAR_SHAPE record — the switches, the size, the
// colour and which face the Hangul is set in.
//
// The record opens with five per-language tables of seven entries each — the
// font ids two bytes wide, the other four one byte — and then the base size,
// before the word the switches live in. The first entry of every table is the
// Hangul one; the six after it are English, Hanja, Japanese, other, symbol
// and user.
func readCharShape(raw []byte) charShape {
	const propertyOffset = charShapePropertyOffset
	if len(raw) < propertyOffset+4 {
		return charShape{}
	}
	bits := binary.LittleEndian.Uint32(raw[propertyOffset:])
	shape := charShape{
		italic:    bits&0x01 != 0,
		bold:      bits&0x02 != 0,
		underline: (bits>>2)&0x03 != 0,
		strike:    (bits>>18)&0x07 != 0,
		fontID:    binary.LittleEndian.Uint16(raw[0:]),
	}
	// The base size is in hundredths of a point, the same unit the .hwpx
	// writes as an attribute.
	shape.sizePoint = hangul.FontSize(int(int32(binary.LittleEndian.Uint32(raw[charShapeBaseSizeOffset:]))))
	if len(raw) >= charShapeColorOffset+4 {
		shape.color = colorHex(binary.LittleEndian.Uint32(raw[charShapeColorOffset:]))
	}
	return shape
}

// colorHex turns a COLORREF into the colour muni holds. The format writes
// 0x00BBGGRR — blue first, which read as if it were RGB turns every red word
// blue. Black is what a document is written in unless it says otherwise and
// is worth nothing to record.
func colorHex(value uint32) string {
	red, green, blue := value&0xFF, (value>>8)&0xFF, (value>>16)&0xFF
	if red == 0 && green == 0 && blue == 0 {
		return ""
	}
	return fmt.Sprintf("#%02X%02X%02X", red, green, blue)
}

// section reads one BodyText stream into blocks.
func (imp *importer) section(raw []byte) []*richdoc.Node {
	return imp.paragraphs(tree(readRecords(raw)))
}
