package pdfx

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/hkjang/muni/internal/richdoc"
)

type textLine struct {
	y      float64
	left   float64
	right  float64
	size   float64
	bold   bool
	italic bool
	mono   bool
	text   string
	items  []textItem
}

type flowKind int

const (
	flowText flowKind = iota
	flowImage
	flowTable
)

type flowElement struct {
	kind     flowKind
	y        float64
	line     textLine
	image    imageItem
	bulleted int  // >=1 when an indented run lost its bullet glyph while printing
	mono     bool // fixed-pitch line, rendered back as a code block
	table    tableSpan
	lines    []textLine
}

var (
	bulletPattern  = regexp.MustCompile(`^([\x{2022}\x{00b7}\x{25aa}\x{2023}\x{25e6}\x{25cf}\x{25cb}\x{25a0}\x{25a1}\x{203b}\x{2013}\x{2014}\-\*\x{f0b7}])[ \t]+(.*)$`)
	orderedPattern = regexp.MustCompile(`^(?:\(([0-9]{1,3})\)|([0-9]{1,3})[.)]|([a-zA-Z])[.)]|([\x{ac00}-\x{d7a3}])[.)])[ \t]+(.*)$`)
	checkboxLine   = regexp.MustCompile(`^\[( |x|X)\][ \t]+(.*)$`)
)

// buildLines clusters positioned glyph runs into visual lines.
func buildLines(items []textItem) []textLine {
	if len(items) == 0 {
		return nil
	}
	sorted := make([]textItem, len(items))
	copy(sorted, items)
	sort.SliceStable(sorted, func(a, b int) bool {
		if math.Abs(sorted[a].y-sorted[b].y) > 0.8 {
			return sorted[a].y > sorted[b].y
		}
		return sorted[a].x < sorted[b].x
	})

	lines := make([]textLine, 0, len(sorted)/2+1)
	group := []textItem{sorted[0]}
	flush := func() {
		if len(group) == 0 {
			return
		}
		if line, ok := assembleLine(group); ok {
			lines = append(lines, line)
		}
		group = nil
	}
	for _, item := range sorted[1:] {
		reference := group[len(group)-1]
		tolerance := math.Max(reference.size, item.size) * 0.45
		if tolerance < 1.5 {
			tolerance = 1.5
		}
		if math.Abs(item.y-reference.y) > tolerance {
			flush()
		}
		group = append(group, item)
	}
	flush()
	return lines
}

func assembleLine(items []textItem) (textLine, bool) {
	sort.SliceStable(items, func(a, b int) bool { return items[a].x < items[b].x })
	var builder strings.Builder
	line := textLine{y: items[0].y, left: items[0].x, right: items[0].endX, bold: true, mono: true}
	weight := 0.0
	sizeTotal := 0.0
	previousEnd := items[0].x
	for index, item := range items {
		if index > 0 {
			gap := item.x - previousEnd
			reference := math.Max(item.size, 1)
			existing := builder.String()
			// A real inter-word space is roughly a quarter em wide in every
			// script; anything smaller is kerning between adjacent glyphs.
			if gap > reference*0.22 && !strings.HasSuffix(existing, " ") && !strings.HasPrefix(item.text, " ") {
				builder.WriteString(" ")
			}
		}
		builder.WriteString(item.text)
		previousEnd = item.endX
		if item.endX > line.right {
			line.right = item.endX
		}
		if item.x < line.left {
			line.left = item.x
		}
		length := float64(len([]rune(item.text)))
		sizeTotal += item.size * length
		weight += length
		if !item.bold {
			line.bold = false
		}
		if !item.mono {
			line.mono = false
		}
		if item.italic {
			line.italic = true
		}
	}
	if weight > 0 {
		line.size = sizeTotal / weight
	}
	// Indentation is carried by the line's own left edge, not by spaces in
	// front of its text.
	line.text = strings.Trim(builder.String(), " ")
	if strings.TrimSpace(line.text) == "" {
		return textLine{}, false
	}
	line.items = append(line.items, items...)
	return line, true
}

func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hangul, r) ||
		unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r)
}

func isCJKBoundary(left, right string) bool {
	leftRunes := []rune(left)
	rightRunes := []rune(right)
	if len(leftRunes) == 0 || len(rightRunes) == 0 {
		return false
	}
	return isCJK(leftRunes[len(leftRunes)-1]) || isCJK(rightRunes[0])
}

// bodySize is the most common text size, weighted by how much text uses it.
func bodySize(lines []textLine) float64 {
	buckets := map[int]float64{}
	for _, line := range lines {
		buckets[int(math.Round(line.size*2))] += float64(len([]rune(line.text)))
	}
	best, bestWeight := 0, 0.0
	for size, weight := range buckets {
		if weight > bestWeight || (weight == bestWeight && size < best) {
			best, bestWeight = size, weight
		}
	}
	if best == 0 {
		return 11
	}
	return float64(best) / 2
}

type documentBuilder struct {
	body      float64
	pages     int
	assets    []richdoc.Asset
	assetByID map[string]string
	blocks    []*richdoc.Node
	headings  []float64
	pending   *paragraphAccumulator
	listItems []listEntry
	listKind  string
	listLefts []float64
	code      []string
}

type paragraphAccumulator struct {
	text  string
	bold  bool
	right float64
	size  float64
}

type listEntry struct {
	text    string
	level   int
	checked *bool
}

// Build converts every page's flow into the final document tree.
func buildDocument(pages []*pageContent) (*richdoc.Node, []richdoc.Asset) {
	dropPageFurniture(pages)
	// The running head and the page number belong to the paper, not to the
	// document, and they have to be recognised across the whole of it: the
	// evidence that a line is furniture is that the same line is on the next
	// page too. So every page is read before any of it is turned into blocks.
	pageLines := make([][]textLine, len(pages))
	for index, page := range pages {
		pageLines[index] = buildLines(page.texts)
	}
	dropRunningFurniture(pageLines, pages)

	allLines := make([]textLine, 0, 256)
	perPage := make([][]flowElement, 0, len(pages))
	pageRights := make([]float64, 0, len(pages))
	for pageIndex, page := range pages {
		lines := pageLines[pageIndex]
		allLines = append(allLines, lines...)
		pageRight := 0.0
		for _, line := range lines {
			if line.right > pageRight {
				pageRight = line.right
			}
		}
		tables := detectTables(lines, bodySize(lines))
		inTable := make([]int, len(lines))
		for index := range inTable {
			inTable[index] = -1
		}
		for spanIndex, span := range tables {
			for cursor := span.start; cursor < span.end; cursor++ {
				inTable[cursor] = spanIndex
			}
		}
		// Indented runs are looked for among the lines that are still prose.
		// A table's rows are indented too, and counting them turned the line
		// above a table into a one-item list.
		free := make([]textLine, 0, len(lines))
		freeIndex := make([]int, 0, len(lines))
		for index, line := range lines {
			if inTable[index] < 0 {
				free = append(free, line)
				freeIndex = append(freeIndex, index)
			}
		}
		bulleted := make([]int, len(lines))
		for position, depth := range detectIndentedRuns(free) {
			bulleted[freeIndex[position]] = depth
		}
		flow := make([]flowElement, 0, len(lines)+len(page.images))
		for index, line := range lines {
			if spanIndex := inTable[index]; spanIndex >= 0 {
				if tables[spanIndex].start == index {
					flow = append(flow, flowElement{kind: flowTable, y: line.y, table: tables[spanIndex], lines: lines})
				}
				continue
			}
			flow = append(flow, flowElement{kind: flowText, y: line.y, line: line, bulleted: bulleted[index], mono: line.mono})
		}
		for _, picture := range page.images {
			flow = append(flow, flowElement{kind: flowImage, y: picture.y + picture.height, image: picture})
		}
		sort.SliceStable(flow, func(a, b int) bool { return flow[a].y > flow[b].y })
		perPage = append(perPage, flow)
		pageRights = append(pageRights, pageRight)
		_ = page
	}

	builder := &documentBuilder{body: bodySize(allLines), pages: len(pages)}
	builder.headings = headingSizes(allLines, builder.body)

	// A table cut in two by a page break is one table: it runs to the foot of
	// one page and starts again at the head of the next, on the same columns.
	var running *richdoc.Node
	var runningColumns []columnBand
	carriedOver := false
	for pageIndex, flow := range perPage {
		var previous *textLine
		pageRight := pageRights[pageIndex]
		for index, element := range flow {
			if element.kind == flowImage {
				builder.flush()
				builder.addImage(element.image)
				previous, running = nil, nil
				continue
			}
			if element.kind == flowTable {
				builder.flush()
				table := element.table.node(element.lines, builder.body)
				// Only the table that opens a page can continue the one that
				// closed the page before it.
				if index == 0 && carriedOver && running != nil &&
					sameColumns(runningColumns, element.table.columns) {
					joinTables(running, table)
				} else {
					builder.blocks = append(builder.blocks, table)
					running, runningColumns = table, element.table.columns
				}
				previous = nil
				continue
			}
			line := element.line
			if element.mono {
				builder.addCodeLine(line.text)
				previous = nil
				continue
			}
			builder.flushCode()
			gap := 0.0
			if previous != nil {
				gap = previous.y - line.y
			}
			builder.addLine(line, previous, gap, pageRight, element.bulleted)
			copied := line
			previous = &copied
			running = nil
			_ = index
		}
		builder.flush()
		carriedOver = len(flow) > 0 && flow[len(flow)-1].kind == flowTable && running != nil
		_ = pageIndex
	}
	builder.flush()

	document := richdoc.Doc(builder.blocks...)
	if len(document.Content) == 0 {
		document.Content = []*richdoc.Node{richdoc.Paragraph()}
	}
	return document, builder.assets
}

// sameColumns reports whether two tables stand on the same columns.
func sameColumns(left, right []columnBand) bool {
	if len(left) == 0 || len(left) != len(right) {
		return false
	}
	for index := range left {
		a, b := left[index], right[index]
		if a.left > b.right || b.left > a.right {
			return false
		}
	}
	return true
}

// joinTables adds the rows of a table's continuation to the table it
// continues, leaving out a heading row repeated at the top of the new page.
func joinTables(table, continuation *richdoc.Node) {
	heading := ""
	if len(table.Content) > 0 {
		heading = strings.Join(strings.Fields(table.Content[0].PlainText()), " ")
	}
	for index, row := range continuation.Content {
		if index == 0 && heading != "" &&
			strings.Join(strings.Fields(row.PlainText()), " ") == heading {
			continue
		}
		// Only the table's own first row is a heading; a row that merely
		// starts a page is not.
		for _, cell := range row.Content {
			if cell != nil && cell.Type == "tableHeader" {
				cell.Type = "tableCell"
			}
		}
		table.Content = append(table.Content, row)
	}
}

// dropRunningFurniture removes the running head, the running foot and the page
// number: the lines that belong to the paper rather than to the document.
//
// Three things have to hold before a line is thrown away. It sits in the top
// or bottom margin; it stands apart from the block of text, which is what
// makes it a margin note rather than the first line of the page; and it looks
// like furniture — the same line is on another page, or it is a page number,
// or it is a web address, or it is set smaller than the body. A browser
// printing a page writes the date and the page's title across the top and the
// address and "1/2" across the foot, and every one of those arrived in the
// text until now.
func dropRunningFurniture(pageLines [][]textLine, pages []*pageContent) {
	if len(pageLines) == 0 {
		return
	}
	all := make([]textLine, 0, 256)
	for _, lines := range pageLines {
		all = append(all, lines...)
	}
	body := bodySize(all)

	type candidate struct {
		page int
		row  int
	}
	repeats := map[string][]candidate{}
	for pageIndex, lines := range pageLines {
		if pageIndex >= len(pages) || pages[pageIndex].height <= 0 {
			continue
		}
		height := pages[pageIndex].height
		for row, line := range lines {
			if !inMargin(line, height) || !standsApart(lines, row, body) {
				continue
			}
			key := marginOf(line, height) + "\x00" + furnitureKey(line.text)
			repeats[key] = append(repeats[key], candidate{page: pageIndex, row: row})
		}
	}

	drop := make([]map[int]bool, len(pageLines))
	for index := range drop {
		drop[index] = map[int]bool{}
	}
	for _, found := range repeats {
		onPages := map[int]bool{}
		for _, item := range found {
			onPages[item.page] = true
		}
		repeated := len(onPages) >= 2 && len(onPages)*2 >= len(pageLines)
		for _, item := range found {
			line := pageLines[item.page][item.row]
			text := strings.TrimSpace(line.text)
			if repeated || isPageNumber(text) || holdsAddress(text) || line.size <= body*0.85 {
				drop[item.page][item.row] = true
			}
		}
	}

	for pageIndex, lines := range pageLines {
		if len(drop[pageIndex]) == 0 {
			continue
		}
		kept := make([]textLine, 0, len(lines))
		for row, line := range lines {
			if drop[pageIndex][row] {
				continue
			}
			kept = append(kept, line)
		}
		pageLines[pageIndex] = kept
	}
}

func inMargin(line textLine, height float64) bool {
	return line.y > height*0.93 || line.y < height*0.07
}

func marginOf(line textLine, height float64) string {
	if line.y > height*0.93 {
		return "top"
	}
	return "bottom"
}

// standsApart reports whether a line is separated from the rest of the page.
// The first line of a page sits a line's height above the second; a running
// head sits in the margin, far above everything.
func standsApart(lines []textLine, row int, body float64) bool {
	gap := math.MaxFloat64
	if row > 0 {
		gap = math.Min(gap, lines[row-1].y-lines[row].y)
	}
	if row+1 < len(lines) {
		gap = math.Min(gap, lines[row].y-lines[row+1].y)
	}
	return gap > math.Max(body, 1)*1.8
}

// furnitureKey is a line with its numbers taken out, so that "1/2" and "2/2"
// — the same footer on two pages — are recognised as the same line.
func furnitureKey(text string) string {
	var out strings.Builder
	digits := false
	for _, r := range strings.TrimSpace(text) {
		if r >= '0' && r <= '9' {
			if !digits {
				out.WriteByte('#')
				digits = true
			}
			continue
		}
		digits = false
		out.WriteRune(r)
	}
	return out.String()
}

// holdsAddress reports whether a line carries a web or file address, which is
// what a browser prints across the foot of the page.
func holdsAddress(text string) bool {
	lower := strings.ToLower(text)
	for _, prefix := range []string{"http://", "https://", "file:///", "www."} {
		if strings.Contains(lower, prefix) {
			return true
		}
	}
	return false
}

func isPageNumber(value string) bool {
	if value == "" {
		return true
	}
	if len([]rune(value)) > 12 {
		return false
	}
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, value)
	if digits == "" {
		return false
	}
	rest := strings.TrimSpace(strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return -1
		}
		return r
	}, value))
	switch strings.ToLower(rest) {
	case "", "-", "--", "- -", "/", "p", "p.", "page", "쪽", "페이지":
		return true
	}
	return false
}

func headingSizes(lines []textLine, body float64) []float64 {
	unique := map[int]bool{}
	for _, line := range lines {
		if line.size > body*1.12 && len([]rune(strings.TrimSpace(line.text))) <= 120 {
			unique[int(math.Round(line.size*2))] = true
		}
	}
	sizes := make([]float64, 0, len(unique))
	for size := range unique {
		sizes = append(sizes, float64(size)/2)
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(sizes)))
	if len(sizes) > 5 {
		sizes = sizes[:5]
	}
	return sizes
}

func (b *documentBuilder) headingLevel(line textLine) int {
	for index, size := range b.headings {
		if math.Abs(line.size-size) < 0.4 {
			return index + 1
		}
	}
	if line.size > b.body*1.12 {
		return len(b.headings) + 1
	}
	return 0
}

func (b *documentBuilder) addLine(line textLine, previous *textLine, gap, pageRight float64, bulleted int) {
	text := strings.TrimSpace(line.text)
	if text == "" {
		return
	}

	if checked, rest, ok := checkboxPrefix(text); ok {
		b.flushParagraph()
		b.appendListItem("task", listEntry{text: rest, level: b.levelFor(line.left), checked: &checked})
		return
	}
	if match := bulletPattern.FindStringSubmatch(text); match != nil {
		b.flushParagraph()
		b.appendListItem("bullet", listEntry{text: strings.TrimSpace(match[2]), level: b.levelFor(line.left)})
		return
	}
	if match := orderedPattern.FindStringSubmatch(text); match != nil && b.headingLevel(line) == 0 {
		rest := match[len(match)-1]
		b.flushParagraph()
		b.appendListItem("ordered", listEntry{text: strings.TrimSpace(rest), level: b.levelFor(line.left)})
		return
	}

	if bulleted > 0 && b.headingLevel(line) == 0 {
		b.flushParagraph()
		b.appendListItem("bullet", listEntry{text: text, level: bulleted - 1})
		return
	}

	if level := b.headingLevel(line); level > 0 && level <= 6 {
		b.flush()
		heading := &richdoc.Node{Type: "heading", Content: []*richdoc.Node{richdoc.Text(text)}}
		heading.SetAttr("level", level)
		b.blocks = append(b.blocks, heading)
		return
	}

	b.flushList()

	// Continuation of a wrapped paragraph: the previous line reached close to
	// the right margin and the vertical step is a normal line advance.
	if b.pending != nil && previous != nil {
		lineHeight := math.Max(line.size, b.body) * 1.75
		reachedMargin := b.pending.right >= pageRight-math.Max(b.body*2.5, 12)
		sameIndent := math.Abs(line.left-previousLeft(previous, line)) < b.body*2.5
		if gap > 0 && gap <= lineHeight && reachedMargin && sameIndent {
			b.pending.text = joinWrapped(b.pending.text, text)
			b.pending.right = line.right
			if !line.bold {
				b.pending.bold = false
			}
			return
		}
	}
	b.flushParagraph()
	b.pending = &paragraphAccumulator{text: text, bold: line.bold, right: line.right, size: line.size}
}

func previousLeft(previous *textLine, current textLine) float64 {
	if previous == nil {
		return current.left
	}
	return previous.left
}

func joinWrapped(left, right string) string {
	if left == "" {
		return right
	}
	leftRunes := []rune(left)
	rightRunes := []rune(right)
	last := leftRunes[len(leftRunes)-1]
	if last == '-' && len(rightRunes) > 0 && unicode.IsLower(rightRunes[0]) {
		return string(leftRunes[:len(leftRunes)-1]) + right
	}
	if isCJK(last) || (len(rightRunes) > 0 && isCJK(rightRunes[0])) {
		return left + right
	}
	return left + " " + right
}

func (b *documentBuilder) appendListItem(kind string, entry listEntry) {
	if b.listKind != "" && b.listKind != kind {
		b.flushList()
	}
	b.listKind = kind
	b.listItems = append(b.listItems, entry)
}

func (b *documentBuilder) flushParagraph() {
	if b.pending == nil {
		return
	}
	text := strings.TrimSpace(b.pending.text)
	if text != "" {
		marks := []richdoc.Mark{}
		if b.pending.bold {
			marks = append(marks, richdoc.Mark{Type: "bold"})
		}
		b.blocks = append(b.blocks, richdoc.Paragraph(richdoc.Text(text, marks...)))
	}
	b.pending = nil
}

func (b *documentBuilder) flushList() {
	if len(b.listItems) == 0 {
		b.listKind = ""
		b.listLefts = nil
		return
	}
	b.blocks = append(b.blocks, buildNestedList(b.listItems, b.listKind, 0))
	b.listItems = nil
	b.listKind = ""
	b.listLefts = nil
}

// buildNestedList turns a flat run of entries carrying indent depths back into
// nested list nodes.
func buildNestedList(entries []listEntry, kind string, depth int) *richdoc.Node {
	listType, itemType := "bulletList", "listItem"
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
	for index < len(entries) {
		entry := entries[index]
		item := &richdoc.Node{Type: itemType, Content: []*richdoc.Node{richdoc.Paragraph(richdoc.Text(entry.text))}}
		if entry.checked != nil {
			item.SetAttr("checked", *entry.checked)
		}
		index++
		nested := index
		for nested < len(entries) && entries[nested].level > entry.level {
			nested++
		}
		if nested > index && depth < 6 {
			item.Content = append(item.Content, buildNestedList(entries[index:nested], kind, depth+1))
			index = nested
		}
		list.Content = append(list.Content, item)
	}
	return list
}

// levelFor maps a line's left edge onto a list nesting depth by keeping a
// stack of the indents seen so far in the current list.
func (b *documentBuilder) levelFor(left float64) int {
	tolerance := b.body * 0.6
	if tolerance < 3 {
		tolerance = 3
	}
	for index := len(b.listLefts) - 1; index >= 0; index-- {
		if math.Abs(b.listLefts[index]-left) <= tolerance {
			b.listLefts = b.listLefts[:index+1]
			return index
		}
	}
	if len(b.listLefts) > 0 && left > b.listLefts[len(b.listLefts)-1]+tolerance && len(b.listLefts) < 7 {
		b.listLefts = append(b.listLefts, left)
		return len(b.listLefts) - 1
	}
	b.listLefts = []float64{left}
	return 0
}

func (b *documentBuilder) addCodeLine(text string) {
	b.flushParagraph()
	b.flushList()
	b.code = append(b.code, text)
}

func (b *documentBuilder) flushCode() {
	if len(b.code) == 0 {
		return
	}
	block := &richdoc.Node{Type: "codeBlock", Content: []*richdoc.Node{richdoc.Text(strings.Join(b.code, "\n"))}}
	b.blocks = append(b.blocks, block)
	b.code = nil
}

// checkboxPrefix recognises the check-box glyphs printed for task lists.
func checkboxPrefix(text string) (bool, string, bool) {
	if match := checkboxLine.FindStringSubmatch(text); match != nil {
		return match[1] != " ", strings.TrimSpace(match[2]), true
	}
	for _, prefix := range []string{"\u2612", "\u2611", "\u2705"} {
		if strings.HasPrefix(text, prefix) {
			return true, strings.TrimSpace(strings.TrimPrefix(text, prefix)), true
		}
	}
	for _, prefix := range []string{"\u2610", "\u25a1"} {
		if strings.HasPrefix(text, prefix) {
			return false, strings.TrimSpace(strings.TrimPrefix(text, prefix)), true
		}
	}
	return false, text, false
}

func (b *documentBuilder) flush() {
	b.flushCode()
	b.flushParagraph()
	b.flushList()
}

// addImage stores each distinct picture once. The same bytes drawn on several
// pages — a logo in a running header, a watermark — would otherwise become one
// attachment per page.
func (b *documentBuilder) addImage(picture imageItem) {
	if len(picture.data) == 0 {
		return
	}
	if b.assetByID == nil {
		b.assetByID = map[string]string{}
	}
	digest := sha256.Sum256(picture.data)
	key := hex.EncodeToString(digest[:])
	placeholder, seen := b.assetByID[key]
	if !seen {
		extension := ".png"
		if picture.mediaType == "image/jpeg" {
			extension = ".jpg"
		}
		placeholder = richdoc.Placeholder(len(b.assets) + 1)
		b.assets = append(b.assets, richdoc.Asset{
			Placeholder: placeholder,
			Name:        "pdf-image-" + strconv.Itoa(len(b.assets)+1) + extension,
			MediaType:   picture.mediaType,
			Data:        picture.data,
		})
		b.assetByID[key] = placeholder
	}
	node := &richdoc.Node{Type: "image"}
	node.SetAttr("src", placeholder)
	if picture.width > 0 {
		node.SetAttr("width", int(picture.width))
	}
	b.blocks = append(b.blocks, node)
}

// dropPageFurniture removes pictures that repeat across most pages. Those are
// letterhead logos and watermarks, not document content, and repeating them
// once per page would bury the text.
func dropPageFurniture(pages []*pageContent) {
	if len(pages) < 3 {
		return
	}
	appearances := map[string]int{}
	for _, page := range pages {
		seen := map[string]bool{}
		for _, picture := range page.images {
			digest := sha256.Sum256(picture.data)
			key := hex.EncodeToString(digest[:])
			if seen[key] {
				continue
			}
			seen[key] = true
			appearances[key]++
		}
	}
	threshold := len(pages)/2 + 1
	for _, page := range pages {
		kept := page.images[:0]
		for _, picture := range page.images {
			digest := sha256.Sum256(picture.data)
			if appearances[hex.EncodeToString(digest[:])] >= threshold {
				continue
			}
			kept = append(kept, picture)
		}
		page.images = kept
	}
}

// detectIndentedRuns finds groups of neighbouring lines indented deeper than
// the page's body margin and assigns each a nesting depth. Browsers print
// bullet glyphs as vector art, so this is the only trace a printed HTML list
// leaves behind.
func detectIndentedRuns(lines []textLine) []int {
	depths := make([]int, len(lines))
	if len(lines) < 2 {
		return depths
	}
	counts := map[int]int{}
	body := bodySize(lines)
	for _, line := range lines {
		counts[int(math.Round(line.left))]++
	}
	dominant, best := 0, 0
	for left, count := range counts {
		if count > best || (count == best && left < dominant) {
			dominant, best = left, count
		}
	}
	right := 0.0
	for _, line := range lines {
		if line.right > right {
			right = line.right
		}
	}
	indented := func(line textLine) bool {
		return !line.mono && line.size <= body*1.1 && line.left >= float64(dominant)+body*0.9
	}
	for index := 0; index < len(lines); {
		if !indented(lines[index]) {
			index++
			continue
		}
		end := index + 1
		for end < len(lines) && indented(lines[end]) && lines[end-1].y-lines[end].y < body*3 {
			end++
		}
		// Wrapped body text is indented too; require every line to stop short
		// of the right margin so paragraphs are not mistaken for list items.
		if end-index >= 2 {
			wrapped := false
			for cursor := index; cursor < end-1; cursor++ {
				if lines[cursor].right >= right-body*2 {
					wrapped = true
					break
				}
			}
			if !wrapped {
				levels := make([]float64, 0, 4)
				for cursor := index; cursor < end; cursor++ {
					left := lines[cursor].left
					depth := -1
					for level, value := range levels {
						if math.Abs(value-left) <= body*0.6 {
							depth = level
							break
						}
					}
					if depth < 0 {
						levels = append(levels, left)
						sort.Float64s(levels)
						for level, value := range levels {
							if math.Abs(value-left) <= 0.01 {
								depth = level
								break
							}
						}
					}
					if depth > 5 {
						depth = 5
					}
					depths[cursor] = depth + 1
				}
			}
		}
		index = end
	}
	return depths
}
