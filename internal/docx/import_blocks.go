package docx

import (
	"math"
	"strconv"
	"strings"

	"github.com/hkjang/muni/internal/richdoc"
)

// block is an intermediate form: paragraphs keep their list membership so a
// second pass can rebuild the nested list structure OOXML flattens away.
type block struct {
	node    *richdoc.Node
	numID   string
	level   int
	kind    string // bullet | ordered | task
	checked bool
}

func (imp *importer) blocks(nodes []*xnode) []block {
	out := make([]block, 0, len(nodes))
	// The cached entries of a table of contents, which are skipped rather than
	// read. See tocEntrySpan.
	skipUntil := -1
	for index, node := range nodes {
		switch {
		case node.is("w", "p"):
			if index <= skipUntil {
				continue
			}
			if end, ok := tocEntrySpan(nodes, index); ok {
				out = append(out, block{node: &richdoc.Node{Type: richdoc.TableOfContentsType}})
				skipUntil = end
				continue
			}
			out = append(out, imp.paragraph(node)...)
		case node.is("w", "tbl"):
			if table := imp.table(node); table != nil {
				out = append(out, block{node: table})
			}
		case node.is("w", "sdt"):
			if content := node.child("w", "sdtContent"); content != nil {
				out = append(out, imp.blocks(content.Children)...)
			}
		case node.is("mc", "AlternateContent"):
			branch := alternateContent(node)
			out = append(out, imp.blocks(branch)...)
			// A shape sitting at block level carries words too, and the block
			// walker only knows paragraphs and tables.
			if words := imp.shapes(branch, nil); len(words) > 0 {
				out = append(out, block{node: richdoc.Paragraph(words...)})
			}
		}
	}
	return out
}

// Word writes a table of contents as a field wrapped around the entries it
// last calculated: a paragraph opens the field, one paragraph per entry
// follows carrying a heading and the page it was on, and a paragraph closes
// it. Those entries are a cache — Word rebuilds them on demand — and in muni
// they are neither rebuilt nor meaningful, because muni has no pages until a
// document is printed. Importing them as prose gives a document a frozen list
// of page numbers that will never be right again.
//
// muni's own contents node is generated from the headings, so the field
// becomes that node and the cache is dropped.

// fieldDelta reports what a paragraph does to the number of open fields.
func fieldDelta(paragraph *xnode) int {
	delta := 0
	var walk func(*xnode)
	walk = func(node *xnode) {
		if node.is("w", "fldChar") {
			switch strings.ToLower(node.attr("w:fldCharType")) {
			case "begin":
				delta++
			case "end":
				delta--
			}
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(paragraph)
	return delta
}

// fieldInstructions reports the field instructions a paragraph carries.
func fieldInstructions(paragraph *xnode) []string {
	var out []string
	var walk func(*xnode)
	walk = func(node *xnode) {
		switch {
		case node.is("w", "instrText"):
			out = append(out, node.Text)
		case node.is("w", "fldSimple"):
			// The one-element form, which opens and closes at once.
			out = append(out, node.attr("w:instr"))
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(paragraph)
	return out
}

// tocFieldInstruction reports whether a field instruction asks for a table of
// contents. Word writes ` TOC \o "1-3" \h \z \u `; the switches after the
// name say which headings and how to link them, and muni decides both itself.
func tocFieldInstruction(instruction string) bool {
	fields := strings.Fields(instruction)
	return len(fields) > 0 && strings.EqualFold(fields[0], "TOC")
}

// tocEntrySpan reports where a table of contents starting at a paragraph ends,
// and whether it is one at all.
//
// The span is worked out before anything is skipped. A running "am I still
// inside it" flag has nowhere to stop: the field's closing fldChar can sit in
// a table cell or a content control, which this walker descends into
// separately, and a file can simply be unbalanced. Either way the flag would
// stay set and every remaining paragraph in the document would be dropped —
// the import succeeding, and the document ending at its table of contents.
//
// Not finding the end means the span is the opening paragraph alone. Losing
// the cached entries of a malformed field is a smaller wrong than losing
// everything after it.
func tocEntrySpan(nodes []*xnode, start int) (end int, isTOC bool) {
	opens := false
	for _, instruction := range fieldInstructions(nodes[start]) {
		if tocFieldInstruction(instruction) {
			opens = true
		}
	}
	if !opens {
		return 0, false
	}
	depth := fieldDelta(nodes[start])
	if depth <= 0 {
		// Opened and closed in the one paragraph.
		return start, true
	}
	for index := start + 1; index < len(nodes); index++ {
		if !nodes[index].is("w", "p") {
			continue
		}
		if depth += fieldDelta(nodes[index]); depth <= 0 {
			return index, true
		}
	}
	return start, true
}

// styledParagraphProperties returns a paragraph's properties with the ones its
// style supplies filled in underneath.
//
// An office template keeps the shape of its body text in a style: 들여쓰기 and
// 줄간격 are set once in "본문" and every paragraph just names it. muni read the
// paragraph alone, so a document written the way templates are written arrived
// with none of its layout.
//
// The paragraph wins, and the two properties that are read elsewhere are left
// out: w:pStyle is the reference rather than a property, and w:numPr already
// has its own path through styleNumbering.
func (imp *importer) styledParagraphProperties(properties *xnode, styleID string) *xnode {
	if styleID == "" {
		return properties
	}
	merged := &xnode{Space: "w", Local: "pPr"}
	if properties != nil {
		merged.Space, merged.Local, merged.Attrs = properties.Space, properties.Local, properties.Attrs
	}
	chain := make([][]*xnode, 0, 4)
	for depth := 0; styleID != "" && depth < 16; depth++ {
		style, ok := imp.styles[styleID]
		if !ok {
			break
		}
		chain = append(chain, style.paragraphProperties.children())
		styleID = style.basedOn
	}
	// w:pStyle is the reference rather than a property, and w:numPr already
	// has its own path through styleNumbering.
	merged.Children = mergeProperties(properties.children(), chain, "w:pStyle", "w:numPr")
	return merged
}

// mergeProperties layers a chain of style property lists under a node's own,
// attribute by attribute rather than element by element. The skip list names
// what must not be inherited; a node's own properties are never dropped.
//
// OOXML inherits per attribute. A style that sets
// `<w:spacing w:line="384" w:lineRule="auto"/>` and a paragraph that sets only
// `<w:spacing w:before="240"/>` render at 160% in Word — the paragraph says
// nothing about w:line, so the style still speaks. Skipping the style's
// element because the paragraph has one of its own loses the line spacing
// entirely, which is most of what reading styles was for. The same shape turns
// up on runs, where a Korean document routinely writes a bare
// `<w:rFonts w:hint="eastAsia"/>` that would otherwise silence the style's
// font.
func mergeProperties(own []*xnode, chain [][]*xnode, skip ...string) []*xnode {
	ignored := map[string]bool{}
	for _, key := range skip {
		ignored[key] = true
	}
	order := make([]string, 0, len(own))
	byKey := map[string]*xnode{}
	take := func(child *xnode, fromStyle bool) {
		// The prefix, not the namespace URI a node actually carries: the
		// caller names what to skip the way the file writes it.
		key := prefixFor(child.Space) + ":" + child.Local
		// Skipping means "do not inherit this", never "discard it". What the
		// node says about itself is always its own: a list item's w:numPr is
		// what makes it a list item.
		if fromStyle && ignored[key] {
			return
		}
		existing, seen := byKey[key]
		if !seen {
			// A style's element is copied, never shared: a later paragraph
			// naming the same style must not see attributes this one gained.
			copied := &xnode{Space: child.Space, Local: child.Local, Text: child.Text, Children: child.Children}
			copied.Attrs = map[string]string{}
			for name, value := range child.Attrs {
				copied.Attrs[name] = value
			}
			byKey[key] = copied
			order = append(order, key)
			return
		}
		if !fromStyle {
			return
		}
		for name, value := range child.Attrs {
			if _, set := existing.Attrs[name]; !set {
				existing.Attrs[name] = value
			}
		}
		if len(existing.Children) == 0 {
			existing.Children = child.Children
		}
	}
	for _, child := range own {
		take(child, false)
	}
	for _, level := range chain {
		for _, child := range level {
			take(child, true)
		}
	}
	out := make([]*xnode, 0, len(order))
	for _, key := range order {
		out = append(out, byKey[key])
	}
	return out
}

func (imp *importer) paragraph(node *xnode) []block {
	own := node.child("w", "pPr")
	properties := imp.styledParagraphProperties(own, own.child("w", "pStyle").val())
	styleKey := imp.styleKey(properties.child("w", "pStyle").val())
	align := alignmentFromJc(properties.child("w", "jc").val())

	styleID := properties.child("w", "pStyle").val()
	numID, level := "", 0
	if numbering := properties.child("w", "numPr"); numbering != nil {
		numID = numbering.child("w", "numId").val()
		level, _ = strconv.Atoi(numbering.child("w", "ilvl").val())
	}
	if numID == "0" {
		numID = ""
	}
	if numID == "" {
		if styleNum, styleLevel, ok := imp.styleNumbering(styleID); ok {
			numID, level = styleNum, styleLevel
		}
	}
	// The built-in "List Bullet"/"List Number" families encode their depth in
	// the style name, and each level often carries its own numbering id. Group
	// the whole family under one synthetic list so nesting survives.
	styleList, styleListLevel := listFromStyleKey(styleKey)
	if styleList != "" {
		numID = "style:" + styleList
		if styleListLevel > level {
			level = styleListLevel
		}
	}

	inline := imp.runs(node.Children, richdoc.Mark{})
	inline = trimInline(inline)

	// A page break is a block of its own in muni, so a paragraph that carries
	// one has to be split around it. Word writes the break either as a
	// property on the paragraph or as a run inside it, and both are common.
	breakBefore := properties.child("w", "pageBreakBefore").flag()
	inline, breakInside := splitPageBreak(inline)

	// A paragraph that only holds a page break carries no content of its own.
	if len(inline) == 0 && numID == "" && styleKey == "" && node.descendant("w", "br") != nil && node.allText() == "" {
		if breakBefore || breakInside {
			return []block{{node: &richdoc.Node{Type: "pageBreak"}}}
		}
		return nil
	}

	kind := ""
	checked := false
	if numID != "" {
		kind = imp.listKinds[numID+":"+strconv.Itoa(level)]
		if kind == "" {
			kind = styleList
		}
		if kind == "" {
			kind = "bullet"
		}
	}
	if state, stripped, ok := detectCheckbox(inline); ok {
		kind, checked, inline = "task", state, stripped
	}
	if box := properties.descendant("w14", "checkbox"); box != nil {
		kind = "task"
		checked = box.child("w14", "checked").flag()
	}

	// Whatever the paragraph turns out to be, a page break belongs in front of
	// it as a block of its own.
	prefix := []block{}
	if breakBefore || breakInside {
		prefix = append(prefix, block{node: &richdoc.Node{Type: "pageBreak"}})
	}

	if styleKey == "code" {
		text := plainInline(inline)
		return append(prefix, block{node: codeBlockNode(text)})
	}

	var paragraph *richdoc.Node
	switch {
	case strings.HasPrefix(styleKey, "heading"):
		level, _ := strconv.Atoi(strings.TrimPrefix(styleKey, "heading"))
		if level < 1 || level > 6 {
			level = 1
		}
		paragraph = &richdoc.Node{Type: "heading", Content: inline}
		paragraph.SetAttr("level", level)
	case styleKey == "title":
		paragraph = &richdoc.Node{Type: "heading", Content: inline}
		paragraph.SetAttr("level", 1)
	case styleKey == "subtitle":
		paragraph = &richdoc.Node{Type: "heading", Content: inline}
		paragraph.SetAttr("level", 2)
	default:
		if outline := properties.child("w", "outlineLvl").val(); outline != "" && numID == "" {
			if value, err := strconv.Atoi(outline); err == nil && value >= 0 && value <= 5 {
				paragraph = &richdoc.Node{Type: "heading", Content: inline}
				paragraph.SetAttr("level", value+1)
			}
		}
		if paragraph == nil {
			paragraph = &richdoc.Node{Type: "paragraph", Content: inline}
		}
	}
	if align != "" && paragraph.Type != "codeBlock" {
		paragraph.SetAttr("textAlign", align)
	}
	if paragraph.Type != "codeBlock" {
		applyParagraphSpacing(paragraph, properties, kind != "")
	}

	result := block{node: paragraph, numID: numID, level: level, kind: kind, checked: checked}
	if styleKey == "quote" && kind == "" {
		return append(prefix, block{node: &richdoc.Node{Type: "blockquote", Content: []*richdoc.Node{paragraph}}})
	}
	if borders := properties.child("w", "pBdr"); borders != nil && len(inline) == 0 && kind == "" {
		if borders.child("w", "bottom") != nil || borders.child("w", "top") != nil {
			return append(prefix, block{node: &richdoc.Node{Type: "horizontalRule"}})
		}
	}
	return append(prefix, result)
}

func codeBlockNode(text string) *richdoc.Node {
	node := &richdoc.Node{Type: "codeBlock"}
	if text != "" {
		node.Content = []*richdoc.Node{richdoc.Text(text)}
	}
	return node
}

func alignmentFromJc(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "center":
		return "center"
	case "right", "end":
		return "right"
	case "both", "distribute", "justify":
		return "justify"
	case "left", "start":
		return "left"
	default:
		return ""
	}
}

func trimInline(nodes []*richdoc.Node) []*richdoc.Node {
	for len(nodes) > 0 {
		last := nodes[len(nodes)-1]
		if last.Type == "text" && strings.TrimSpace(last.Text) == "" {
			nodes = nodes[:len(nodes)-1]
			continue
		}
		if last.Type == "hardBreak" {
			nodes = nodes[:len(nodes)-1]
			continue
		}
		break
	}
	return nodes
}

func plainInline(nodes []*richdoc.Node) string {
	var out strings.Builder
	for _, node := range nodes {
		switch node.Type {
		case "text":
			out.WriteString(node.Text)
		case "hardBreak":
			out.WriteString("\n")
		}
	}
	return out.String()
}

// detectCheckbox recognises the leading glyph muni writes for task items and
// the "[ ] / [x]" convention other editors produce.
func detectCheckbox(nodes []*richdoc.Node) (bool, []*richdoc.Node, bool) {
	if len(nodes) == 0 || nodes[0].Type != "text" {
		return false, nodes, false
	}
	text := nodes[0].Text
	trimmed := strings.TrimLeft(text, " \t")
	checkedPrefixes := []string{"☒ ", "☑ ", "[x] ", "[X] ", " ", "☒", "☑"}
	uncheckedPrefixes := []string{"☐ ", "□ ", "[ ] ", " ", "☐", "□"}
	for _, prefix := range checkedPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true, replaceFirstText(nodes, strings.TrimPrefix(trimmed, prefix)), true
		}
	}
	for _, prefix := range uncheckedPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return false, replaceFirstText(nodes, strings.TrimPrefix(trimmed, prefix)), true
		}
	}
	return false, nodes, false
}

func replaceFirstText(nodes []*richdoc.Node, value string) []*richdoc.Node {
	value = strings.TrimLeft(value, " ")
	if value == "" {
		return nodes[1:]
	}
	head := *nodes[0]
	head.Text = value
	out := make([]*richdoc.Node, 0, len(nodes))
	out = append(out, &head)
	out = append(out, nodes[1:]...)
	return out
}

// groupBlocks rebuilds nested bullet/ordered/task lists from the flat
// paragraph stream, then wraps everything into the final block sequence.
func groupBlocks(blocks []block) []*richdoc.Node {
	out := make([]*richdoc.Node, 0, len(blocks))
	index := 0
	for index < len(blocks) {
		current := blocks[index]
		if current.kind == "" {
			// Word requires a paragraph between and after tables; those
			// spacers carry no content and only clutter the imported document.
			if current.node != nil && current.node.Type == "paragraph" && current.node.IsBlank() &&
				len(out) > 0 && out[len(out)-1].Type == "table" {
				index++
				continue
			}
			out = append(out, current.node)
			index++
			continue
		}
		end := index
		for end < len(blocks) && blocks[end].kind != "" {
			end++
		}
		out = append(out, buildLists(blocks[index:end], 0)...)
		index = end
	}
	return out
}

func buildLists(items []block, depth int) []*richdoc.Node {
	out := make([]*richdoc.Node, 0, 4)
	index := 0
	for index < len(items) {
		kind := items[index].kind
		numID := items[index].numID
		baseLevel := items[index].level
		end := index
		for end < len(items) {
			candidate := items[end]
			if candidate.level < baseLevel {
				break
			}
			if candidate.level == baseLevel && (candidate.kind != kind || candidate.numID != numID) {
				break
			}
			end++
		}
		out = append(out, buildOneList(items[index:end], kind, baseLevel, depth))
		index = end
	}
	return out
}

func buildOneList(items []block, kind string, baseLevel, depth int) *richdoc.Node {
	listType := "bulletList"
	itemType := "listItem"
	switch kind {
	case "ordered":
		listType = "orderedList"
	case "task":
		listType, itemType = "taskList", "taskItem"
	}
	list := &richdoc.Node{Type: listType}
	if listType == "orderedList" {
		list.SetAttr("start", 1)
	}
	index := 0
	for index < len(items) {
		current := items[index]
		item := &richdoc.Node{Type: itemType, Content: []*richdoc.Node{current.node}}
		if itemType == "taskItem" {
			item.SetAttr("checked", current.checked)
		}
		index++
		nested := index
		for nested < len(items) && items[nested].level > baseLevel {
			nested++
		}
		if nested > index && depth < 8 {
			item.Content = append(item.Content, buildLists(items[index:nested], depth+1)...)
			index = nested
		}
		list.Content = append(list.Content, item)
	}
	return list
}

func (imp *importer) table(node *xnode) *richdoc.Node {
	widths := make([]int, 0, 8)
	if grid := node.child("w", "tblGrid"); grid != nil {
		for _, column := range grid.Children {
			if !column.is("w", "gridCol") {
				continue
			}
			twips, _ := strconv.Atoi(column.attr("w:w"))
			widths = append(widths, twipsToPixels(twips))
		}
	}

	firstRowIsHeader := imp.tableLeadsWithAHeader(node.child("w", "tblPr"))

	type pending struct {
		cell   *richdoc.Node
		column int
		span   int
	}
	rows := make([]*richdoc.Node, 0, 8)
	open := map[int]*pending{}
	// Which w:tr this is, not how many rows have been kept: a first row that
	// yields no cells is skipped without being counted, and the header would
	// land on the row after it.
	rowIndex := -1
	for _, rowNode := range node.Children {
		if !rowNode.is("w", "tr") {
			continue
		}
		rowIndex++
		header := firstRowIsHeader && rowIndex == 0
		if properties := rowNode.child("w", "trPr"); properties != nil && properties.child("w", "tblHeader") != nil {
			header = true
		}
		row := &richdoc.Node{Type: "tableRow"}
		column := 0
		for _, cellNode := range rowNode.Children {
			if !cellNode.is("w", "tc") {
				continue
			}
			properties := cellNode.child("w", "tcPr")
			span := 1
			if gridSpan := properties.child("w", "gridSpan"); gridSpan != nil {
				if value, err := strconv.Atoi(gridSpan.val()); err == nil && value > 0 {
					span = value
				}
			}
			merge := properties.child("w", "vMerge")
			continuation := merge != nil && strings.ToLower(merge.val()) != "restart"
			if continuation {
				if previous, ok := open[column]; ok && previous != nil {
					previous.cell.SetAttr("rowspan", previous.cell.AttrInt("rowspan", 1)+1)
				}
				column += span
				continue
			}
			cellType := "tableCell"
			if header || cellIsHeader(cellNode, properties) {
				cellType = "tableHeader"
			}
			cell := &richdoc.Node{Type: cellType}
			cell.SetAttr("colspan", span)
			cell.SetAttr("rowspan", 1)
			// muni can hold a cell shade now, so an imported one is kept
			// rather than only used as a hint about which row is the header.
			if shade := cellShade(properties); shade != "" && cellType != "tableHeader" {
				cell.SetAttr("backgroundColor", shade)
			}
			// Word's default is the top, and saying nothing means the top.
			// muni draws a cell centred unless told otherwise, so an imported
			// cell has to say where its text sat or it moves.
			cell.SetAttr("verticalAlign", cellVerticalAlign(properties))
			if columnWidths := widthsFor(widths, column, span); len(columnWidths) > 0 {
				cell.SetAttr("colwidth", columnWidths)
			}
			content := groupBlocks(imp.blocks(cellNode.Children))
			if len(content) == 0 {
				content = []*richdoc.Node{richdoc.Paragraph()}
			}
			cell.Content = content
			row.Content = append(row.Content, cell)
			if merge != nil {
				open[column] = &pending{cell: cell, column: column, span: span}
			} else {
				delete(open, column)
			}
			column += span
		}
		if len(row.Content) > 0 {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return nil
	}
	return &richdoc.Node{Type: "table", Content: rows}
}

func cellIsHeader(cell *xnode, properties *xnode) bool {
	if properties != nil {
		if shading := properties.child("w", "shd"); shading != nil {
			fill := strings.ToUpper(strings.TrimSpace(shading.attr("w:fill")))
			if fill != "" && fill != "AUTO" && fill != "FFFFFF" {
				// Shaded cells are only treated as headers when fully bold.
				return allRunsBold(cell)
			}
		}
	}
	return false
}

func allRunsBold(cell *xnode) bool {
	found := false
	var walk func(*xnode) bool
	walk = func(node *xnode) bool {
		for _, child := range node.Children {
			if child.is("w", "r") {
				if strings.TrimSpace(child.allText()) == "" {
					continue
				}
				found = true
				if !child.child("w", "rPr").child("w", "b").flag() {
					return false
				}
				continue
			}
			if !walk(child) {
				return false
			}
		}
		return true
	}
	return walk(cell) && found
}

func widthsFor(widths []int, column, span int) []any {
	if len(widths) == 0 {
		return nil
	}
	out := make([]any, 0, span)
	for offset := 0; offset < span; offset++ {
		index := column + offset
		if index >= len(widths) || widths[index] <= 0 {
			return nil
		}
		out = append(out, widths[index])
	}
	return out
}

func twipsToPixels(twips int) int {
	if twips <= 0 {
		return 0
	}
	return int(float64(twips)/twipsPerInch*96 + 0.5)
}

// listFromStyleKey maps the built-in list style families onto a list kind and
// nesting depth ("List Bullet 2" is the second level).
func listFromStyleKey(styleKey string) (string, int) {
	for _, prefix := range []struct{ name, kind string }{{"listbullet", "bullet"}, {"listnumber", "ordered"}} {
		if !strings.HasPrefix(styleKey, prefix.name) {
			continue
		}
		level, err := strconv.Atoi(strings.TrimPrefix(styleKey, prefix.name))
		if err != nil || level < 1 {
			level = 1
		}
		if level > 9 {
			level = 9
		}
		return prefix.kind, level - 1
	}
	return "", 0
}

// splitPageBreak takes the page-break markers out of a paragraph's inline
// content and reports whether there were any. Everything after a break stays
// in the same paragraph: Word's own layout does the same, and splitting the
// sentence would change the text rather than the pagination.
func splitPageBreak(inline []*richdoc.Node) ([]*richdoc.Node, bool) {
	found := false
	out := make([]*richdoc.Node, 0, len(inline))
	for _, node := range inline {
		if node != nil && node.Type == "pageBreak" {
			found = true
			continue
		}
		out = append(out, node)
	}
	return out, found
}

// cellShade reads a cell's fill colour, if it has one worth keeping. White and
// automatic are the absence of a shade, not a shade.
func cellShade(properties *xnode) string {
	if properties == nil {
		return ""
	}
	shading := properties.child("w", "shd")
	if shading == nil {
		return ""
	}
	fill := strings.ToUpper(strings.TrimSpace(shading.attr("w:fill")))
	if fill == "" || fill == "AUTO" || fill == "FFFFFF" || len(fill) != 6 {
		return ""
	}
	for _, r := range fill {
		if !((r >= '0' && r <= '9') || (r >= 'A' && r <= 'F')) {
			return ""
		}
	}
	return "#" + strings.ToLower(fill)
}

// applyParagraphSpacing carries indentation and line spacing back out of the
// file.
//
// Export wrote all three — w:ind w:left, w:ind w:firstLine, w:spacing w:line —
// and import read none of them, so muni could not read its own output.
// Formatting a document, exporting it to Word and opening it again came back
// flat, and a Word document written the way a Korean office writes one — 줄
// 간격 160%, 첫 줄 들여쓰기 — lost both on the way in.
//
// A list item's indentation is the list's own, expressed in the same
// attribute; taking it as an author's setting would indent every bullet a
// second time.
func applyParagraphSpacing(paragraph *richdoc.Node, properties *xnode, inList bool) {
	if paragraph == nil || properties == nil {
		return
	}

	if indent := properties.child("w", "ind"); indent != nil && !inList {
		// Rounded to the nearest step rather than truncated: a document
		// written by Word uses whatever measurement its author dragged the
		// ruler to, and 430 twips is one step, not none.
		if left, err := strconv.Atoi(indent.attr("w:left")); err == nil && left > 0 {
			steps := (left + twipsPerIndentStep/2) / twipsPerIndentStep
			if steps > maxIndentSteps {
				steps = maxIndentSteps
			}
			if steps > 0 {
				paragraph.SetAttr("indent", steps)
			}
		}
		// muni's first-line indent is on or off, so any positive value is on.
		// A negative one is a hanging indent, which is a different thing and
		// is left alone rather than turned into its opposite.
		if first, err := strconv.Atoi(indent.attr("w:firstLine")); err == nil && first > 0 {
			paragraph.SetAttr("firstLine", true)
		}
	}

	spacing := properties.child("w", "spacing")
	if spacing == nil {
		return
	}
	// w:line is a multiple of 240 when the rule is "auto"; at "exact" or
	// "atLeast" it is a fixed height in twips, which muni has no way to
	// express and would misread as an enormous multiple.
	if rule := spacing.attr("w:lineRule"); rule != "" && rule != "auto" {
		return
	}
	line, err := strconv.Atoi(spacing.attr("w:line"))
	if err != nil || line <= 0 {
		return
	}
	height := float64(line) / 240
	if height < 0.5 || height > 5 {
		return
	}
	// 1.05 is what Word writes for single spacing in some templates, and
	// showing "1.05" in the line-height box for a document nobody deliberately
	// spaced is noise. The editor's own values survive exactly.
	rounded := math.Round(height*100) / 100
	if rounded == 1 || rounded == 1.05 {
		return
	}
	paragraph.SetAttr("lineHeight", strconv.FormatFloat(rounded, 'f', -1, 64))
}

// tableLeadsWithAHeader reports whether a table's first row is its header.
//
// Word almost never says so on the row. w:tblHeader means "repeat this row at
// the top of every page", which is a printing instruction that most tables
// leave off; what a header row actually is, is the table style's firstRow
// formatting switched on by w:tblLook. Reading the cells finds nothing either,
// because the bold and the shading are in the style rather than on them.
//
// Both halves are needed. w:tblLook says to apply the style's first-row
// formatting, and Word writes it on nearly every table — including ones using
// Table Grid, which draws every row alike. Marking a header there would invent
// one the document does not have.
func (imp *importer) tableLeadsWithAHeader(properties *xnode) bool {
	if properties == nil {
		return false
	}
	look := properties.child("w", "tblLook")
	if look == nil || !looksAtFirstRow(look) {
		return false
	}
	styleID := properties.child("w", "tblStyle").val()
	for depth := 0; styleID != "" && depth < 16; depth++ {
		style, ok := imp.styles[styleID]
		if !ok {
			return false
		}
		if style.firstRowFormatting {
			return true
		}
		styleID = style.basedOn
	}
	return false
}

// looksAtFirstRow reads w:tblLook, which Word writes twice over: as named
// attributes, and as the hex bitmask older files carry.
func looksAtFirstRow(look *xnode) bool {
	switch strings.TrimSpace(look.attr("w:firstRow")) {
	case "1", "true", "on":
		return true
	case "0", "false", "off":
		return false
	}
	value, err := strconv.ParseUint(strings.TrimSpace(look.attr("w:val")), 16, 32)
	return err == nil && value&0x0020 != 0
}

// cellVerticalAlign reads where a cell's text sits between the top and the
// bottom of its row. Word writes "top", "center" or "bottom", and a cell that
// says nothing means the top.
func cellVerticalAlign(properties *xnode) string {
	switch strings.ToLower(strings.TrimSpace(properties.child("w", "vAlign").val())) {
	case "center":
		return "middle"
	case "bottom":
		return "bottom"
	default:
		return "top"
	}
}
