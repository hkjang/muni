package hwp

import (
	"encoding/binary"
	"net/http"
	"sort"
	"strings"

	"github.com/hkjang/muni/internal/richdoc"
)

// A CTRL_HEADER says what kind of thing sits at a control mark in the
// paragraph's text, and everything the thing is made of hangs beneath it: a
// table's cells, a picture's reference to the bytes.
//
// The id is four characters stored back to front. The errata is worth heeding
// here — a picture's id is "$pic", not the "gso " the specification gives.
func controlID(raw []byte) string {
	if len(raw) < 4 {
		return ""
	}
	return string([]byte{raw[3], raw[2], raw[1], raw[0]})
}

// control reads whatever a control mark stands for, when muni has somewhere to
// put it.
func (imp *importer) control(node *recordNode) []*richdoc.Node {
	switch controlID(node.data) {
	case "tbl ":
		if table := imp.table(node); table != nil {
			return []*richdoc.Node{table}
		}
	case "$pic", "gso ":
		if picture := imp.pictureFrom(node); picture != nil {
			return []*richdoc.Node{picture}
		}
	}
	return nil
}

// table reads a table control into muni's table.
//
// Each cell is a LIST_HEADER carrying where it sits and how far it reaches,
// with its paragraphs beneath it. The rows are rebuilt from where the cells
// say they are rather than from the order they were written in.
func (imp *importer) table(node *recordNode) *richdoc.Node {
	cells := node.all(tagListHeader)
	if len(cells) == 0 {
		return nil
	}
	type placed struct {
		row, column   uint16
		rowSpan, span uint16
		node          *richdoc.Node
	}
	placedCells := make([]placed, 0, len(cells))
	rowCount := 0
	for _, cell := range cells {
		address, ok := readCellAddress(cell.data)
		if !ok {
			continue
		}
		content := imp.paragraphs(cell.children)
		if len(content) == 0 {
			content = []*richdoc.Node{richdoc.Paragraph()}
		}
		built := &richdoc.Node{Type: "tableCell", Content: content}
		built.SetAttr("colspan", int(address.span))
		built.SetAttr("rowspan", int(address.rowSpan))
		built.SetAttr("verticalAlign", "top")
		placedCells = append(placedCells, placed{
			row: address.row, column: address.column,
			rowSpan: address.rowSpan, span: address.span, node: built,
		})
		if int(address.row)+1 > rowCount {
			rowCount = int(address.row) + 1
		}
	}
	if len(placedCells) == 0 {
		return nil
	}
	// Indexed by where each cell says it is, rather than scanned for. The
	// scan was rows × 1024 columns × cells, which a five-thousand-cell table
	// turns into billions of comparisons and a crafted one into far more.
	byPlace := map[[2]uint16][]*richdoc.Node{}
	columns := map[uint16][]uint16{}
	for _, cell := range placedCells {
		key := [2]uint16{cell.row, cell.column}
		if _, seen := byPlace[key]; !seen {
			columns[cell.row] = append(columns[cell.row], cell.column)
		}
		byPlace[key] = append(byPlace[key], cell.node)
	}
	rows := make([]*richdoc.Node, 0, rowCount)
	for rowIndex := 0; rowIndex < rowCount; rowIndex++ {
		at := columns[uint16(rowIndex)]
		if len(at) == 0 {
			continue
		}
		sort.Slice(at, func(a, b int) bool { return at[a] < at[b] })
		row := &richdoc.Node{Type: "tableRow"}
		for _, column := range at {
			row.Content = append(row.Content, byPlace[[2]uint16{uint16(rowIndex), column}]...)
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil
	}
	return &richdoc.Node{Type: "table", Content: rows}
}

// cellAddress is where a cell sits and how far it reaches.
type cellAddress struct {
	column, row   uint16
	span, rowSpan uint16
}

// readCellAddress reads the part of a cell's LIST_HEADER that says where it is.
//
// The common part is a paragraph count and a property word; the cell's own
// address follows it.
func readCellAddress(raw []byte) (cellAddress, bool) {
	const common = 4 + 4
	if len(raw) < common+8 {
		return cellAddress{}, false
	}
	address := cellAddress{
		column:  binary.LittleEndian.Uint16(raw[common:]),
		row:     binary.LittleEndian.Uint16(raw[common+2:]),
		span:    binary.LittleEndian.Uint16(raw[common+4:]),
		rowSpan: binary.LittleEndian.Uint16(raw[common+6:]),
	}
	if address.span == 0 {
		address.span = 1
	}
	if address.rowSpan == 0 {
		address.rowSpan = 1
	}
	// A file that means something else by these bytes would give absurd
	// numbers; a table is not a thousand columns wide.
	if address.column > 1000 || address.row > 10000 || address.span > 1000 || address.rowSpan > 10000 {
		return cellAddress{}, false
	}
	return address, true
}

// pictureFrom reads a picture control into an image node, keeping its bytes.
//
// The stream that holds them is named for the id the document's own BinData
// record gave it — BIN0001 and so on — and the extension is whatever the
// picture was.
func (imp *importer) pictureFrom(node *recordNode) *richdoc.Node {
	id := imp.pictureStreamID(node)
	if id == "" {
		return nil
	}
	data, name, ok := imp.binaryStream(id)
	if !ok {
		return nil
	}
	if placeholder, seen := imp.assetByID[id]; seen {
		image := &richdoc.Node{Type: "image"}
		image.SetAttr("src", placeholder)
		return image
	}
	placeholder := richdoc.Placeholder(len(imp.assets) + 1)
	imp.assets = append(imp.assets, richdoc.Asset{
		Placeholder: placeholder,
		Name:        name,
		MediaType:   http.DetectContentType(data),
		Data:        data,
	})
	imp.assetByID[id] = placeholder
	image := &richdoc.Node{Type: "image"}
	image.SetAttr("src", placeholder)
	return image
}

// pictureStreamID finds the BinData id a picture refers to.
//
// It is written in the picture's own record, after the border, the four
// corners, the crop and the margins. Hunting for it instead — taking any
// two-byte value that happens to name a stream this file has — finds a count
// or a flag first, so in a document with several pictures they all resolve to
// whichever one had the lowest number.
const pictureBinIDOffset = 4 + 4 + 4 + 32 + 16 + 8

func (imp *importer) pictureStreamID(node *recordNode) string {
	picture := findRecord(node, tagShapePicture)
	if picture == nil {
		return ""
	}
	if len(picture.data) >= pictureBinIDOffset+2 {
		id := binary.LittleEndian.Uint16(picture.data[pictureBinIDOffset:])
		if id != 0 {
			if name := binaryName(id); imp.hasBinary(name) {
				return name
			}
		}
	}
	// The offset moved between versions of the format. Searching the
	// picture's own record is a guess, but a far narrower one than searching
	// everything beneath the control.
	for offset := 0; offset+2 <= len(picture.data); offset += 2 {
		id := binary.LittleEndian.Uint16(picture.data[offset:])
		if id == 0 {
			continue
		}
		if name := binaryName(id); imp.hasBinary(name) {
			return name
		}
	}
	return ""
}

// findRecord looks for a tag anywhere beneath a node.
func findRecord(node *recordNode, tag uint16) *recordNode {
	for _, child := range node.children {
		if child.tag == tag {
			return child
		}
		if found := findRecord(child, tag); found != nil {
			return found
		}
	}
	return nil
}

// binaryName is the stream name a BinData id gives: BIN followed by the id in
// four hexadecimal digits.
func binaryName(id uint16) string {
	const digits = "0123456789ABCDEF"
	return "BIN" + string([]byte{
		digits[(id>>12)&0xF], digits[(id>>8)&0xF], digits[(id>>4)&0xF], digits[id&0xF],
	})
}

// hasBinary reports whether a stream of that name exists, without reading it.
// Asking by reading meant inflating a picture in full to answer a question
// about its name, once per candidate.
func (imp *importer) hasBinary(id string) bool {
	_, ok := imp.binaryName(id)
	return ok
}

// binaryName finds the stream whose name starts with an id. The stream carries
// the picture's own extension, so the match is on the start.
func (imp *importer) binaryName(id string) (string, bool) {
	for _, name := range imp.file.names("BinData") {
		if strings.HasPrefix(strings.ToUpper(name), strings.ToUpper(id)) {
			return name, true
		}
	}
	return "", false
}

// binaryStream reads a picture's bytes, once.
func (imp *importer) binaryStream(id string) ([]byte, string, bool) {
	name, ok := imp.binaryName(id)
	if !ok {
		return nil, "", false
	}
	if cached, seen := imp.binaryCache[name]; seen {
		return cached, name, len(cached) > 0
	}
	raw, ok := imp.stream("BinData/" + name)
	if !ok {
		raw = nil
	}
	imp.binaryCache[name] = raw
	return raw, name, len(raw) > 0
}
