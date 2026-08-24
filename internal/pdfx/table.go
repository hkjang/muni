package pdfx

import (
	"math"
	"sort"
	"strings"

	"github.com/hkjang/muni/internal/richdoc"
)

type lineCell struct {
	left  float64
	right float64
	text  string
}

// segmentCells splits a visual line wherever the horizontal gap is far wider
// than a word space, which is what separates table columns.
func segmentCells(line textLine, body float64) []lineCell {
	if len(line.items) == 0 {
		return nil
	}
	items := make([]textItem, len(line.items))
	copy(items, line.items)
	sort.SliceStable(items, func(a, b int) bool { return items[a].x < items[b].x })
	threshold := math.Max(math.Max(line.size, body)*1.15, 7)

	cells := make([]lineCell, 0, 4)
	var builder strings.Builder
	current := lineCell{left: items[0].x, right: items[0].endX}
	previousEnd := items[0].x
	for index, item := range items {
		if index > 0 {
			gap := item.x - previousEnd
			if gap > threshold {
				current.text = strings.TrimSpace(builder.String())
				if current.text != "" {
					cells = append(cells, current)
				}
				builder.Reset()
				current = lineCell{left: item.x, right: item.endX}
			} else if gap > math.Max(item.size, 1)*0.22 && !strings.HasSuffix(builder.String(), " ") {
				builder.WriteString(" ")
			}
		}
		builder.WriteString(item.text)
		previousEnd = item.endX
		if item.endX > current.right {
			current.right = item.endX
		}
	}
	current.text = strings.TrimSpace(builder.String())
	if current.text != "" {
		cells = append(cells, current)
	}
	return cells
}

type tableSpan struct {
	start   int
	end     int // exclusive
	columns []float64
	rows    [][]lineCell
}

// detectTables finds runs of neighbouring lines whose cells line up in the
// same columns. Justified prose also has wide gaps, but they fall at arbitrary
// positions, so requiring two rows to share column offsets rules it out.
func detectTables(lines []textLine, body float64) []tableSpan {
	segmented := make([][]lineCell, len(lines))
	for index, line := range lines {
		if cells := segmentCells(line, body); len(cells) >= 2 {
			segmented[index] = cells
		}
	}

	spans := make([]tableSpan, 0, 2)
	for index := 0; index < len(lines); {
		if segmented[index] == nil {
			index++
			continue
		}
		// Track every column seen so far: a row that skips the first column
		// because of a vertical merge still belongs to the same table.
		known := append([]lineCell{}, segmented[index]...)
		end := index + 1
		for end < len(lines) && segmented[end] != nil &&
			lines[end-1].y-lines[end].y < math.Max(lines[end].size, body)*4.5 &&
			columnsAlign(known, segmented[end], body) {
			known = append(known, segmented[end]...)
			end++
		}
		if end-index >= 2 {
			span := tableSpan{start: index, end: end}
			span.columns = columnPositions(segmented[index:end], body)
			if len(span.columns) >= 2 {
				for cursor := index; cursor < end; cursor++ {
					span.rows = append(span.rows, segmented[cursor])
				}
				spans = append(spans, span)
				index = end
				continue
			}
		}
		index++
	}
	return spans
}

func columnsAlign(left, right []lineCell, body float64) bool {
	matches := 0
	for _, cell := range right {
		for _, reference := range left {
			if math.Abs(cell.left-reference.left) < math.Max(body*0.8, 6) {
				matches++
				break
			}
		}
	}
	return matches >= 2
}

func columnPositions(rows [][]lineCell, body float64) []float64 {
	positions := make([]float64, 0, 8)
	tolerance := math.Max(body*0.8, 6)
	for _, row := range rows {
		for _, cell := range row {
			found := false
			for index, position := range positions {
				if math.Abs(position-cell.left) < tolerance {
					positions[index] = math.Min(position, cell.left)
					found = true
					break
				}
			}
			if !found {
				positions = append(positions, cell.left)
			}
		}
	}
	sort.Float64s(positions)
	if len(positions) > 24 {
		return nil
	}
	return positions
}

func (span tableSpan) node(lines []textLine, body float64) *richdoc.Node {
	tolerance := math.Max(body*0.8, 6)
	columnCount := len(span.columns)
	table := &richdoc.Node{Type: "table"}
	headerRow := lines[span.start].bold
	for rowIndex, cells := range span.rows {
		row := &richdoc.Node{Type: "tableRow"}
		texts := make([]string, columnCount)
		for _, cell := range cells {
			target := 0
			bestDistance := math.MaxFloat64
			for index, position := range span.columns {
				if distance := math.Abs(position - cell.left); distance < bestDistance {
					target, bestDistance = index, distance
				}
			}
			if bestDistance > tolerance*3 {
				continue
			}
			if texts[target] != "" {
				texts[target] += " "
			}
			texts[target] += cell.text
		}
		cellType := "tableCell"
		if rowIndex == 0 && headerRow {
			cellType = "tableHeader"
		}
		for _, text := range texts {
			cell := &richdoc.Node{Type: cellType}
			cell.SetAttr("colspan", 1)
			cell.SetAttr("rowspan", 1)
			cell.Content = []*richdoc.Node{richdoc.Paragraph(richdoc.Text(text))}
			if text == "" {
				cell.Content = []*richdoc.Node{richdoc.Paragraph()}
			}
			row.Content = append(row.Content, cell)
		}
		table.Content = append(table.Content, row)
	}
	return table
}
