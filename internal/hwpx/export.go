package hwpx

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strconv"
	"strings"
	"time"

	"github.com/hkjang/muni/internal/hangul"
	"github.com/hkjang/muni/internal/richdoc"
)

// A .hwpx is written the way it is read: the text in Contents/section0.xml,
// the formatting it refers to by number in Contents/header.xml, the pictures
// in BinData. Every run names a charPr id and every paragraph a paraPr id, so
// the builder collects the distinct shapes a document uses and numbers them,
// rather than writing formatting where it is applied — which is not a thing
// HWPX has anywhere to put.
//
// Having a writer as well as a reader is also the only way muni gets a round
// trip for this format, and a round trip is the test the reader lacked.

// Image is a picture referenced by the document that should be embedded.
type Image struct {
	Data      []byte
	MediaType string
}

// Options describe the document as a whole.
type Options struct {
	Title        string
	Landscape    bool
	Created      time.Time
	ResolveImage func(src string) (Image, bool)
}

// Build renders a document into a .hwpx package.
func Build(doc *richdoc.Node, opts Options) ([]byte, error) {
	if doc == nil {
		doc = richdoc.Doc()
	}
	b := &builder{
		opts:     opts,
		charPrs:  map[string]int{},
		paraPrs:  map[string]int{},
		pictures: map[string]string{},
	}
	// The one shape every run starts from, and the one every paragraph does.
	b.charPrID(charKey{})
	b.paraPrID(paraKey{})
	b.blocks(doc.Content, blockContext{})
	return b.pack()
}

type builder struct {
	opts Options
	body strings.Builder

	charPrs   map[string]int
	charOrder []charKey
	paraPrs   map[string]int
	paraOrder []paraKey

	binData  []binItem
	pictures map[string]string // source → binary item id
}

type binItem struct {
	id        string
	extension string
	mediaType string
	data      []byte
}

// charKey is one distinct run shape.
type charKey struct {
	bold, italic, underline, strike, mono bool
	color, size, family                   string
}

// paraKey is one distinct paragraph shape.
type paraKey struct {
	align     string
	indent    int // HWPUNIT
	firstLine bool
	lineRate  string
}

type blockContext struct {
	indent   int
	heading  int
	quote    bool
	mono     bool
	bold     bool
	listMark string
}

func (b *builder) charPrID(key charKey) int {
	name := fmt.Sprintf("%v", key)
	if id, ok := b.charPrs[name]; ok {
		return id
	}
	id := len(b.charOrder)
	b.charPrs[name] = id
	b.charOrder = append(b.charOrder, key)
	return id
}

func (b *builder) paraPrID(key paraKey) int {
	name := fmt.Sprintf("%v", key)
	if id, ok := b.paraPrs[name]; ok {
		return id
	}
	id := len(b.paraOrder)
	b.paraPrs[name] = id
	b.paraOrder = append(b.paraOrder, key)
	return id
}

// indentUnits is one of muni's indent steps in HWPUNIT: a quarter inch, the
// same width the reader turns back into a step.
const indentUnits = hangul.UnitsPerInch / 4

func (b *builder) blocks(nodes []*richdoc.Node, ctx blockContext) {
	for _, node := range nodes {
		b.block(node, ctx)
	}
}

func (b *builder) block(node *richdoc.Node, ctx blockContext) {
	if node == nil {
		return
	}
	switch node.Type {
	case "paragraph":
		b.paragraph(node, ctx, node.Content, "")
	case "heading":
		level := node.AttrInt("level", 1)
		if level < 1 {
			level = 1
		}
		if level > 6 {
			level = 6
		}
		child := ctx
		child.heading = level
		child.bold = true
		b.paragraph(node, child, node.Content, "")
	case "blockquote":
		child := ctx
		child.quote = true
		child.indent++
		if len(node.Content) == 0 {
			b.paragraph(nil, child, nil, "")
			return
		}
		b.blocks(node.Content, child)
	case "codeBlock":
		child := ctx
		child.mono = true
		child.indent++
		text := node.PlainText()
		for _, line := range strings.Split(text, "\n") {
			b.paragraph(nil, child, []*richdoc.Node{richdoc.Text(line)}, "")
		}
	case "bulletList":
		b.list(node, ctx, func(int) string { return "• " })
	case "orderedList":
		start := node.AttrInt("start", 1)
		b.list(node, ctx, func(index int) string { return strconv.Itoa(start+index) + ". " })
	case "taskList":
		b.taskList(node, ctx)
	case "horizontalRule":
		key := paraKey{align: "center"}
		b.body.WriteString(`<hp:p paraPrIDRef="` + strconv.Itoa(b.paraPrID(key)) + `" styleIDRef="0">` +
			`<hp:run charPrIDRef="0"><hp:t>` + strings.Repeat("─", 30) + `</hp:t></hp:run></hp:p>`)
	case "pageBreak":
		b.body.WriteString(`<hp:p paraPrIDRef="0" styleIDRef="0" pageBreak="1"><hp:run charPrIDRef="0"><hp:t/></hp:run></hp:p>`)
	case "table":
		b.table(node, ctx)
	case "image":
		if picture := b.picture(node); picture != "" {
			key := paraKey{align: hangul.Alignment(node.AttrString("textAlign"))}
			b.body.WriteString(`<hp:p paraPrIDRef="` + strconv.Itoa(b.paraPrID(key)) + `" styleIDRef="0">` +
				`<hp:run charPrIDRef="0">` + picture + `</hp:run></hp:p>`)
		}
	case "doc":
		b.blocks(node.Content, ctx)
	case "text":
		b.paragraph(nil, ctx, []*richdoc.Node{node}, "")
	default:
		// Unknown block: keep its children rather than dropping content.
		if len(node.Content) > 0 {
			b.blocks(node.Content, ctx)
		}
	}
}

func (b *builder) list(node *richdoc.Node, ctx blockContext, mark func(int) string) {
	index := 0
	for _, item := range node.Content {
		if item == nil || item.Type != "listItem" {
			continue
		}
		child := ctx
		child.indent++
		child.listMark = mark(index)
		index++
		b.listItem(item, child)
	}
}

func (b *builder) taskList(node *richdoc.Node, ctx blockContext) {
	for _, item := range node.Content {
		if item == nil || item.Type != "taskItem" {
			continue
		}
		child := ctx
		child.indent++
		child.listMark = "☐ "
		if checked, ok := item.Attr("checked").(bool); ok && checked {
			child.listMark = "☑ "
		}
		b.listItem(item, child)
	}
}

// listItem writes an item's first paragraph with the marker in front of it
// and the rest beneath. HWPX has real numbering, but it lives in the header
// as a numbering definition every list refers to; a marker in the text reads
// the same on every screen and survives a round trip through muni's reader,
// which is where the lists came from.
func (b *builder) listItem(item *richdoc.Node, ctx blockContext) {
	first := true
	for _, child := range item.Content {
		if child == nil {
			continue
		}
		if first && child.Type == "paragraph" {
			b.paragraph(child, ctx, child.Content, ctx.listMark)
			first = false
			continue
		}
		plain := ctx
		plain.listMark = ""
		b.block(child, plain)
		first = false
	}
	if first {
		b.paragraph(nil, ctx, nil, ctx.listMark)
	}
}

// paragraph writes one <hp:p>, taking its shape from the node and the context.
func (b *builder) paragraph(node *richdoc.Node, ctx blockContext, inline []*richdoc.Node, prefix string) {
	key := paraKey{indent: ctx.indent * indentUnits}
	if node != nil {
		key.align = hangul.Alignment(node.AttrString("textAlign"))
		if steps := node.AttrInt("indent", 0); steps > 0 {
			key.indent += steps * indentUnits
		}
		if first, ok := node.Attr("firstLine").(bool); ok && first {
			key.firstLine = true
		}
		key.lineRate = node.AttrString("lineHeight")
	}
	style := 0
	if ctx.heading > 0 {
		style = ctx.heading
	}
	b.body.WriteString(`<hp:p paraPrIDRef="` + strconv.Itoa(b.paraPrID(key)) +
		`" styleIDRef="` + strconv.Itoa(style) + `">`)
	if prefix != "" {
		b.body.WriteString(`<hp:run charPrIDRef="` + strconv.Itoa(b.charPrID(b.baseKey(ctx))) +
			`"><hp:t>` + escape(prefix) + `</hp:t></hp:run>`)
	}
	b.inline(inline, ctx)
	if len(inline) == 0 && prefix == "" {
		b.body.WriteString(`<hp:run charPrIDRef="0"><hp:t/></hp:run>`)
	}
	b.body.WriteString(`</hp:p>`)
}

func (b *builder) baseKey(ctx blockContext) charKey {
	key := charKey{bold: ctx.bold, mono: ctx.mono}
	switch ctx.heading {
	case 1:
		key.size = "1800"
	case 2:
		key.size = "1500"
	case 3:
		key.size = "1300"
	case 4, 5, 6:
		key.size = "1150"
	}
	return key
}

// inline writes runs, one per distinct shape.
func (b *builder) inline(nodes []*richdoc.Node, ctx blockContext) {
	for _, node := range nodes {
		if node == nil {
			continue
		}
		switch node.Type {
		case "text":
			key := b.baseKey(ctx)
			applyMarks(&key, node.Marks)
			b.body.WriteString(`<hp:run charPrIDRef="` + strconv.Itoa(b.charPrID(key)) + `"><hp:t>` +
				escape(node.Text) + `</hp:t></hp:run>`)
		case "hardBreak":
			b.body.WriteString(`<hp:run charPrIDRef="0"><hp:t><hp:lineBreak/></hp:t></hp:run>`)
		case "image":
			if picture := b.picture(node); picture != "" {
				b.body.WriteString(`<hp:run charPrIDRef="0">` + picture + `</hp:run>`)
			}
		case richdoc.FootnoteType:
			b.footnote(node, ctx)
		default:
			if len(node.Content) > 0 {
				b.inline(node.Content, ctx)
			}
		}
	}
}

func applyMarks(key *charKey, marks []richdoc.Mark) {
	for _, mark := range marks {
		switch mark.Type {
		case "bold", "strong":
			key.bold = true
		case "italic", "em":
			key.italic = true
		case "underline":
			key.underline = true
		case "strike", "strikethrough", "s":
			key.strike = true
		case "code":
			key.mono = true
		case "textStyle":
			if color := mark.AttrString("color"); color != "" {
				key.color = color
			}
			if family := mark.AttrString("fontFamily"); family != "" {
				key.family = family
			}
			if size := mark.AttrString("fontSize"); size != "" {
				if point, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(size), "pt"), 64); err == nil && point > 0 {
					key.size = strconv.Itoa(int(point * 100))
				}
			}
		}
	}
}

// footnote writes a note where it sits in the sentence. HWPX keeps a note's
// paragraphs inside the reference, which is where muni keeps them too.
func (b *builder) footnote(node *richdoc.Node, ctx blockContext) {
	b.body.WriteString(`<hp:run charPrIDRef="0"><hp:footNote><hp:subList>` +
		`<hp:p paraPrIDRef="0" styleIDRef="0">`)
	plain := ctx
	plain.heading = 0
	plain.bold = false
	b.inline(node.Content, plain)
	b.body.WriteString(`</hp:p></hp:subList></hp:footNote></hp:run>`)
}

// escape writes text as XML character data.
func escape(value string) string {
	var out strings.Builder
	for _, r := range value {
		switch r {
		case '&':
			out.WriteString("&amp;")
		case '<':
			out.WriteString("&lt;")
		case '>':
			out.WriteString("&gt;")
		case '"':
			out.WriteString("&quot;")
		case '\t':
			out.WriteString("</hp:t><hp:tab/><hp:t>")
		default:
			if r < 0x20 && r != '\n' {
				continue
			}
			out.WriteRune(r)
		}
	}
	return out.String()
}

// picture embeds a picture and returns the <hp:pic> that shows it.
func (b *builder) picture(node *richdoc.Node) string {
	src := node.AttrString("src")
	if src == "" || b.opts.ResolveImage == nil {
		return ""
	}
	id, seen := b.pictures[src]
	if !seen {
		found, ok := b.opts.ResolveImage(src)
		if !ok || len(found.Data) == 0 {
			return ""
		}
		id = "image" + strconv.Itoa(len(b.binData)+1)
		b.binData = append(b.binData, binItem{
			id: id, extension: extensionFor(found.MediaType, found.Data),
			mediaType: found.MediaType, data: found.Data,
		})
		b.pictures[src] = id
	}
	item := b.binData[b.pictureIndex(id)]
	width, height := pictureSize(item.data, node.AttrInt("width", 0))
	return `<hp:pic id="` + id + `" reverse="0">` +
		`<hp:sz width="` + strconv.Itoa(width) + `" height="` + strconv.Itoa(height) + `" widthRelTo="ABSOLUTE" heightRelTo="ABSOLUTE"/>` +
		`<hp:pos treatAsChar="1" vertRelTo="PARA" horzRelTo="PARA"/>` +
		`<hp:imgRect><hc:pt0 x="0" y="0"/><hc:pt1 x="` + strconv.Itoa(width) + `" y="0"/>` +
		`<hc:pt2 x="` + strconv.Itoa(width) + `" y="` + strconv.Itoa(height) + `"/><hc:pt3 x="0" y="` + strconv.Itoa(height) + `"/></hp:imgRect>` +
		`<hc:img binaryItemIDRef="` + id + `" bright="0" contrast="0" effect="REAL_PIC" alpha="0"/>` +
		`</hp:pic>`
}

func (b *builder) pictureIndex(id string) int {
	for index, item := range b.binData {
		if item.id == id {
			return index
		}
	}
	return 0
}

// pictureSize is a picture's size in HWPUNIT, from its pixels at 96 to the
// inch, no wider than the text column.
func pictureSize(data []byte, requestedPixels int) (int, int) {
	const column = 6 * hangul.UnitsPerInch // an A4 text column, roughly
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	pixelsWide, pixelsHigh := 320, 240
	if err == nil && config.Width > 0 && config.Height > 0 {
		pixelsWide, pixelsHigh = config.Width, config.Height
	}
	if requestedPixels > 0 && pixelsWide > 0 {
		pixelsHigh = pixelsHigh * requestedPixels / pixelsWide
		pixelsWide = requestedPixels
	}
	width := pixelsWide * hangul.UnitsPerInch / 96
	height := pixelsHigh * hangul.UnitsPerInch / 96
	if width > column && width > 0 {
		height = height * column / width
		width = column
	}
	if height <= 0 {
		height = width
	}
	return width, height
}

func extensionFor(mediaType string, data []byte) string {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "image/jpeg", "image/jpg":
		return "jpg"
	case "image/gif":
		return "gif"
	case "image/bmp":
		return "bmp"
	}
	if bytes.HasPrefix(data, []byte{0xFF, 0xD8}) {
		return "jpg"
	}
	return "png"
}

// table writes an <hp:tbl> inside a paragraph of its own, which is where HWPX
// puts a table.
func (b *builder) table(node *richdoc.Node, ctx blockContext) {
	rows := []*richdoc.Node{}
	for _, row := range node.Content {
		if row != nil && row.Type == "tableRow" {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return
	}
	columns := 0
	for _, row := range rows {
		span := 0
		for _, cell := range row.Content {
			if cell != nil {
				span += cellSpan(cell, "colspan")
			}
		}
		if span > columns {
			columns = span
		}
	}
	if columns == 0 {
		return
	}
	// The column width is the text column shared out evenly; the reader
	// records widths in pixels and the .docx path will do what it does.
	const column = 6 * hangul.UnitsPerInch
	cellWidth := column / columns

	var out strings.Builder
	out.WriteString(`<hp:p paraPrIDRef="0" styleIDRef="0"><hp:run charPrIDRef="0">`)
	out.WriteString(`<hp:tbl id="` + strconv.Itoa(len(b.body.String())) + `" rowCnt="` + strconv.Itoa(len(rows)) +
		`" colCnt="` + strconv.Itoa(columns) + `" cellSpacing="0" borderFillIDRef="1" repeatHeader="` +
		strconv.FormatBool(len(rows) > 0 && rows[0].Content != nil && rows[0].Content[0] != nil && rows[0].Content[0].Type == "tableHeader") + `">`)
	out.WriteString(`<hp:sz width="` + strconv.Itoa(column) + `" widthRelTo="ABSOLUTE" height="0" heightRelTo="ABSOLUTE"/>`)
	out.WriteString(`<hp:pos treatAsChar="1" vertRelTo="PARA" horzRelTo="PARA"/>`)
	// Merged cells occupy addresses in later rows, which the next cell in
	// that row has to step over.
	occupied := map[[2]int]bool{}
	for rowIndex, row := range rows {
		out.WriteString(`<hp:tr>`)
		columnIndex := 0
		for _, cell := range row.Content {
			if cell == nil || (cell.Type != "tableCell" && cell.Type != "tableHeader") {
				continue
			}
			for occupied[[2]int{rowIndex, columnIndex}] {
				columnIndex++
			}
			span := cellSpan(cell, "colspan")
			rowSpan := cellSpan(cell, "rowspan")
			for down := 0; down < rowSpan; down++ {
				for across := 0; across < span; across++ {
					occupied[[2]int{rowIndex + down, columnIndex + across}] = true
				}
			}
			header := cell.Type == "tableHeader"
			out.WriteString(`<hp:tc name="" header="` + strconv.FormatBool(header) + `" hasMargin="0" protect="0" editable="0" dirty="0" borderFillIDRef="1"`)
			if align := cellVerticalAlignValue(cell); align != "" {
				out.WriteString(` vertAlign="` + align + `"`)
			}
			out.WriteString(`>`)
			out.WriteString(`<hp:cellAddr colAddr="` + strconv.Itoa(columnIndex) + `" rowAddr="` + strconv.Itoa(rowIndex) + `"/>`)
			out.WriteString(`<hp:cellSpan colSpan="` + strconv.Itoa(span) + `" rowSpan="` + strconv.Itoa(rowSpan) + `"/>`)
			out.WriteString(`<hp:cellSz width="` + strconv.Itoa(cellWidth*span) + `" height="0"/>`)
			out.WriteString(`<hp:cellMargin left="510" right="510" top="141" bottom="141"/>`)
			out.WriteString(`<hp:subList>`)
			saved := b.body
			b.body = strings.Builder{}
			child := ctx
			child.bold = header
			child.indent = 0
			if len(cell.Content) == 0 {
				b.paragraph(nil, child, nil, "")
			} else {
				b.blocks(cell.Content, child)
			}
			out.WriteString(b.body.String())
			b.body = saved
			out.WriteString(`</hp:subList></hp:tc>`)
			columnIndex += span
		}
		out.WriteString(`</hp:tr>`)
	}
	out.WriteString(`</hp:tbl></hp:run></hp:p>`)
	b.body.WriteString(out.String())
}

func cellSpan(cell *richdoc.Node, name string) int {
	span := cell.AttrInt(name, 1)
	if span < 1 {
		return 1
	}
	if span > 64 {
		return 64
	}
	return span
}

func cellVerticalAlignValue(cell *richdoc.Node) string {
	switch cell.AttrString("verticalAlign") {
	case "middle":
		return "CENTER"
	case "bottom":
		return "BOTTOM"
	case "top":
		return "TOP"
	}
	return ""
}
