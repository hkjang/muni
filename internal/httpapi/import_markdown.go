package httpapi

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode"

	"github.com/hkjang/muni/internal/richdoc"
)

// markdownDocument parses CommonMark with the GitHub table, task-list and
// strikethrough extensions, plus the small set of inline HTML tags muni's own
// Markdown export emits, so an exported file imports back unchanged.
func markdownDocument(value string) (json.RawMessage, []richdoc.Asset, error) {
	context := &inlineContext{}
	blocks := parseMarkdownBlocks(splitLines(value), context, 0)
	document := richdoc.Doc(blocks...)
	content, err := document.JSON()
	if err != nil {
		return nil, nil, err
	}
	return content, context.assets, nil
}

func splitLines(value string) []string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.TrimPrefix(value, "\ufeff")
	return strings.Split(value, "\n")
}

const maxMarkdownDepth = 8

func parseMarkdownBlocks(lines []string, context *inlineContext, depth int) []*richdoc.Node {
	blocks := make([]*richdoc.Node, 0, len(lines)/2+1)
	index := 0
	for index < len(lines) {
		if strings.TrimSpace(lines[index]) == "" {
			index++
			continue
		}
		if node, consumed := parseFencedCode(lines[index:]); consumed > 0 {
			blocks = append(blocks, node)
			index += consumed
			continue
		}
		if level, text, ok := atxHeading(lines[index]); ok {
			heading := &richdoc.Node{Type: "heading", Content: context.parse(text)}
			heading.SetAttr("level", level)
			blocks = append(blocks, heading)
			index++
			continue
		}
		if isThematicBreak(lines[index]) {
			blocks = append(blocks, &richdoc.Node{Type: "horizontalRule"})
			index++
			continue
		}
		if depth < maxMarkdownDepth && isBlockquoteLine(lines[index]) {
			node, consumed := parseBlockquote(lines[index:], context, depth)
			blocks = append(blocks, node)
			index += consumed
			continue
		}
		if index+1 < len(lines) && strings.Contains(lines[index], "|") && isTableDelimiterRow(lines[index+1]) {
			if node, consumed := parseMarkdownTable(lines[index:], context); consumed > 0 {
				blocks = append(blocks, node)
				index += consumed
				continue
			}
		}
		if depth < maxMarkdownDepth {
			if _, ok := parseListMarker(lines[index]); ok {
				node, consumed := parseMarkdownList(lines[index:], context, depth)
				if consumed > 0 {
					blocks = append(blocks, node)
					index += consumed
					continue
				}
			}
		}
		if node, consumed := parseIndentedCode(lines[index:]); consumed > 0 {
			blocks = append(blocks, node)
			index += consumed
			continue
		}
		node, consumed := parseMarkdownParagraph(lines[index:], context)
		if consumed == 0 {
			index++
			continue
		}
		if node != nil {
			blocks = append(blocks, node)
		}
		index += consumed
	}
	return blocks
}

func parseFencedCode(lines []string) (*richdoc.Node, int) {
	fence, language, ok := codeFence(lines[0])
	if !ok {
		return nil, 0
	}
	body := make([]string, 0, 8)
	index := 1
	for index < len(lines) && !closesFence(lines[index], fence) {
		body = append(body, lines[index])
		index++
	}
	if index < len(lines) {
		index++
	}
	node := &richdoc.Node{Type: "codeBlock"}
	if language != "" {
		node.SetAttr("language", language)
	}
	if text := strings.Join(body, "\n"); text != "" {
		node.Content = []*richdoc.Node{richdoc.Text(text)}
	}
	return node, index
}

func codeFence(line string) (string, string, bool) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return "", "", false
	}
	for _, marker := range []string{"```", "~~~"} {
		if !strings.HasPrefix(trimmed, marker) {
			continue
		}
		rest := strings.TrimLeft(trimmed, string(marker[0]))
		language := strings.Fields(strings.TrimSpace(rest))
		if len(language) == 0 {
			return marker, "", true
		}
		return marker, sanitizeLanguage(language[0]), true
	}
	return "", "", false
}

func sanitizeLanguage(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if len(value) > 32 {
		return ""
	}
	for _, r := range value {
		if !(r == '-' || r == '+' || r == '#' || r == '.' || unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return ""
		}
	}
	return value
}

func closesFence(line, fence string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, fence) && strings.Trim(trimmed, string(fence[0])) == ""
}

func atxHeading(line string) (int, string, bool) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 || !strings.HasPrefix(trimmed, "#") {
		return 0, "", false
	}
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level > 6 {
		return 0, "", false
	}
	rest := trimmed[level:]
	if rest != "" && rest[0] != ' ' && rest[0] != '\t' {
		return 0, "", false
	}
	text := strings.TrimSpace(rest)
	// A closing run of hashes is decoration, not content.
	if stripped := strings.TrimRight(text, "#"); stripped != text {
		if stripped == "" || strings.HasSuffix(stripped, " ") {
			text = strings.TrimSpace(stripped)
		}
	}
	return level, text, true
}

func setextHeading(line string) (int, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return 0, false
	}
	if strings.Trim(trimmed, "=") == "" {
		return 1, true
	}
	if strings.Trim(trimmed, "-") == "" && len(trimmed) >= 2 {
		return 2, true
	}
	return 0, false
}

func isThematicBreak(line string) bool {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 3 {
		return false
	}
	for _, marker := range []string{"-", "*", "_"} {
		compact := strings.ReplaceAll(trimmed, " ", "")
		if strings.Trim(compact, marker) == "" && len(compact) >= 3 {
			return true
		}
	}
	return false
}

func isBlockquoteLine(line string) bool {
	trimmed := strings.TrimLeft(line, " ")
	return strings.HasPrefix(trimmed, ">")
}

func parseBlockquote(lines []string, context *inlineContext, depth int) (*richdoc.Node, int) {
	inner := make([]string, 0, 8)
	index := 0
	for index < len(lines) {
		line := lines[index]
		if isBlockquoteLine(line) {
			trimmed := strings.TrimPrefix(strings.TrimLeft(line, " "), ">")
			inner = append(inner, strings.TrimPrefix(trimmed, " "))
			index++
			continue
		}
		// A blank line, or any new block, ends the quote; other text is a lazy
		// continuation of the quoted paragraph.
		if strings.TrimSpace(line) == "" || startsNewBlock(line) {
			break
		}
		inner = append(inner, line)
		index++
	}
	blocks := parseMarkdownBlocks(inner, context, depth+1)
	if len(blocks) == 0 {
		blocks = []*richdoc.Node{richdoc.Paragraph()}
	}
	return &richdoc.Node{Type: "blockquote", Content: blocks}, index
}

func startsNewBlock(line string) bool {
	if _, _, ok := atxHeading(line); ok {
		return true
	}
	if _, _, ok := codeFence(line); ok {
		return true
	}
	if isThematicBreak(line) || isBlockquoteLine(line) {
		return true
	}
	_, ok := parseListMarker(line)
	return ok
}

func parseIndentedCode(lines []string) (*richdoc.Node, int) {
	if !strings.HasPrefix(lines[0], "    ") && !strings.HasPrefix(lines[0], "\t") {
		return nil, 0
	}
	body := make([]string, 0, 8)
	index := 0
	pendingBlanks := 0
	for index < len(lines) {
		line := lines[index]
		if strings.TrimSpace(line) == "" {
			pendingBlanks++
			index++
			continue
		}
		if !strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "\t") {
			break
		}
		for ; pendingBlanks > 0; pendingBlanks-- {
			body = append(body, "")
		}
		if strings.HasPrefix(line, "\t") {
			body = append(body, strings.TrimPrefix(line, "\t"))
		} else {
			body = append(body, line[4:])
		}
		index++
	}
	if len(body) == 0 {
		return nil, 0
	}
	node := &richdoc.Node{Type: "codeBlock", Content: []*richdoc.Node{richdoc.Text(strings.Join(body, "\n"))}}
	return node, index - pendingBlanks
}

func parseMarkdownParagraph(lines []string, context *inlineContext) (*richdoc.Node, int) {
	body := make([]string, 0, 4)
	index := 0
	for index < len(lines) {
		line := lines[index]
		if strings.TrimSpace(line) == "" {
			break
		}
		if len(body) > 0 {
			if level, ok := setextHeading(line); ok {
				heading := &richdoc.Node{Type: "heading", Content: context.parse(strings.Join(body, "\n"))}
				heading.SetAttr("level", level)
				return heading, index + 1
			}
			if startsNewBlock(line) {
				break
			}
			if index+1 < len(lines) && strings.Contains(line, "|") && isTableDelimiterRow(lines[index+1]) {
				break
			}
		}
		body = append(body, strings.TrimLeft(line, " "))
		index++
	}
	if len(body) == 0 {
		return nil, 0
	}
	inline := context.parse(strings.Join(body, "\n"))
	if len(inline) == 0 {
		return nil, index
	}
	return richdoc.Paragraph(inline...), index
}

type listMarker struct {
	indent        int
	contentIndent int
	ordered       bool
	start         int
	delimiter     byte
	content       string
}

func parseListMarker(line string) (listMarker, bool) {
	trimmed := strings.TrimLeft(line, " \t")
	indent := len(line) - len(trimmed)
	if indent > 8 || trimmed == "" {
		return listMarker{}, false
	}
	if isThematicBreak(line) {
		return listMarker{}, false
	}
	switch trimmed[0] {
	case '-', '*', '+':
		if len(trimmed) < 2 || (trimmed[1] != ' ' && trimmed[1] != '\t') {
			return listMarker{}, false
		}
		rest := strings.TrimLeft(trimmed[1:], " \t")
		return listMarker{
			indent:        indent,
			contentIndent: indent + len(trimmed) - len(rest),
			delimiter:     trimmed[0],
			content:       rest,
		}, true
	}
	digits := 0
	for digits < len(trimmed) && trimmed[digits] >= '0' && trimmed[digits] <= '9' {
		digits++
	}
	if digits == 0 || digits > 9 || digits >= len(trimmed) {
		return listMarker{}, false
	}
	if trimmed[digits] != '.' && trimmed[digits] != ')' {
		return listMarker{}, false
	}
	if digits+1 >= len(trimmed) || (trimmed[digits+1] != ' ' && trimmed[digits+1] != '\t') {
		return listMarker{}, false
	}
	number, err := strconv.Atoi(trimmed[:digits])
	if err != nil {
		return listMarker{}, false
	}
	rest := strings.TrimLeft(trimmed[digits+1:], " \t")
	return listMarker{
		indent:        indent,
		contentIndent: indent + len(trimmed) - len(rest),
		ordered:       true,
		start:         number,
		delimiter:     trimmed[digits],
		content:       rest,
	}, true
}

// parseMarkdownList collects one list, splitting it into items whose bodies are
// parsed recursively so nested lists, code blocks and paragraphs all survive.
func parseMarkdownList(lines []string, context *inlineContext, depth int) (*richdoc.Node, int) {
	first, ok := parseListMarker(lines[0])
	if !ok {
		return nil, 0
	}
	type rawItem struct {
		body    []string
		checked *bool
	}
	items := make([]rawItem, 0, 4)
	index := 0
	for index < len(lines) {
		marker, ok := parseListMarker(lines[index])
		if !ok || marker.indent > first.contentIndent || marker.ordered != first.ordered || marker.delimiter != first.delimiter {
			break
		}
		content := marker.content
		var checked *bool
		if state, rest, found := taskMarker(content); found {
			checked = &state
			content = rest
		}
		body := []string{content}
		index++
		for index < len(lines) {
			line := lines[index]
			if strings.TrimSpace(line) == "" {
				// A blank line only ends the item when the next line leaves it.
				if index+1 < len(lines) && indentOf(lines[index+1]) < marker.contentIndent && strings.TrimSpace(lines[index+1]) != "" {
					break
				}
				body = append(body, "")
				index++
				continue
			}
			if indentOf(line) >= marker.contentIndent {
				body = append(body, trimIndent(line, marker.contentIndent))
				index++
				continue
			}
			if _, isItem := parseListMarker(line); isItem {
				break
			}
			if startsNewBlock(line) {
				break
			}
			// Lazy continuation of the item's paragraph.
			body = append(body, strings.TrimLeft(line, " \t"))
			index++
		}
		items = append(items, rawItem{body: body, checked: checked})
	}
	if len(items) == 0 {
		return nil, 0
	}

	isTask := items[0].checked != nil
	listType, itemType := "bulletList", "listItem"
	switch {
	case isTask:
		listType, itemType = "taskList", "taskItem"
	case first.ordered:
		listType = "orderedList"
	}
	list := &richdoc.Node{Type: listType}
	if listType == "orderedList" {
		list.SetAttr("start", first.start)
	}
	for _, item := range items {
		blocks := parseMarkdownBlocks(item.body, context, depth+1)
		if len(blocks) == 0 {
			blocks = []*richdoc.Node{richdoc.Paragraph()}
		}
		node := &richdoc.Node{Type: itemType, Content: blocks}
		if isTask {
			state := false
			if item.checked != nil {
				state = *item.checked
			}
			node.SetAttr("checked", state)
		}
		list.Content = append(list.Content, node)
	}
	return list, index
}

func taskMarker(content string) (bool, string, bool) {
	if len(content) < 3 || content[0] != '[' || content[2] != ']' {
		return false, content, false
	}
	switch content[1] {
	case ' ':
		return false, strings.TrimLeft(content[3:], " \t"), true
	case 'x', 'X':
		return true, strings.TrimLeft(content[3:], " \t"), true
	}
	return false, content, false
}

func indentOf(line string) int {
	count := 0
	for _, r := range line {
		switch r {
		case ' ':
			count++
		case '\t':
			count += 4
		default:
			return count
		}
	}
	return count
}

func trimIndent(line string, width int) string {
	removed := 0
	index := 0
	for index < len(line) && removed < width {
		switch line[index] {
		case ' ':
			removed++
			index++
		case '\t':
			removed += 4
			index++
		default:
			return line[index:]
		}
	}
	return line[index:]
}

func isTableDelimiterRow(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || !strings.ContainsAny(trimmed, "-") {
		return false
	}
	for _, cell := range splitTableRow(trimmed) {
		cell = strings.TrimSpace(cell)
		if cell == "" {
			return false
		}
		body := strings.TrimSuffix(strings.TrimPrefix(cell, ":"), ":")
		if body == "" || strings.Trim(body, "-") != "" {
			return false
		}
	}
	return true
}

// splitTableRow splits on unescaped pipes and drops the optional leading and
// trailing pipe characters.
func splitTableRow(line string) []string {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "|")
	trimmed = strings.TrimSuffix(trimmed, "|")
	cells := make([]string, 0, 4)
	var current strings.Builder
	runes := []rune(trimmed)
	for index := 0; index < len(runes); index++ {
		if runes[index] == '\\' && index+1 < len(runes) && runes[index+1] == '|' {
			current.WriteRune('|')
			index++
			continue
		}
		if runes[index] == '|' {
			cells = append(cells, current.String())
			current.Reset()
			continue
		}
		current.WriteRune(runes[index])
	}
	cells = append(cells, current.String())
	return cells
}

func parseMarkdownTable(lines []string, context *inlineContext) (*richdoc.Node, int) {
	header := splitTableRow(lines[0])
	alignments := splitTableRow(lines[1])
	if len(header) < 1 || len(alignments) != len(header) {
		return nil, 0
	}
	align := make([]string, len(alignments))
	for index, cell := range alignments {
		cell = strings.TrimSpace(cell)
		left := strings.HasPrefix(cell, ":")
		right := strings.HasSuffix(cell, ":")
		switch {
		case left && right:
			align[index] = "center"
		case right:
			align[index] = "right"
		}
	}
	buildRow := func(cells []string, header bool) *richdoc.Node {
		row := &richdoc.Node{Type: "tableRow"}
		for index := 0; index < len(align); index++ {
			text := ""
			if index < len(cells) {
				text = strings.TrimSpace(cells[index])
			}
			cellType := "tableCell"
			if header {
				cellType = "tableHeader"
			}
			paragraph := richdoc.Paragraph(context.parse(text)...)
			if align[index] != "" {
				paragraph.SetAttr("textAlign", align[index])
			}
			cell := &richdoc.Node{Type: cellType, Content: []*richdoc.Node{paragraph}}
			cell.SetAttr("colspan", 1)
			cell.SetAttr("rowspan", 1)
			row.Content = append(row.Content, cell)
		}
		return row
	}
	table := &richdoc.Node{Type: "table", Content: []*richdoc.Node{buildRow(header, true)}}
	index := 2
	for index < len(lines) {
		line := lines[index]
		if strings.TrimSpace(line) == "" || !strings.Contains(line, "|") {
			break
		}
		table.Content = append(table.Content, buildRow(splitTableRow(line), false))
		index++
	}
	return table, index
}

// plainTextDocument keeps blank-line separated blocks as paragraphs and turns
// single line breaks inside a block into hard breaks.
func plainTextDocument(value string) (json.RawMessage, error) {
	blocks := make([]*richdoc.Node, 0, 8)
	current := make([]string, 0, 4)
	flush := func() {
		if len(current) == 0 {
			return
		}
		inline := make([]*richdoc.Node, 0, len(current)*2)
		for index, line := range current {
			if index > 0 {
				inline = append(inline, &richdoc.Node{Type: "hardBreak"})
			}
			if line != "" {
				inline = append(inline, richdoc.Text(line))
			}
		}
		blocks = append(blocks, richdoc.Paragraph(inline...))
		current = current[:0]
	}
	for _, line := range splitLines(value) {
		line = strings.TrimRight(line, " \t")
		if line == "" {
			flush()
			continue
		}
		current = append(current, line)
	}
	flush()
	document := richdoc.Doc(blocks...)
	return document.JSON()
}
