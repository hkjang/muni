package httpapi

import (
	"encoding/json"
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/hkjang/muni/internal/richdoc"
)

// renderHTML converts stored document JSON into HTML for the HTML and PDF
// exports. Every text value is escaped, and only vetted link and image
// sources survive, so the result is safe to hand to a browser.
func renderHTML(raw json.RawMessage) string {
	document, err := richdoc.Parse(raw)
	if err != nil {
		return ""
	}
	var out strings.Builder
	renderHTMLBlocks(&out, document.Content)
	return out.String()
}

func renderHTMLBlocks(out *strings.Builder, nodes []*richdoc.Node) {
	for _, node := range nodes {
		renderHTMLBlock(out, node)
	}
}

func renderHTMLBlock(out *strings.Builder, node *richdoc.Node) {
	if node == nil {
		return
	}
	switch node.Type {
	case "doc":
		renderHTMLBlocks(out, node.Content)
	case "paragraph":
		out.WriteString("<p" + blockIDAttribute(node) + styleAttribute(node) + ">")
		renderHTMLInline(out, node.Content)
		out.WriteString("</p>")
	case "heading":
		level := node.AttrInt("level", 1)
		if level < 1 || level > 6 {
			level = 1
		}
		fmt.Fprintf(out, "<h%d%s%s>", level, blockIDAttribute(node), styleAttribute(node))
		renderHTMLInline(out, node.Content)
		fmt.Fprintf(out, "</h%d>", level)
	case "bulletList":
		out.WriteString("<ul>")
		renderHTMLBlocks(out, node.Content)
		out.WriteString("</ul>")
	case "orderedList":
		start := node.AttrInt("start", 1)
		if start > 1 {
			out.WriteString(`<ol start="` + strconv.Itoa(start) + `">`)
		} else {
			out.WriteString("<ol>")
		}
		renderHTMLBlocks(out, node.Content)
		out.WriteString("</ol>")
	case "listItem":
		out.WriteString("<li" + blockIDAttribute(node) + ">")
		renderHTMLBlocks(out, node.Content)
		out.WriteString("</li>")
	case "taskList":
		out.WriteString(`<ul data-type="taskList">`)
		renderHTMLBlocks(out, node.Content)
		out.WriteString("</ul>")
	case "taskItem":
		glyph := "&#9744;"
		checked := ""
		if node.AttrBool("checked") {
			glyph = "&#9746;"
			checked = ` data-checked="true"`
		}
		out.WriteString(`<li` + blockIDAttribute(node) + checked + `><span aria-hidden="true">` + glyph + `</span><div>`)
		renderHTMLBlocks(out, node.Content)
		out.WriteString("</div></li>")
	case "blockquote":
		out.WriteString("<blockquote" + blockIDAttribute(node) + ">")
		renderHTMLBlocks(out, node.Content)
		out.WriteString("</blockquote>")
	case "codeBlock":
		language := node.AttrString("language")
		out.WriteString("<pre" + blockIDAttribute(node) + "><code")
		if language != "" && isSafeToken(language) {
			out.WriteString(` class="language-` + html.EscapeString(language) + `"`)
		}
		out.WriteString(">" + html.EscapeString(codeText(node)) + "</code></pre>")
	case "horizontalRule":
		out.WriteString("<hr" + blockIDAttribute(node) + ">")
	case "table":
		out.WriteString("<table" + blockIDAttribute(node) + ">")
		renderHTMLTableRows(out, node.Content)
		out.WriteString("</table>")
	case "tableRow":
		out.WriteString("<tr>")
		renderHTMLBlocks(out, node.Content)
		out.WriteString("</tr>")
	case "tableCell", "tableHeader":
		tag := "td"
		if node.Type == "tableHeader" {
			tag = "th"
		}
		out.WriteString("<" + tag + cellAttributes(node) + ">")
		renderHTMLBlocks(out, node.Content)
		out.WriteString("</" + tag + ">")
	case "image":
		out.WriteString(imageHTML(node))
	case "hardBreak":
		out.WriteString("<br>")
	case "text":
		renderHTMLInline(out, []*richdoc.Node{node})
	default:
		renderHTMLBlocks(out, node.Content)
	}
}

// renderHTMLTableRows groups the leading all-header rows into a thead so the
// browser repeats them at the top of every printed page.
func renderHTMLTableRows(out *strings.Builder, rows []*richdoc.Node) {
	head := 0
	for head < len(rows) && isHeaderRow(rows[head]) {
		head++
	}
	if head > 0 {
		out.WriteString("<thead>")
		renderHTMLBlocks(out, rows[:head])
		out.WriteString("</thead>")
	}
	if head < len(rows) {
		out.WriteString("<tbody>")
		renderHTMLBlocks(out, rows[head:])
		out.WriteString("</tbody>")
	}
}

func isHeaderRow(row *richdoc.Node) bool {
	if row == nil || row.Type != "tableRow" || len(row.Content) == 0 {
		return false
	}
	for _, cell := range row.Content {
		if cell == nil || cell.Type != "tableHeader" {
			return false
		}
	}
	return true
}

// blockIDAttribute carries a block's stable identity into the exported HTML so
// the same anchor survives an export/import round trip and can back a deep link.
func blockIDAttribute(node *richdoc.Node) string {
	value := node.AttrString(richdoc.BlockIDAttr)
	if !isBlockIDToken(value) {
		return ""
	}
	return ` data-block-id="` + value + `"`
}

// isBlockIDToken accepts the identifier shape muni writes and the ones other
// producers use, while keeping anything that would need escaping out of the
// attribute. isSafeToken is deliberately not reused: it guards code language
// classes and does not allow the underscore block ids carry.
func isBlockIDToken(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

func styleAttribute(node *richdoc.Node) string {
	switch strings.ToLower(node.AttrString("textAlign")) {
	case "center":
		return ` style="text-align:center"`
	case "right":
		return ` style="text-align:right"`
	case "justify":
		return ` style="text-align:justify"`
	default:
		return ""
	}
}

func cellAttributes(node *richdoc.Node) string {
	var out strings.Builder
	if colspan := node.AttrInt("colspan", 1); colspan > 1 {
		out.WriteString(` colspan="` + strconv.Itoa(colspan) + `"`)
	}
	if rowspan := node.AttrInt("rowspan", 1); rowspan > 1 {
		out.WriteString(` rowspan="` + strconv.Itoa(rowspan) + `"`)
	}
	if widths, ok := node.Attr("colwidth").([]any); ok && len(widths) > 0 {
		total := 0
		for _, value := range widths {
			if width, ok := value.(float64); ok {
				total += int(width)
			}
		}
		if total > 0 {
			out.WriteString(` style="width:` + strconv.Itoa(total) + `px"`)
		}
	}
	return out.String()
}

func imageHTML(node *richdoc.Node) string {
	src := node.AttrString("src")
	if !safeImageSource(src) {
		return ""
	}
	out := `<img` + blockIDAttribute(node) + ` src="` + html.EscapeString(src) + `" alt="` + html.EscapeString(node.AttrString("alt")) + `"`
	if width := node.AttrInt("width", 0); width > 0 {
		out += ` width="` + strconv.Itoa(width) + `"`
	}
	return out + ">"
}

func safeImageSource(src string) bool {
	return strings.HasPrefix(src, "data:image/") || strings.HasPrefix(src, "/api/v1/attachments/")
}

func isSafeToken(value string) bool {
	for _, r := range value {
		if !(r == '-' || r == '+' || r == '#' || r == '.' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			return false
		}
	}
	return value != "" && len(value) <= 32
}

func codeText(node *richdoc.Node) string {
	var out strings.Builder
	for _, child := range node.Content {
		switch child.Type {
		case "text":
			out.WriteString(child.Text)
		case "hardBreak":
			out.WriteString("\n")
		}
	}
	return out.String()
}

func renderHTMLInline(out *strings.Builder, nodes []*richdoc.Node) {
	for _, node := range nodes {
		if node == nil {
			continue
		}
		switch node.Type {
		case "hardBreak":
			out.WriteString("<br>")
		case "image":
			out.WriteString(imageHTML(node))
		case "text":
			out.WriteString(applyMarks(html.EscapeString(node.Text), node.Marks))
		default:
			renderHTMLBlock(out, node)
		}
	}
}

func applyMarks(value string, marks []richdoc.Mark) string {
	for _, mark := range marks {
		switch mark.Type {
		case "bold":
			value = "<strong>" + value + "</strong>"
		case "italic":
			value = "<em>" + value + "</em>"
		case "underline":
			value = "<u>" + value + "</u>"
		case "strike":
			value = "<s>" + value + "</s>"
		case "code":
			value = "<code>" + value + "</code>"
		case "superscript":
			value = "<sup>" + value + "</sup>"
		case "subscript":
			value = "<sub>" + value + "</sub>"
		case "highlight":
			if color := cssColor(mark.AttrString("color")); color != "" {
				value = `<mark style="background-color:` + color + `">` + value + `</mark>`
			} else {
				value = "<mark>" + value + "</mark>"
			}
		case "textStyle":
			style := ""
			if color := cssColor(mark.AttrString("color")); color != "" {
				style += "color:" + color + ";"
			}
			if family := mark.AttrString("fontFamily"); family != "" && isSafeStyleValue(family) {
				style += "font-family:" + html.EscapeString(family) + ";"
			}
			if size := mark.AttrString("fontSize"); size != "" && isSafeStyleValue(size) {
				style += "font-size:" + html.EscapeString(size) + ";"
			}
			if style != "" {
				value = `<span style="` + style + `">` + value + `</span>`
			}
		case "link":
			href := strings.TrimSpace(mark.AttrString("href"))
			lower := strings.ToLower(href)
			if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "mailto:") {
				value = `<a href="` + html.EscapeString(href) + `" rel="noopener noreferrer">` + value + `</a>`
			}
		}
	}
	return value
}

// cssColor only lets through simple colour literals so a mark cannot inject
// arbitrary declarations into the style attribute.
func cssColor(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 32 || !isSafeStyleValue(value) {
		return ""
	}
	return html.EscapeString(value)
}

func isSafeStyleValue(value string) bool {
	if strings.ContainsAny(value, ";{}()<>\\\"") {
		return false
	}
	return len(value) <= 64 && !strings.Contains(strings.ToLower(value), "url")
}

// renderMarkdown produces CommonMark with GitHub table and task-list
// extensions, preserving the document's nesting.
func renderMarkdown(title string, raw json.RawMessage) string {
	document, err := richdoc.Parse(raw)
	if err != nil {
		return ""
	}
	var out strings.Builder
	if strings.TrimSpace(title) != "" {
		out.WriteString("# " + escapeMarkdown(title) + "\n\n")
	}
	writer := &markdownWriter{out: &out}
	writer.blocks(document.Content, "")
	return strings.TrimRight(out.String(), "\n") + "\n"
}

type markdownWriter struct {
	out *strings.Builder
}

func (w *markdownWriter) blocks(nodes []*richdoc.Node, indent string) {
	for _, node := range nodes {
		w.block(node, indent)
	}
}

func (w *markdownWriter) block(node *richdoc.Node, indent string) {
	if node == nil {
		return
	}
	switch node.Type {
	case "doc":
		w.blocks(node.Content, indent)
	case "paragraph":
		if text := w.inline(node.Content); strings.TrimSpace(text) != "" {
			w.out.WriteString(indent + text + "\n\n")
		}
	case "heading":
		level := node.AttrInt("level", 1)
		if level < 1 || level > 6 {
			level = 1
		}
		w.out.WriteString(indent + strings.Repeat("#", level) + " " + w.inline(node.Content) + "\n\n")
	case "bulletList":
		w.list(node, indent, false)
	case "orderedList":
		w.list(node, indent, true)
	case "taskList":
		for _, item := range node.Content {
			if item == nil {
				continue
			}
			marker := "- [ ] "
			if item.AttrBool("checked") {
				marker = "- [x] "
			}
			w.listItem(item, indent, marker)
		}
		w.out.WriteString("\n")
	case "blockquote":
		var inner strings.Builder
		nested := &markdownWriter{out: &inner}
		nested.blocks(node.Content, "")
		for _, line := range strings.Split(strings.TrimRight(inner.String(), "\n"), "\n") {
			w.out.WriteString(indent + "> " + line + "\n")
		}
		w.out.WriteString("\n")
	case "codeBlock":
		language := node.AttrString("language")
		w.out.WriteString(indent + "```" + language + "\n")
		for _, line := range strings.Split(codeText(node), "\n") {
			w.out.WriteString(indent + line + "\n")
		}
		w.out.WriteString(indent + "```\n\n")
	case "horizontalRule":
		w.out.WriteString(indent + "---\n\n")
	case "image":
		w.out.WriteString(indent + w.inline([]*richdoc.Node{node}) + "\n\n")
	case "table":
		w.table(node, indent)
	default:
		w.blocks(node.Content, indent)
	}
}

func (w *markdownWriter) list(node *richdoc.Node, indent string, ordered bool) {
	number := node.AttrInt("start", 1)
	if number < 1 {
		number = 1
	}
	for _, item := range node.Content {
		if item == nil {
			continue
		}
		marker := "- "
		if ordered {
			marker = strconv.Itoa(number) + ". "
			number++
		}
		w.listItem(item, indent, marker)
	}
	w.out.WriteString("\n")
}

func (w *markdownWriter) listItem(item *richdoc.Node, indent, marker string) {
	var inner strings.Builder
	nested := &markdownWriter{out: &inner}
	nested.blocks(item.Content, "")
	lines := strings.Split(strings.TrimRight(inner.String(), "\n"), "\n")
	continuation := indent + strings.Repeat(" ", len([]rune(marker)))
	for index, line := range lines {
		if index == 0 {
			w.out.WriteString(indent + marker + line + "\n")
			continue
		}
		if strings.TrimSpace(line) == "" {
			w.out.WriteString("\n")
			continue
		}
		w.out.WriteString(continuation + line + "\n")
	}
}

func (w *markdownWriter) table(node *richdoc.Node, indent string) {
	rows, columns := tableGrid(node, func(cell *richdoc.Node) string {
		text := strings.TrimSpace(strings.ReplaceAll(w.cellText(cell), "\n", " "))
		return strings.ReplaceAll(text, "|", "\\|")
	})
	if len(rows) == 0 || columns == 0 {
		return
	}
	pad := func(cells []string) string {
		out := "|"
		for index := 0; index < columns; index++ {
			value := ""
			if index < len(cells) {
				value = cells[index]
			}
			out += " " + value + " |"
		}
		return out
	}
	w.out.WriteString(indent + pad(rows[0]) + "\n")
	w.out.WriteString(indent + "|" + strings.Repeat(" --- |", columns) + "\n")
	for _, row := range rows[1:] {
		w.out.WriteString(indent + pad(row) + "\n")
	}
	w.out.WriteString("\n")
}

func (w *markdownWriter) cellText(cell *richdoc.Node) string {
	var inner strings.Builder
	nested := &markdownWriter{out: &inner}
	nested.blocks(cell.Content, "")
	return inner.String()
}

func (w *markdownWriter) inline(nodes []*richdoc.Node) string {
	var out strings.Builder
	for _, node := range nodes {
		if node == nil {
			continue
		}
		switch node.Type {
		case "text":
			out.WriteString(markdownMarks(escapeMarkdown(node.Text), node.Marks))
		case "hardBreak":
			out.WriteString("  \n")
		case "image":
			src := node.AttrString("src")
			if !safeImageSource(src) {
				continue
			}
			out.WriteString("![" + escapeMarkdown(node.AttrString("alt")) + "](" + src + ")")
		default:
			out.WriteString(w.inline(node.Content))
		}
	}
	return out.String()
}

func markdownMarks(value string, marks []richdoc.Mark) string {
	for _, mark := range marks {
		switch mark.Type {
		case "code":
			value = "`" + strings.ReplaceAll(value, "`", "'") + "`"
		case "bold":
			value = "**" + value + "**"
		case "italic":
			value = "*" + value + "*"
		case "strike":
			value = "~~" + value + "~~"
		case "underline":
			value = "<u>" + value + "</u>"
		case "highlight":
			value = "==" + value + "=="
		case "link":
			href := strings.TrimSpace(mark.AttrString("href"))
			lower := strings.ToLower(href)
			if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "mailto:") {
				value = "[" + value + "](" + href + ")"
			}
		}
	}
	return value
}

func escapeMarkdown(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, "`", "\\`", "*", `\*`, "_", `\_`, "[", `\[`, "]", `\]`)
	return replacer.Replace(value)
}

// renderPlainText flattens the document while keeping list markers and table
// columns readable.
func renderPlainText(title string, raw json.RawMessage) string {
	document, err := richdoc.Parse(raw)
	if err != nil {
		return ""
	}
	var out strings.Builder
	if strings.TrimSpace(title) != "" {
		out.WriteString(title + "\n\n")
	}
	var walk func(nodes []*richdoc.Node, indent string)
	inline := func(node *richdoc.Node) string {
		var text strings.Builder
		var collect func(*richdoc.Node)
		collect = func(current *richdoc.Node) {
			if current == nil {
				return
			}
			switch current.Type {
			case "text":
				text.WriteString(current.Text)
			case "hardBreak":
				text.WriteString("\n")
			}
			for _, child := range current.Content {
				collect(child)
			}
		}
		for _, child := range node.Content {
			collect(child)
		}
		return strings.TrimSpace(text.String())
	}
	walk = func(nodes []*richdoc.Node, indent string) {
		number := 1
		for _, node := range nodes {
			if node == nil {
				continue
			}
			switch node.Type {
			case "heading":
				out.WriteString("\n" + indent + inline(node) + "\n")
			case "paragraph":
				if text := inline(node); text != "" {
					out.WriteString(indent + text + "\n")
				}
			case "codeBlock":
				for _, line := range strings.Split(codeText(node), "\n") {
					out.WriteString(indent + "    " + line + "\n")
				}
			case "horizontalRule":
				out.WriteString(indent + strings.Repeat("-", 40) + "\n")
			case "bulletList", "taskList":
				for _, item := range node.Content {
					marker := "• "
					if node.Type == "taskList" {
						marker = "[ ] "
						if item.AttrBool("checked") {
							marker = "[x] "
						}
					}
					out.WriteString(indent + marker)
					walkItem(&out, item, indent+"  ", walk)
				}
			case "orderedList":
				number = node.AttrInt("start", 1)
				for _, item := range node.Content {
					out.WriteString(indent + strconv.Itoa(number) + ". ")
					number++
					walkItem(&out, item, indent+"   ", walk)
				}
			case "table":
				rows, columns := tableGrid(node, func(cell *richdoc.Node) string {
					return strings.ReplaceAll(cell.PlainText(), "\n", " ")
				})
				for _, row := range rows {
					for len(row) < columns {
						row = append(row, "")
					}
					out.WriteString(indent + strings.Join(row, "\t") + "\n")
				}
				out.WriteString("\n")
			case "image":
				if alt := node.AttrString("alt"); alt != "" {
					out.WriteString(indent + "[" + alt + "]\n")
				}
			default:
				walk(node.Content, indent)
			}
		}
	}
	walk(document.Content, "")
	return strings.TrimSpace(out.String()) + "\n"
}

func walkItem(out *strings.Builder, item *richdoc.Node, indent string, walk func([]*richdoc.Node, string)) {
	if item == nil {
		out.WriteString("\n")
		return
	}
	first := true
	for _, child := range item.Content {
		if child == nil {
			continue
		}
		if first && (child.Type == "paragraph" || child.Type == "heading") {
			out.WriteString(child.PlainText() + "\n")
			first = false
			continue
		}
		walk([]*richdoc.Node{child}, indent)
	}
	if first {
		out.WriteString("\n")
	}
}

// tableGrid lays a table out as a rectangular grid, expanding colspan and
// rowspan into blank continuation cells so formats without merged-cell
// support still line their columns up.
func tableGrid(node *richdoc.Node, text func(*richdoc.Node) string) ([][]string, int) {
	occupied := map[int]map[int]bool{}
	mark := func(row, column int) {
		if occupied[row] == nil {
			occupied[row] = map[int]bool{}
		}
		occupied[row][column] = true
	}
	sourceRows := make([]*richdoc.Node, 0, len(node.Content))
	for _, row := range node.Content {
		if row != nil && row.Type == "tableRow" {
			sourceRows = append(sourceRows, row)
		}
	}
	grid := make([][]string, len(sourceRows))
	columns := 0
	for rowIndex, row := range sourceRows {
		for _, cell := range row.Content {
			if cell == nil || (cell.Type != "tableCell" && cell.Type != "tableHeader") {
				continue
			}
			column := 0
			for occupied[rowIndex] != nil && occupied[rowIndex][column] {
				column++
			}
			colspan := cell.AttrInt("colspan", 1)
			if colspan < 1 {
				colspan = 1
			}
			rowspan := cell.AttrInt("rowspan", 1)
			if rowspan < 1 {
				rowspan = 1
			}
			if rowIndex+rowspan > len(sourceRows) {
				rowspan = len(sourceRows) - rowIndex
			}
			for offsetRow := 0; offsetRow < rowspan; offsetRow++ {
				for offsetColumn := 0; offsetColumn < colspan; offsetColumn++ {
					mark(rowIndex+offsetRow, column+offsetColumn)
				}
			}
			for len(grid[rowIndex]) <= column {
				grid[rowIndex] = append(grid[rowIndex], "")
			}
			grid[rowIndex][column] = text(cell)
			if column+colspan > columns {
				columns = column + colspan
			}
		}
	}
	for index := range grid {
		for len(grid[index]) < columns {
			grid[index] = append(grid[index], "")
		}
	}
	return grid, columns
}
