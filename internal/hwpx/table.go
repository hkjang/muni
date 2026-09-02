package hwpx

import (
	"strconv"
	"strings"

	"github.com/hkjang/muni/internal/richdoc"
)

// table reads an <hp:tbl>.
//
// The rows are the <hp:tr> the file wrote, read in order. A cell also carries
// its own address, which a file could disagree with; nothing seen so far
// does, and trusting the order keeps a merged cell where its row put it.
func (imp *importer) table(current *node) *richdoc.Node {
	rows := []*richdoc.Node{}
	headerRows := headerRowCount(current)
	for _, child := range current.children {
		if !child.is("tr") {
			continue
		}
		row := &richdoc.Node{Type: "tableRow"}
		for _, cellNode := range child.children {
			if !cellNode.is("tc") {
				continue
			}
			row.Content = append(row.Content, imp.cell(cellNode, len(rows) < headerRows))
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

// headerRowCount reads how many rows repeat at the top of each page, which is
// the only thing in an HWPX table that says "these are the headings".
func headerRowCount(table *node) int {
	if value := strings.TrimSpace(table.attr("repeatHeader")); value != "" {
		if strings.EqualFold(value, "true") || value == "1" {
			return 1
		}
	}
	return 0
}

func (imp *importer) cell(current *node, header bool) *richdoc.Node {
	kind := "tableCell"
	if header || strings.EqualFold(strings.TrimSpace(current.attr("header")), "true") {
		kind = "tableHeader"
	}
	cell := &richdoc.Node{Type: kind}
	cell.SetAttr("colspan", spanOf(current, "colSpan"))
	cell.SetAttr("rowspan", spanOf(current, "rowSpan"))
	// Word's default is the top and so is Hangul's; muni records it either way
	// because its own export reads this attribute.
	cell.SetAttr("verticalAlign", cellVerticalAlign(current))
	if shade := imp.cellShade(current); shade != "" && kind != "tableHeader" {
		cell.SetAttr("backgroundColor", shade)
	}

	content := []*richdoc.Node{}
	for _, child := range current.children {
		if !child.is("subList") {
			continue
		}
		for _, block := range child.children {
			content = append(content, imp.block(block)...)
		}
	}
	if len(content) == 0 {
		content = []*richdoc.Node{richdoc.Paragraph()}
	}
	cell.Content = content
	return cell
}

// spanOf reads how far a merged cell reaches. The span lives on <hp:cellSpan>,
// and a cell that says nothing covers one.
func spanOf(cell *node, name string) int {
	span := cell.child("cellSpan")
	if span == nil {
		return 1
	}
	value, err := strconv.Atoi(strings.TrimSpace(span.attr(name)))
	if err != nil || value < 1 {
		return 1
	}
	if value > 64 {
		return 64
	}
	return value
}

func cellVerticalAlign(cell *node) string {
	// The alignment is on the cell's paragraph list, not the cell: that is
	// where Hangul writes it in every cell of every file of its own. The
	// cell itself is read second, for a writer that put it there.
	value := ""
	if list := cell.child("subList"); list != nil {
		value = list.attr("vertAlign")
	}
	if value == "" {
		value = cell.attr("vertAlign")
	}
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "CENTER":
		return "middle"
	case "BOTTOM":
		return "bottom"
	}
	return "top"
}

// cellShade reads a cell's fill, if it has one worth keeping. White and no
// fill are the absence of a shade rather than a shade.
func (imp *importer) cellShade(cell *node) string {
	fill := cell.descendant("fillBrush")
	if fill == nil {
		return ""
	}
	colour := normalizeColor(fill.descendant("winBrush").attr("faceColor"))
	if colour == "" || strings.EqualFold(colour, "#FFFFFF") {
		return ""
	}
	return strings.ToLower(colour)
}
