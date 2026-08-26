package docx

import (
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
	for _, node := range nodes {
		switch {
		case node.is("w", "p"):
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
			if fallback := node.child("mc", "Fallback"); fallback != nil {
				out = append(out, imp.blocks(fallback.Children)...)
			}
		}
	}
	return out
}

func (imp *importer) paragraph(node *xnode) []block {
	properties := node.child("w", "pPr")
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

	type pending struct {
		cell   *richdoc.Node
		column int
		span   int
	}
	rows := make([]*richdoc.Node, 0, 8)
	open := map[int]*pending{}
	for _, rowNode := range node.Children {
		if !rowNode.is("w", "tr") {
			continue
		}
		header := false
		if properties := rowNode.child("w", "trPr"); properties != nil {
			header = properties.child("w", "tblHeader") != nil
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
