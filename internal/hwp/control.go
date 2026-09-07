package hwp

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/image/bmp"

	"github.com/hkjang/muni/internal/hangul"
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
// put it. Most of what it stands for is a block of its own; a note is inline,
// and belongs in the sentence that referred to it.
//
// The ids are what real files carry. Every picture in every file read so far
// arrived as "gso " — a general shape object — and so did every text box and
// every group of either. "$pic" has not been seen in one.
func (imp *importer) control(node *recordNode) (blocks, inline []*richdoc.Node) {
	switch controlID(node.data) {
	case "tbl ":
		table, captions := imp.table(node)
		if table != nil {
			return append([]*richdoc.Node{table}, captions...), nil
		}
		return captions, nil
	case "gso ", "$pic":
		return imp.shapeObject(node), nil
	case "fn  ", "en  ":
		// A footnote and an endnote are one kind of note to muni, and
		// muni's PDF already prints its notes at the end.
		if note := imp.note(node); note != nil {
			return nil, []*richdoc.Node{note}
		}
	case "head", "foot":
		imp.furniture(controlID(node.data), node)
	case "secd":
		imp.pageDef(node)
	}
	return nil, nil
}

// pageDef reads the paper from a section definition: the first section's
// says which way the whole document is turned.
func (imp *importer) pageDef(node *recordNode) {
	page := node.find(tagPageDef)
	if page == nil || len(page.data) < 8 || imp.pageSeen {
		return
	}
	imp.pageSeen = true
	width := binary.LittleEndian.Uint32(page.data[0:])
	height := binary.LittleEndian.Uint32(page.data[4:])
	imp.landscape = width > height
}

// fieldLink reads the address a hyperlink field points at. After the id,
// the property word and one flag byte comes the command as a length-
// prefixed string, in the form both Hangul formats use.
func fieldLink(node *recordNode) string {
	if controlID(node.data) != "%hlk" {
		return ""
	}
	command, _ := readWideString(node.data, 9)
	return hangul.LinkAddress(command)
}

// furniture keeps the words of a header or footer. muni holds one line of
// each for the whole document, so the first of each kind is the one kept
// and the paragraphs in it are joined by a space.
func (imp *importer) furniture(kind string, node *recordNode) {
	lines := []string{}
	for _, list := range topRecords(node, tagListHeader, tagListHeader) {
		for _, block := range imp.paragraphs(list.children) {
			if text := strings.TrimSpace(block.PlainText()); text != "" {
				lines = append(lines, text)
			}
		}
	}
	text := strings.Join(lines, " ")
	if text == "" {
		return
	}
	switch kind {
	case "head":
		if imp.headerText == "" {
			imp.headerText = text
		}
	case "foot":
		if imp.footerText == "" {
			imp.footerText = text
		}
	}
}

// shapeObject reads a drawing: the pictures in it, and the words in any text
// box it holds. A group holds several of either.
//
// A text box's paragraphs sit under a LIST_HEADER, and a table inside the box
// has LIST_HEADERs of its own beneath that — so only the top-most lists are
// taken here, and paragraphs() reads whatever is inside them the way it reads
// anything else.
func (imp *importer) shapeObject(node *recordNode) []*richdoc.Node {
	out := []*richdoc.Node{}
	sizes := drawnSizes(node)
	for _, picture := range topRecords(node, tagShapePicture, tagListHeader) {
		if image := imp.pictureAt(picture); image != nil {
			if size, ok := sizes[picture]; ok {
				if width, height := hangul.PictureSize(size[0], size[1]); width > 0 {
					image.SetAttr("width", width)
					image.SetAttr("height", height)
				}
			}
			out = append(out, image)
		}
	}
	for _, list := range topRecords(node, tagListHeader, tagListHeader) {
		out = append(out, imp.paragraphs(list.children)...)
	}
	return out
}

// note reads a footnote or endnote control: its paragraphs, joined by a space,
// because muni's note holds text and nothing else.
func (imp *importer) note(node *recordNode) *richdoc.Node {
	lines := []string{}
	for _, list := range topRecords(node, tagListHeader, tagListHeader) {
		for _, block := range imp.paragraphs(list.children) {
			if text := strings.TrimSpace(block.PlainText()); text != "" {
				lines = append(lines, text)
			}
		}
	}
	if len(lines) == 0 {
		return nil
	}
	return &richdoc.Node{Type: richdoc.FootnoteType, Content: []*richdoc.Node{richdoc.Text(strings.Join(lines, " "))}}
}

// The size a picture is drawn at is not written on the picture; it is written
// on the SHAPE_COMPONENT above it, which is where every drawing — a picture,
// a text box, a group of either — says how big it is. Two sizes are there:
// the size the picture came in at, and after it the size it is drawn at, the
// one someone changed by dragging a corner.
//
// Where those numbers begin depends on where the component sits. A component
// directly beneath the control header writes the control id twice, once for
// the control and once for itself; one inside a group writes it once.
const (
	shapeComponentTopOffset   = 8
	shapeComponentGroupOffset = 4
	// After the id come the offsets within the group, the group level with
	// the version, and the size the picture came in at.
	shapeComponentDrawnSize = 4 + 4 + 4 + 4 + 4
)

// drawnSizes is how large each picture in a drawing is drawn, in HWPUNIT,
// looked up by the picture's own record.
func drawnSizes(node *recordNode) map[*recordNode][2]int {
	out := map[*recordNode][2]int{}
	var walk func(current *recordNode, offset int)
	walk = func(current *recordNode, offset int) {
		for _, child := range current.children {
			if child.tag == tagListHeader {
				// A text box's paragraphs, and any drawing of their own
				// inside them, are read as the words they are.
				continue
			}
			if child.tag != tagShapeComponent {
				walk(child, offset)
				continue
			}
			if at := offset + shapeComponentDrawnSize; len(child.data) >= at+8 {
				width := int(binary.LittleEndian.Uint32(child.data[at:]))
				height := int(binary.LittleEndian.Uint32(child.data[at+4:]))
				for _, picture := range child.children {
					if picture.tag == tagShapePicture {
						out[picture] = [2]int{width, height}
					}
				}
			}
			walk(child, shapeComponentGroupOffset)
		}
	}
	walk(node, shapeComponentTopOffset)
	return out
}

// topRecords finds records with a tag beneath a node, without descending into
// a record with the stop tag — what is inside one of those belongs to it.
func topRecords(node *recordNode, tag, stop uint16) []*recordNode {
	out := []*recordNode{}
	var walk func(*recordNode)
	walk = func(current *recordNode) {
		for _, child := range current.children {
			if child.tag == tag {
				out = append(out, child)
			}
			if child.tag == stop {
				continue
			}
			walk(child)
		}
	}
	walk(node)
	return out
}

// table reads a table control into muni's table, and its caption into the
// paragraphs that follow it.
//
// Each cell is a LIST_HEADER carrying where it sits and how far it reaches,
// with its paragraphs beneath it. The rows are rebuilt from where the cells
// say they are rather than from the order they were written in. A caption is
// a LIST_HEADER too, one with no cell address — and one that was dropped for
// having none, until a real file's "[표 캡션]" went missing.
func (imp *importer) table(node *recordNode) (table *richdoc.Node, captions []*richdoc.Node) {
	cells := node.all(tagListHeader)
	if len(cells) == 0 {
		return nil, nil
	}
	type placed struct {
		row, column   uint16
		rowSpan, span uint16
		node          *richdoc.Node
	}
	placedCells := make([]placed, 0, len(cells))
	// How wide each column is, learnt from the cells that cover one column
	// apiece: a merged cell gives the total across the columns it covers.
	columnPixels := map[int]int{}
	rowCount := 0
	for _, cell := range cells {
		address, ok := readCellAddress(cell.data)
		if !ok {
			captions = append(captions, imp.paragraphs(cell.children)...)
			continue
		}
		if address.span == 1 && columnPixels[int(address.column)] == 0 {
			if pixels := hangul.PixelWidth(int(address.width)); pixels > 0 {
				columnPixels[int(address.column)] = pixels
			}
		}
		content := imp.paragraphs(cell.children)
		if len(content) == 0 {
			content = []*richdoc.Node{richdoc.Paragraph()}
		}
		built := &richdoc.Node{Type: "tableCell", Content: content}
		built.SetAttr("colspan", int(address.span))
		built.SetAttr("rowspan", int(address.rowSpan))
		built.SetAttr("verticalAlign", address.verticalAlign)
		if shade := imp.borderFillShade(address.borderFill); shade != "" {
			built.SetAttr("backgroundColor", shade)
		}
		placedCells = append(placedCells, placed{
			row: address.row, column: address.column,
			rowSpan: address.rowSpan, span: address.span, node: built,
		})
		if int(address.row)+1 > rowCount {
			rowCount = int(address.row) + 1
		}
	}
	if len(placedCells) == 0 {
		return nil, captions
	}
	// Second, once every column has been seen: a cell in the first row can
	// cover a column only a later row measures.
	for _, cell := range placedCells {
		if widths := hangul.ColumnWidths(columnPixels, int(cell.column), int(cell.span)); len(widths) > 0 {
			cell.node.SetAttr("colwidth", widths)
		}
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
		return nil, captions
	}
	return &richdoc.Node{Type: "table", Content: rows}, captions
}

// cellAddress is where a cell sits, how far it reaches, how wide it is, where
// in it the words sit and what paints it.
type cellAddress struct {
	column, row   uint16
	span, rowSpan uint16
	width         uint32
	// verticalAlign is where the words sit in the cell, named the way muni
	// marks it.
	verticalAlign string
	// borderFill is which of DocInfo's BORDER_FILL records paints the cell,
	// counting from one; zero is a cell that names none.
	borderFill uint16
}

// readCellAddress reads the part of a cell's LIST_HEADER that says where it is.
//
// The common part is a paragraph count and a property word; the cell's own
// address follows it, and after the address its size in HWPUNIT, its height,
// its four margins and the number of the border and fill that paint it. A
// file that stops early still places its cells — each tail is read only when
// it is there.
func readCellAddress(raw []byte) (cellAddress, bool) {
	const common = 4 + 4
	if len(raw) < common+8 {
		return cellAddress{}, false
	}
	// The property word every paragraph list opens with says in bits 5 and 6
	// where in the cell its words sit.
	property := binary.LittleEndian.Uint32(raw[4:])
	address := cellAddress{
		column:        binary.LittleEndian.Uint16(raw[common:]),
		row:           binary.LittleEndian.Uint16(raw[common+2:]),
		span:          binary.LittleEndian.Uint16(raw[common+4:]),
		rowSpan:       binary.LittleEndian.Uint16(raw[common+6:]),
		verticalAlign: hangul.CellVerticalAlign((property >> 5) & 0x03),
	}
	if len(raw) >= common+12 {
		address.width = binary.LittleEndian.Uint32(raw[common+8:])
	}
	// Past the width come the height and the four margins, and then the
	// number of the BORDER_FILL: 4 + 4 + 4×2.
	if len(raw) >= common+26 {
		address.borderFill = binary.LittleEndian.Uint16(raw[common+24:])
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

// borderFillShade is the background of the BORDER_FILL a cell names. The
// records are counted from one in the order DocInfo wrote them, so a cell
// naming none says zero.
func (imp *importer) borderFillShade(id uint16) string {
	if id < 1 || int(id) > len(imp.borderFills) {
		return ""
	}
	return imp.borderFills[id-1]
}

// pictureFrom reads a picture control into an image node, keeping its bytes.
//
// The stream that holds them is named for the id the document's own BinData
// record gave it — BIN0001 and so on — and the extension is whatever the
// picture was.
func (imp *importer) pictureAt(picture *recordNode) *richdoc.Node {
	id := imp.pictureStreamID(picture)
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
	mediaType := http.DetectContentType(data)
	if mediaType == "image/bmp" {
		// Old .hwp files keep their pictures as BMP, which no browser shows
		// and the import therefore drops. Every picture in a 2010s report
		// file was BMP. Converted here, so it arrives as something the
		// editor can draw rather than being found and then thrown away.
		if converted, ok := bmpToPNG(data); ok {
			data, mediaType = converted, "image/png"
			name = strings.TrimSuffix(name, filepath.Ext(name)) + ".png"
		}
	}
	placeholder := richdoc.Placeholder(len(imp.assets) + 1)
	imp.assets = append(imp.assets, richdoc.Asset{
		Placeholder: placeholder,
		Name:        name,
		MediaType:   mediaType,
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

func (imp *importer) pictureStreamID(picture *recordNode) string {
	if picture == nil || picture.tag != tagShapePicture {
		return ""
	}
	// Two layouts have been seen in real files. In 5.0.3.x the id leads the
	// picture information; in 5.0.2.x it follows the brightness, contrast and
	// effect bytes, three bytes later at an odd offset — which a scan that
	// steps two bytes at a time can never land on.
	for _, offset := range []int{pictureBinIDOffset, pictureBinIDOffset + 3} {
		if len(picture.data) < offset+2 {
			continue
		}
		if number := binary.LittleEndian.Uint16(picture.data[offset:]); number != 0 {
			if name, ok := imp.binaryAt(number); ok {
				return name
			}
		}
	}
	// Neither: search the tail of the record byte by byte. The format has
	// undocumented gaps, and alignment is not something it promises.
	for offset := pictureBinIDOffset - 8; offset+2 <= len(picture.data); offset++ {
		if offset < 0 {
			continue
		}
		number := binary.LittleEndian.Uint16(picture.data[offset:])
		if number == 0 || number > 4096 {
			continue
		}
		if name, ok := imp.binaryAt(number); ok {
			return name
		}
	}
	return ""
}

// binaryAt is the stream the number a picture wrote stands for.
//
// The number is not the stream's own. It counts DocInfo's BIN_DATA records,
// and each of those says which stream it means — a report whose three
// pictures were written in one order and listed in another gave every picture
// its neighbour's image, because the number was read as the stream's. The
// giveaway was that each picture was drawn at exactly the shape of one of the
// others.
//
// A file whose records say nothing about the number — one where they were not
// read at all — falls back to reading it as the stream's, which is what every
// file whose two orders agree has always done.
func (imp *importer) binaryAt(number uint16) (string, bool) {
	if number >= 1 && int(number) <= len(imp.binaries) {
		if named := imp.binaries[number-1]; named != "" && imp.hasBinary(named) {
			return named, true
		}
	}
	name := binaryName(number)
	return name, imp.hasBinary(name)
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
//
// A real file can hold two streams for one id — BIN0003.bmp beside
// BIN0003.jpg — and which one the directory lists first is chance. The one a
// browser can show is preferred: the import keeps only pictures the editor can
// draw, and taking the BMP first threw away a picture that was there in JPEG.
func (imp *importer) binaryName(id string) (string, bool) {
	best, found := "", false
	for _, name := range imp.file.names("BinData") {
		if !strings.HasPrefix(strings.ToUpper(name), strings.ToUpper(id)) {
			continue
		}
		if !found || extensionRank(name) < extensionRank(best) {
			best, found = name, true
		}
	}
	return best, found
}

// extensionRank orders a picture's formats by how readily a browser shows
// them; lower is better.
func extensionRank(name string) int {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(name), ".")) {
	case "png":
		return 0
	case "jpg", "jpeg":
		return 1
	case "gif", "webp":
		return 2
	case "bmp":
		return 5
	}
	return 9
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

// bmpToPNG re-encodes a BMP as PNG. A picture that cannot be decoded is left
// as it was, which the import then drops — the same as before, and better than
// storing bytes that claim to be a PNG and are not.
func bmpToPNG(data []byte) ([]byte, bool) {
	picture, err := bmp.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, false
	}
	var out bytes.Buffer
	if err := png.Encode(&out, picture); err != nil {
		return nil, false
	}
	return out.Bytes(), true
}
