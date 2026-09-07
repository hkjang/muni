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
	columns []columnBand
	rows    [][]lineCell
}

// columnBand is the horizontal strip of the page one column occupies.
type columnBand struct {
	left  float64
	right float64
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
	claimed := 0
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
			span.columns = columnBands(segmented[index:end], body)
			if len(span.columns) >= 2 {
				// A heading row is centred over columns whose text is set to
				// the left and the right, so it overlaps neither and is left
				// behind as a stray paragraph above its own table. A line
				// that splits into exactly as many cells as the table has
				// columns, right above it, is that heading.
				//
				// The columns stay as the aligned rows drew them: a centred
				// heading would otherwise add a column of its own.
				if index > claimed && fills(segmented[index-1], len(span.columns)) &&
					lines[index-1].y-lines[index].y < math.Max(lines[index].size, body)*4.5 {
					span.start = index - 1
				}
				for cursor := span.start; cursor < end; cursor++ {
					span.rows = append(span.rows, segmented[cursor])
				}
				spans = append(spans, span)
				claimed = end
				index = end
				continue
			}
		}
		index++
	}
	return spans
}

// columnsAlign reports whether two rows are laid out on the same columns.
//
// Two cells are in the same column when the strips they occupy overlap. The
// left edges alone will not do: a heading centred over left-aligned text, or
// over figures set to the right, starts nowhere near them, and matching on
// the edge invents a column for every alignment in the table.
// fills reports whether a row splits into exactly as many cells as the table
// has columns.
func fills(cells []lineCell, columns int) bool {
	return len(cells) == columns && columns >= 2
}

func columnsAlign(left, right []lineCell, body float64) bool {
	matches := 0
	for _, cell := range right {
		for _, reference := range left {
			if overlaps(cell, reference, body) {
				matches++
				break
			}
		}
	}
	return matches >= 2
}

func overlaps(a, b lineCell, body float64) bool {
	padding := columnPadding(body)
	return a.left-padding < b.right && b.left-padding < a.right
}

func columnPadding(body float64) float64 {
	return math.Max(body*0.3, 2)
}

// columnBands works out where the columns of a table are: every strip of the
// page that a cell sits in, with overlapping strips merged.
//
// The grid is taken from the rows that fill it — the ones with the most cells
// — so that a title spanning the whole width does not swallow every column
// into one.
func columnBands(rows [][]lineCell, body float64) []columnBand {
	widest := 0
	for _, row := range rows {
		if len(row) > widest {
			widest = len(row)
		}
	}
	if widest < 2 {
		return nil
	}
	padding := columnPadding(body)
	spans := make([]columnBand, 0, widest*len(rows))
	for _, row := range rows {
		if len(row) < widest {
			continue
		}
		for _, cell := range row {
			spans = append(spans, columnBand{left: cell.left - padding, right: cell.right + padding})
		}
	}
	sort.Slice(spans, func(a, b int) bool { return spans[a].left < spans[b].left })
	bands := make([]columnBand, 0, widest)
	for _, span := range spans {
		if len(bands) > 0 && span.left <= bands[len(bands)-1].right {
			if span.right > bands[len(bands)-1].right {
				bands[len(bands)-1].right = span.right
			}
			continue
		}
		bands = append(bands, span)
	}
	if len(bands) < 2 || len(bands) > 24 {
		return nil
	}
	return bands
}

// bandFor places a cell in the column it belongs to: the one its middle sits
// in, the one it shares most of its width with, or the nearest one.
func bandFor(bands []columnBand, cell lineCell) int {
	middle := (cell.left + cell.right) / 2
	for index, band := range bands {
		if middle >= band.left && middle <= band.right {
			return index
		}
	}
	best, bestOverlap := -1, 0.0
	for index, band := range bands {
		shared := math.Min(cell.right, band.right) - math.Max(cell.left, band.left)
		if shared > bestOverlap {
			best, bestOverlap = index, shared
		}
	}
	if best >= 0 {
		return best
	}
	// A heading centred over a narrow column touches none of them; the
	// column it is nearest to is the one it names.
	nearest, distance := -1, math.MaxFloat64
	for index, band := range bands {
		if away := math.Abs((band.left+band.right)/2 - middle); away < distance {
			nearest, distance = index, away
		}
	}
	return nearest
}

func (span tableSpan) node(lines []textLine, body float64) *richdoc.Node {
	columnCount := len(span.columns)
	table := &richdoc.Node{Type: "table"}
	headerRow := lines[span.start].bold
	for rowIndex, cells := range span.rows {
		row := &richdoc.Node{Type: "tableRow"}
		texts := make([]string, columnCount)
		for position, cell := range cells {
			// A row with a cell for every column needs no guessing: they are
			// the columns, in order.
			target := position
			if len(cells) != columnCount {
				target = bandFor(span.columns, cell)
			}
			if target < 0 || target >= columnCount {
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
