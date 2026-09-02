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
	// Header and Footer are the one line of each muni keeps, printed on
	// every page.
	Header string
	Footer string
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
		fonts:    []string{"함초롬바탕"},
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

	fonts      []string        // faces, at the numbers the runs refer to them by
	preview    strings.Builder // the opening text, for Preview/PrvText.txt
	paragraphs int             // paragraphs written, which numbers the next
	objects    int             // pictures and tables written, which orders the next
}

// openParagraph writes the opening tag of a paragraph, numbered the way
// Hangul numbers its own.
func (b *builder) openParagraph(paraPr, style int, pageBreak bool) string {
	id := b.paragraphs
	b.paragraphs++
	flag := "0"
	if pageBreak {
		flag = "1"
	}
	return `<hp:p id="` + strconv.Itoa(id) + `" paraPrIDRef="` + strconv.Itoa(paraPr) + `" styleIDRef="` + strconv.Itoa(style) +
		`" pageBreak="` + flag + `" columnBreak="0" merged="0">`
}

// nextObject numbers a picture or table in drawing order.
func (b *builder) nextObject() int {
	b.objects++
	return b.objects
}

// notePreview keeps the first kilobyte of text for the preview part.
func (b *builder) notePreview(text string) {
	if b.preview.Len() < 1024 {
		b.preview.WriteString(text)
	}
}

// openSubList opens the paragraph list inside a cell or a note. Its vertical
// alignment is the cell's: that attribute lives here and nowhere else.
func openSubList(vertAlign string) string {
	return `<hp:subList id="" textDirection="HORIZONTAL" lineWrap="BREAK" vertAlign="` + vertAlign +
		`" linkListIDRef="0" linkListNextIDRef="0" textWidth="0" textHeight="0" hasTextRef="0" hasNumRef="0">`
}

// inlinePosition anchors a picture or table in the text like a character.
const inlinePosition = `<hp:pos treatAsChar="1" affectLSpacing="0" flowWithText="1" allowOverlap="0" holdAnchorAndSO="0" vertRelTo="PARA" horzRelTo="PARA" vertAlign="TOP" horzAlign="LEFT" vertOffset="0" horzOffset="0"/>`

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
	// list is the kind of list a paragraph is an item of, and level how
	// deep: a list in HWPX is a shape its paragraphs share, not an element.
	list  string
	level int
}

type blockContext struct {
	indent   int
	heading  int
	quote    bool
	mono     bool
	bold     bool
	listMark string
	// listKind and listDepth say which list the next paragraph is an item
	// of, and how many lists enclose it.
	listKind  string
	listDepth int
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
	case "bulletList", "orderedList":
		b.list(node, ctx, node.Type)
	case "taskList":
		b.taskList(node, ctx)
	case "horizontalRule":
		key := paraKey{align: "center"}
		b.body.WriteString(b.openParagraph(b.paraPrID(key), 0, false) +
			`<hp:run charPrIDRef="0"><hp:t>` + strings.Repeat("─", 30) + `</hp:t></hp:run></hp:p>`)
	case "pageBreak":
		b.body.WriteString(b.openParagraph(0, 0, true) + `<hp:run charPrIDRef="0"><hp:t/></hp:run></hp:p>`)
	case "table":
		b.table(node, ctx)
	case "image":
		if picture := b.picture(node); picture != "" {
			key := paraKey{align: hangul.Alignment(node.AttrString("textAlign"))}
			b.body.WriteString(b.openParagraph(b.paraPrID(key), 0, false) +
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

// list writes the items of a bullet or numbered list. The list itself is
// not written: its paragraphs share a shape that says "bullet" or "number"
// and how deep, and Hangul draws the marks from that.
func (b *builder) list(node *richdoc.Node, ctx blockContext, kind string) {
	for _, item := range node.Content {
		if item == nil || item.Type != "listItem" {
			continue
		}
		child := ctx
		child.indent++
		child.listMark = ""
		child.listKind = kind
		child.listDepth = ctx.listDepth + 1
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

// listItem writes an item's first paragraph as the item — with the list's
// shape, or a task's mark in front of it — and the rest beneath it as plain
// paragraphs.
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
		plain.listKind = ""
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
	if ctx.listKind != "" {
		key.list = ctx.listKind
		key.level = ctx.listDepth - 1
	}
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
	b.body.WriteString(b.openParagraph(b.paraPrID(key), style, false))
	if prefix != "" {
		b.body.WriteString(`<hp:run charPrIDRef="` + strconv.Itoa(b.charPrID(b.baseKey(ctx))) +
			`"><hp:t>` + escape(prefix) + `</hp:t></hp:run>`)
	}
	b.inline(inline, ctx)
	if len(inline) == 0 && prefix == "" {
		b.body.WriteString(`<hp:run charPrIDRef="0"><hp:t/></hp:run>`)
	}
	b.body.WriteString(`</hp:p>`)
	b.notePreview("\n")
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
			b.notePreview(node.Text)
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
	b.body.WriteString(`<hp:run charPrIDRef="0"><hp:footNote>` + openSubList("TOP") + b.openParagraph(0, 0, false))
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
			// Inside the text element, not between two of them: that is
			// where Hangul writes it, and where the reader looks for it.
			out.WriteString("<hp:tab/>")
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
	w, h := strconv.Itoa(width), strconv.Itoa(height)
	pixelsWide, pixelsHigh := strconv.Itoa(width*96/hangul.UnitsPerInch), strconv.Itoa(height*96/hangul.UnitsPerInch)
	number := strconv.Itoa(b.nextObject())
	// Every element Hangul writes for a picture, in its order, with the
	// picture unrotated, unclipped and unscaled. A loader that has only ever
	// seen Hangul's pictures expects each of them to be there.
	return `<hp:pic id="` + number + `" zOrder="` + number + `" numberingType="PICTURE" textWrap="TOP_AND_BOTTOM" textFlow="BOTH_SIDES" lock="0" dropcapstyle="None" href="" groupLevel="0" instid="` + number + `" reverse="0">` +
		`<hp:offset x="0" y="0"/><hp:orgSz width="` + w + `" height="` + h + `"/><hp:curSz width="` + w + `" height="` + h + `"/>` +
		`<hp:flip horizontal="0" vertical="0"/>` +
		`<hp:rotationInfo angle="0" centerX="` + strconv.Itoa(width/2) + `" centerY="` + strconv.Itoa(height/2) + `" rotateimage="1"/>` +
		`<hp:renderingInfo><hc:transMatrix e1="1" e2="0" e3="0" e4="0" e5="1" e6="0"/><hc:scaMatrix e1="1" e2="0" e3="0" e4="0" e5="1" e6="0"/><hc:rotMatrix e1="1" e2="0" e3="0" e4="0" e5="1" e6="0"/></hp:renderingInfo>` +
		`<hp:imgRect><hc:pt0 x="0" y="0"/><hc:pt1 x="` + w + `" y="0"/><hc:pt2 x="` + w + `" y="` + h + `"/><hc:pt3 x="0" y="` + h + `"/></hp:imgRect>` +
		`<hp:imgClip left="0" right="` + pixelsWide + `" top="0" bottom="` + pixelsHigh + `"/>` +
		`<hp:inMargin left="0" right="0" top="0" bottom="0"/>` +
		`<hp:imgDim dimwidth="` + pixelsWide + `" dimheight="` + pixelsHigh + `"/>` +
		`<hc:img binaryItemIDRef="` + id + `" bright="0" contrast="0" effect="REAL_PIC" alpha="0"/>` +
		`<hp:effects/>` +
		`<hp:sz width="` + w + `" widthRelTo="ABSOLUTE" height="` + h + `" heightRelTo="ABSOLUTE" protect="0"/>` +
		inlinePosition +
		`<hp:outMargin left="0" right="0" top="0" bottom="0"/>` +
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

	header := len(rows[0].Content) > 0 && rows[0].Content[0] != nil && rows[0].Content[0].Type == "tableHeader"
	// Heights are nominal, a line per paragraph: Hangul lays the table out
	// again when it opens the file, but a height of nothing is not a height
	// it accepts.
	const lineHeight = 1900
	rowHeights := make([]int, len(rows))
	for rowIndex, row := range rows {
		rowHeights[rowIndex] = lineHeight
		for _, cell := range row.Content {
			if cell != nil && cellSpan(cell, "rowspan") == 1 && len(cell.Content)*lineHeight > rowHeights[rowIndex] {
				rowHeights[rowIndex] = len(cell.Content) * lineHeight
			}
		}
	}
	tableHeight := 0
	for _, height := range rowHeights {
		tableHeight += height
	}
	number := strconv.Itoa(b.nextObject())

	var out strings.Builder
	out.WriteString(b.openParagraph(0, 0, false) + `<hp:run charPrIDRef="0">`)
	out.WriteString(`<hp:tbl id="` + number + `" zOrder="` + number + `" numberingType="TABLE" textWrap="TOP_AND_BOTTOM" textFlow="BOTH_SIDES" lock="0" dropcapstyle="None" pageBreak="CELL" repeatHeader="` +
		flag(header) + `" rowCnt="` + strconv.Itoa(len(rows)) + `" colCnt="` + strconv.Itoa(columns) +
		`" cellSpacing="0" borderFillIDRef="` + strconv.Itoa(tableBorder) + `" noAdjust="0">`)
	out.WriteString(`<hp:sz width="` + strconv.Itoa(column) + `" widthRelTo="ABSOLUTE" height="` + strconv.Itoa(tableHeight) + `" heightRelTo="ABSOLUTE" protect="0"/>`)
	out.WriteString(inlinePosition)
	out.WriteString(`<hp:outMargin left="283" right="283" top="283" bottom="283"/><hp:inMargin left="510" right="510" top="141" bottom="141"/>`)
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
			cellHeight := 0
			for down := 0; down < rowSpan; down++ {
				for across := 0; across < span; across++ {
					occupied[[2]int{rowIndex + down, columnIndex + across}] = true
				}
				if rowIndex+down < len(rowHeights) {
					cellHeight += rowHeights[rowIndex+down]
				}
			}
			header := cell.Type == "tableHeader"
			// The paragraphs come first inside a cell and the address after;
			// the order is the format's, and a loader holds to it.
			out.WriteString(`<hp:tc name="" header="0" hasMargin="0" protect="0" editable="0" dirty="0" borderFillIDRef="` + strconv.Itoa(tableBorder) + `">`)
			out.WriteString(openSubList(cellVerticalAlignValue(cell)))
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
			out.WriteString(`</hp:subList>`)
			out.WriteString(`<hp:cellAddr colAddr="` + strconv.Itoa(columnIndex) + `" rowAddr="` + strconv.Itoa(rowIndex) + `"/>`)
			out.WriteString(`<hp:cellSpan colSpan="` + strconv.Itoa(span) + `" rowSpan="` + strconv.Itoa(rowSpan) + `"/>`)
			out.WriteString(`<hp:cellSz width="` + strconv.Itoa(cellWidth*span) + `" height="` + strconv.Itoa(cellHeight) + `"/>`)
			out.WriteString(`<hp:cellMargin left="510" right="510" top="141" bottom="141"/>`)
			out.WriteString(`</hp:tc>`)
			columnIndex += span
		}
		out.WriteString(`</hp:tr>`)
	}
	out.WriteString(`</hp:tbl></hp:run></hp:p>`)
	b.body.WriteString(out.String())
}

// flag writes a yes or no the way the format spells them.
func flag(value bool) string {
	if value {
		return "1"
	}
	return "0"
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
	}
	return "TOP"
}
