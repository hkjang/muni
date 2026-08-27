package hwp

import (
	"encoding/binary"
	"net/http"
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
	rows := make([]*richdoc.Node, rowCount)
	for index := range rows {
		rows[index] = &richdoc.Node{Type: "tableRow"}
	}
	// Sort within a row by column, so a file that wrote its cells out of order
	// still reads left to right.
	for rowIndex := 0; rowIndex < rowCount; rowIndex++ {
		for column := 0; column < 1<<10; column++ {
			placedAny := false
			for _, cell := range placedCells {
				if int(cell.row) == rowIndex && int(cell.column) == column {
					rows[rowIndex].Content = append(rows[rowIndex].Content, cell.node)
					placedAny = true
				}
			}
			if !placedAny && column > len(placedCells) {
				break
			}
		}
	}
	kept := rows[:0]
	for _, row := range rows {
		if len(row.Content) > 0 {
			kept = append(kept, row)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return &richdoc.Node{Type: "table", Content: kept}
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
// It sits in the shape's own record rather than in the control header, so the
// records beneath the control are searched for the first one that names a
// stream this file actually has.
func (imp *importer) pictureStreamID(node *recordNode) string {
	found := ""
	var walk func(*recordNode)
	walk = func(current *recordNode) {
		if found != "" {
			return
		}
		for offset := 0; offset+2 <= len(current.data); offset += 2 {
			id := binary.LittleEndian.Uint16(current.data[offset:])
			if id == 0 {
				continue
			}
			name := binaryName(id)
			if _, _, ok := imp.binaryStream(name); ok {
				found = name
				return
			}
		}
		for _, child := range current.children {
			walk(child)
		}
	}
	for _, child := range node.children {
		walk(child)
	}
	return found
}

// binaryName is the stream name a BinData id gives: BIN followed by the id in
// four hexadecimal digits.
func binaryName(id uint16) string {
	const digits = "0123456789ABCDEF"
	return "BIN" + string([]byte{
		digits[(id>>12)&0xF], digits[(id>>8)&0xF], digits[(id>>4)&0xF], digits[id&0xF],
	})
}

// binaryStream finds a picture's bytes. The stream carries the picture's own
// extension, so the name is matched by its start.
func (imp *importer) binaryStream(id string) ([]byte, string, bool) {
	for _, name := range imp.file.names("BinData") {
		if !strings.HasPrefix(strings.ToUpper(name), strings.ToUpper(id)) {
			continue
		}
		raw, ok := imp.stream("BinData/" + name)
		if !ok || len(raw) == 0 {
			continue
		}
		return raw, name, true
	}
	return nil, "", false
}
