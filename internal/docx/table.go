package docx

import (
	"strings"

	"github.com/hkjang/muni/internal/richdoc"
)

type placedCell struct {
	node     *richdoc.Node
	column   int
	colSpan  int
	rowSpan  int
	vMerge   string // "", "restart", "continue"
	isHeader bool
}

// layout resolves the logical grid of a table, expanding rowspans into the
// continuation cells that WordprocessingML requires.
func layout(rows []*richdoc.Node) ([][]placedCell, int) {
	occupied := map[int]map[int]bool{}
	mark := func(row, column int) {
		if occupied[row] == nil {
			occupied[row] = map[int]bool{}
		}
		occupied[row][column] = true
	}
	taken := func(row, column int) bool { return occupied[row] != nil && occupied[row][column] }

	grid := make([][]placedCell, len(rows))
	columns := 0
	for rowIndex, row := range rows {
		if row == nil {
			continue
		}
		column := 0
		for _, cell := range row.Content {
			if cell == nil || (cell.Type != "tableCell" && cell.Type != "tableHeader") {
				continue
			}
			for taken(rowIndex, column) {
				column++
			}
			colSpan := cell.AttrInt("colspan", 1)
			if colSpan < 1 {
				colSpan = 1
			}
			rowSpan := cell.AttrInt("rowspan", 1)
			if rowSpan < 1 {
				rowSpan = 1
			}
			if rowIndex+rowSpan > len(rows) {
				rowSpan = len(rows) - rowIndex
			}
			placed := placedCell{node: cell, column: column, colSpan: colSpan, rowSpan: rowSpan, isHeader: cell.Type == "tableHeader"}
			if rowSpan > 1 {
				placed.vMerge = "restart"
			}
			grid[rowIndex] = append(grid[rowIndex], placed)
			for offsetRow := 0; offsetRow < rowSpan; offsetRow++ {
				for offsetColumn := 0; offsetColumn < colSpan; offsetColumn++ {
					mark(rowIndex+offsetRow, column+offsetColumn)
				}
			}
			for offsetRow := 1; offsetRow < rowSpan; offsetRow++ {
				grid[rowIndex+offsetRow] = append(grid[rowIndex+offsetRow], placedCell{
					node: nil, column: column, colSpan: colSpan, rowSpan: 1, vMerge: "continue", isHeader: placed.isHeader,
				})
			}
			column += colSpan
			if column > columns {
				columns = column
			}
		}
	}
	for index := range grid {
		sortByColumn(grid[index])
	}
	return grid, columns
}

func sortByColumn(cells []placedCell) {
	for i := 1; i < len(cells); i++ {
		for j := i; j > 0 && cells[j].column < cells[j-1].column; j-- {
			cells[j], cells[j-1] = cells[j-1], cells[j]
		}
	}
}

// columnWidths converts the editor's per-cell pixel widths into twips that add
// up to the printable page width, falling back to an even split.
func columnWidths(grid [][]placedCell, columns int) []int {
	if columns <= 0 {
		return nil
	}
	pixels := make([]float64, columns)
	for _, row := range grid {
		for _, cell := range row {
			if cell.node == nil {
				continue
			}
			values, _ := cell.node.Attr("colwidth").([]any)
			if len(values) == 0 {
				continue
			}
			for offset := 0; offset < cell.colSpan && offset < len(values); offset++ {
				index := cell.column + offset
				if index >= columns {
					break
				}
				width := 0.0
				switch typed := values[offset].(type) {
				case float64:
					width = typed
				case int:
					width = float64(typed)
				}
				if width > 0 && pixels[index] == 0 {
					pixels[index] = width
				}
			}
		}
	}
	total := 0.0
	missing := 0
	for _, value := range pixels {
		if value > 0 {
			total += value
		} else {
			missing++
		}
	}
	if total == 0 {
		width := contentWidthTwip / columns
		widths := make([]int, columns)
		for index := range widths {
			widths[index] = width
		}
		widths[columns-1] = contentWidthTwip - width*(columns-1)
		return widths
	}
	if missing > 0 {
		average := total / float64(columns-missing)
		for index, value := range pixels {
			if value == 0 {
				pixels[index] = average
				total += average
			}
		}
	}
	widths := make([]int, columns)
	assigned := 0
	for index, value := range pixels {
		widths[index] = int(value / total * float64(contentWidthTwip))
		if widths[index] < 240 {
			widths[index] = 240
		}
		assigned += widths[index]
	}
	// Absorb rounding drift into the last column so the grid stays exact.
	if difference := contentWidthTwip - assigned; difference != 0 && columns > 0 {
		if widths[columns-1]+difference >= 240 {
			widths[columns-1] += difference
		}
	}
	return widths
}

func (b *builder) table(node *richdoc.Node, ctx blockContext) {
	rows := make([]*richdoc.Node, 0, len(node.Content))
	for _, row := range node.Content {
		if row != nil && row.Type == "tableRow" {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return
	}
	grid, columns := layout(rows)
	if columns == 0 {
		return
	}
	widths := columnWidths(grid, columns)

	var out strings.Builder
	out.WriteString(`<w:tbl><w:tblPr><w:tblStyle w:val="TableGrid"/>` +
		`<w:tblW w:w="5000" w:type="pct"/>` +
		`<w:tblLayout w:type="fixed"/>` +
		`<w:tblLook w:val="04A0" w:firstRow="1" w:lastRow="0" w:firstColumn="1" w:lastColumn="0" w:noHBand="0" w:noVBand="1"/>` +
		`</w:tblPr><w:tblGrid>`)
	for _, width := range widths {
		out.WriteString(`<w:gridCol` + intAttr("w:w", width) + `/>`)
	}
	out.WriteString(`</w:tblGrid>`)
	b.body.WriteString(out.String())

	for rowIndex, row := range grid {
		headerRow := len(row) > 0
		for _, cell := range row {
			if !cell.isHeader {
				headerRow = false
			}
		}
		b.body.WriteString(`<w:tr>`)
		if headerRow && rowIndex == 0 {
			b.body.WriteString(`<w:trPr><w:cantSplit/><w:tblHeader/></w:trPr>`)
		}
		for _, cell := range row {
			b.tableCell(cell, widths, ctx)
		}
		b.body.WriteString(`</w:tr>`)
	}
	b.body.WriteString(`</w:tbl>`)
	// Word needs a paragraph between (or after) tables to stay well formed.
	b.body.WriteString(`<w:p><w:pPr><w:spacing w:before="0" w:after="120"/></w:pPr></w:p>`)
}

func (b *builder) tableCell(cell placedCell, widths []int, ctx blockContext) {
	width := 0
	for offset := 0; offset < cell.colSpan; offset++ {
		if index := cell.column + offset; index < len(widths) {
			width += widths[index]
		}
	}
	var properties strings.Builder
	properties.WriteString(`<w:tcW` + intAttr("w:w", width) + ` w:type="dxa"/>`)
	if cell.colSpan > 1 {
		properties.WriteString(`<w:gridSpan` + intAttr("w:val", cell.colSpan) + `/>`)
	}
	switch cell.vMerge {
	case "restart":
		properties.WriteString(`<w:vMerge w:val="restart"/>`)
	case "continue":
		properties.WriteString(`<w:vMerge/>`)
	}
	if cell.isHeader {
		properties.WriteString(`<w:shd w:val="clear" w:color="auto" w:fill="F3F4FA"/>`)
	}
	if cell.node != nil {
		if background := hexColor(cell.node.AttrString("backgroundColor")); background != "" {
			properties.WriteString(`<w:shd w:val="clear" w:color="auto"` + attr("w:fill", background) + `/>`)
		}
	}
	properties.WriteString(`<w:vAlign w:val="center"/>`)

	b.body.WriteString(`<w:tc><w:tcPr>` + properties.String() + `</w:tcPr>`)
	before := b.body.Len()
	if cell.node != nil {
		child := ctx
		child.inTable = true
		child.header = cell.isHeader
		child.list = nil
		child.style = ""
		child.indent = 0
		b.blocks(cell.node.Content, child)
	}
	if b.body.Len() == before {
		// Every table cell must contain at least one paragraph.
		b.body.WriteString(`<w:p/>`)
	}
	b.body.WriteString(`</w:tc>`)
}
