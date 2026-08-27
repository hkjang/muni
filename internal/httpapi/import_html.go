package httpapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/hkjang/muni/internal/richdoc"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type dataURIImage struct {
	mediaType string
	data      []byte
}

// decodeDataURIImage extracts the bytes of a base64 data: URI image.
func decodeDataURIImage(value string) (dataURIImage, bool) {
	if !strings.HasPrefix(value, "data:image/") {
		return dataURIImage{}, false
	}
	comma := strings.Index(value, ",")
	if comma < 0 {
		return dataURIImage{}, false
	}
	meta := value[len("data:"):comma]
	if !strings.Contains(meta, "base64") {
		return dataURIImage{}, false
	}
	mediaType := strings.ToLower(strings.SplitN(meta, ";", 2)[0])
	if !safeInlineImageType(mediaType) {
		return dataURIImage{}, false
	}
	payload := strings.NewReplacer("\n", "", "\r", "", " ", "").Replace(value[comma+1:])
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(data) == 0 {
		return dataURIImage{}, false
	}
	return dataURIImage{mediaType: mediaType, data: data}, true
}

func imageAssetName(alt, mediaType string) string {
	name := strings.TrimSpace(alt)
	if name == "" {
		name = "image"
	}
	name = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r < 0x20 {
			return -1
		}
		return r
	}, name)
	extension := ".png"
	switch mediaType {
	case "image/jpeg":
		extension = ".jpg"
	case "image/gif":
		extension = ".gif"
	case "image/webp":
		extension = ".webp"
	}
	if strings.HasSuffix(strings.ToLower(name), extension) {
		return truncateRunes(name, 200)
	}
	return truncateRunes(name, 200) + extension
}

// htmlDocument converts an HTML file into a document, keeping headings, lists,
// tables, inline formatting, links and inline images. Script and style content
// never reaches the output.
func htmlDocument(body []byte) (json.RawMessage, []richdoc.Asset, error) {
	root, err := xhtml.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	converter := &htmlConverter{inline: &inlineContext{}}
	start := findHTMLBody(root)
	blocks := converter.blocks(childrenOf(start), 0)
	if len(blocks) == 0 {
		if text := strings.TrimSpace(visibleText(start)); text != "" {
			blocks = append(blocks, richdoc.Paragraph(richdoc.Text(text)))
		}
	}
	document := richdoc.Doc(blocks...)
	richdoc.LiftImages(document)
	content, err := document.JSON()
	if err != nil {
		return nil, nil, err
	}
	return content, converter.inline.assets, nil
}

func findHTMLBody(root *xhtml.Node) *xhtml.Node {
	var found *xhtml.Node
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if found != nil {
			return
		}
		if node.Type == xhtml.ElementNode && node.DataAtom == atom.Body {
			found = node
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	if found == nil {
		return root
	}
	return found
}

func childrenOf(node *xhtml.Node) []*xhtml.Node {
	if node == nil {
		return nil
	}
	out := make([]*xhtml.Node, 0, 8)
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		out = append(out, child)
	}
	return out
}

func visibleText(node *xhtml.Node) string {
	if node == nil {
		return ""
	}
	if node.Type == xhtml.TextNode {
		return node.Data
	}
	if node.Type == xhtml.ElementNode && isIgnoredHTMLElement(node.Data) {
		return ""
	}
	var out strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		out.WriteString(visibleText(child))
	}
	return out.String()
}

func isIgnoredHTMLElement(name string) bool {
	switch strings.ToLower(name) {
	case "script", "style", "head", "title", "meta", "link", "noscript", "template",
		"iframe", "object", "embed", "svg", "canvas", "audio", "video", "form", "select", "textarea", "button":
		return true
	}
	return false
}

const maxHTMLDepth = 24

type htmlConverter struct {
	inline *inlineContext
}

func htmlAttr(node *xhtml.Node, name string) string {
	if node == nil {
		return ""
	}
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, name) {
			return attribute.Val
		}
	}
	return ""
}

func (h *htmlConverter) blocks(nodes []*xhtml.Node, depth int) []*richdoc.Node {
	out := make([]*richdoc.Node, 0, len(nodes))
	pending := make([]*xhtml.Node, 0, 4)
	flush := func() {
		if len(pending) == 0 {
			return
		}
		if inline := h.inlineNodes(pending, nil, depth); len(inline) > 0 {
			out = append(out, richdoc.Paragraph(inline...))
		}
		pending = pending[:0]
	}
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.Type == xhtml.CommentNode || node.Type == xhtml.DoctypeNode {
			continue
		}
		if node.Type == xhtml.TextNode {
			if strings.TrimSpace(node.Data) != "" {
				pending = append(pending, node)
			}
			continue
		}
		if node.Type != xhtml.ElementNode || isIgnoredHTMLElement(node.Data) {
			continue
		}
		if isInlineHTMLElement(node.Data) {
			pending = append(pending, node)
			continue
		}
		flush()
		out = append(out, h.block(node, depth)...)
	}
	flush()
	return out
}

func isInlineHTMLElement(name string) bool {
	switch strings.ToLower(name) {
	case "a", "abbr", "b", "bdi", "bdo", "br", "cite", "code", "data", "dfn", "em", "i",
		"img", "kbd", "mark", "q", "s", "samp", "small", "span", "strong", "sub", "sup",
		"time", "u", "var", "wbr", "del", "ins", "font", "big", "tt", "strike", "nobr", "label":
		return true
	}
	return false
}

func (h *htmlConverter) block(node *xhtml.Node, depth int) []*richdoc.Node {
	if depth > maxHTMLDepth {
		return nil
	}
	blocks := h.blockOfType(node, depth)
	// Restore the identity the exporter wrote, so a document exported and
	// imported again keeps the anchors its comments and citations point at.
	if id := strings.TrimSpace(htmlAttr(node, "data-block-id")); isBlockIDToken(id) && len(blocks) == 1 {
		blocks[0].SetAttr(richdoc.BlockIDAttr, id)
	}
	return blocks
}

func (h *htmlConverter) blockOfType(node *xhtml.Node, depth int) []*richdoc.Node {
	name := strings.ToLower(node.Data)
	switch name {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		heading := &richdoc.Node{Type: "heading", Content: h.inlineNodes(childrenOf(node), nil, depth+1)}
		heading.SetAttr("level", int(name[1]-'0'))
		applyHTMLAlignment(heading, node)
		if heading.IsBlank() {
			return nil
		}
		return []*richdoc.Node{heading}

	case "p":
		inline := h.inlineNodes(childrenOf(node), nil, depth+1)
		if len(inline) == 0 {
			return nil
		}
		paragraph := richdoc.Paragraph(inline...)
		applyHTMLAlignment(paragraph, node)
		return []*richdoc.Node{paragraph}

	case "ul", "ol", "menu":
		return h.list(node, depth)

	case "dl":
		return h.definitionList(node, depth)

	case "blockquote":
		inner := h.blocks(childrenOf(node), depth+1)
		if len(inner) == 0 {
			inner = []*richdoc.Node{richdoc.Paragraph()}
		}
		return []*richdoc.Node{{Type: "blockquote", Content: inner}}

	case "pre":
		return []*richdoc.Node{h.codeBlock(node)}

	case "hr":
		return []*richdoc.Node{{Type: "horizontalRule"}}

	case "table":
		if table := h.table(node, depth); table != nil {
			return []*richdoc.Node{table}
		}
		return nil

	case "figure":
		return h.blocks(childrenOf(node), depth+1)

	case "figcaption":
		inline := h.inlineNodes(childrenOf(node), nil, depth+1)
		if len(inline) == 0 {
			return nil
		}
		return []*richdoc.Node{richdoc.Paragraph(inline...)}

	case "br":
		return nil

	default:
		// Generic containers pass through; a container holding only inline
		// content becomes a paragraph of its own.
		children := childrenOf(node)
		if !hasBlockChild(children) {
			inline := h.inlineNodes(children, nil, depth+1)
			if len(inline) == 0 {
				return nil
			}
			paragraph := richdoc.Paragraph(inline...)
			applyHTMLAlignment(paragraph, node)
			return []*richdoc.Node{paragraph}
		}
		return h.blocks(children, depth+1)
	}
}

func hasBlockChild(nodes []*xhtml.Node) bool {
	for _, node := range nodes {
		if node.Type == xhtml.ElementNode && !isInlineHTMLElement(node.Data) && !isIgnoredHTMLElement(node.Data) {
			return true
		}
	}
	return false
}

func applyHTMLAlignment(target *richdoc.Node, node *xhtml.Node) {
	align := strings.ToLower(strings.TrimSpace(htmlAttr(node, "align")))
	if align == "" {
		align = strings.ToLower(styleProperty(htmlAttr(node, "style"), "text-align"))
	}
	switch align {
	case "center", "right", "justify":
		target.SetAttr("textAlign", align)
	case "left", "start":
		target.SetAttr("textAlign", "left")
	}
}

// styleProperty reads one declaration out of an inline style attribute.
func styleProperty(style, property string) string {
	for _, declaration := range strings.Split(style, ";") {
		parts := strings.SplitN(declaration, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(parts[0]), property) {
			return strings.TrimSpace(parts[1])
		}
	}
	return ""
}

func (h *htmlConverter) codeBlock(node *xhtml.Node) *richdoc.Node {
	block := &richdoc.Node{Type: "codeBlock"}
	language := ""
	var findCode func(*xhtml.Node)
	findCode = func(current *xhtml.Node) {
		if language != "" || current == nil {
			return
		}
		if current.Type == xhtml.ElementNode && strings.EqualFold(current.Data, "code") {
			for _, class := range strings.Fields(htmlAttr(current, "class")) {
				if value := strings.TrimPrefix(class, "language-"); value != class {
					language = sanitizeLanguage(value)
					return
				}
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			findCode(child)
		}
	}
	findCode(node)
	if language != "" {
		block.SetAttr("language", language)
	}
	text := strings.TrimRight(strings.TrimLeft(visibleText(node), "\n"), "\n")
	if text != "" {
		block.Content = []*richdoc.Node{richdoc.Text(text)}
	}
	return block
}

func (h *htmlConverter) list(node *xhtml.Node, depth int) []*richdoc.Node {
	ordered := strings.EqualFold(node.Data, "ol")
	task := strings.EqualFold(htmlAttr(node, "data-type"), "taskList")
	listType, itemType := "bulletList", "listItem"
	switch {
	case task:
		listType, itemType = "taskList", "taskItem"
	case ordered:
		listType = "orderedList"
	}
	list := &richdoc.Node{Type: listType}
	if ordered {
		start := 1
		if value, err := strconv.Atoi(strings.TrimSpace(htmlAttr(node, "start"))); err == nil && value > 0 {
			start = value
		}
		list.SetAttr("start", start)
	}
	for _, child := range childrenOf(node) {
		if child.Type != xhtml.ElementNode || !strings.EqualFold(child.Data, "li") {
			continue
		}
		inner := h.blocks(childrenOf(child), depth+1)
		if len(inner) == 0 {
			inner = []*richdoc.Node{richdoc.Paragraph()}
		}
		item := &richdoc.Node{Type: itemType, Content: inner}
		if id := strings.TrimSpace(htmlAttr(child, "data-block-id")); isBlockIDToken(id) {
			item.SetAttr(richdoc.BlockIDAttr, id)
		}
		if task {
			checked := strings.EqualFold(htmlAttr(child, "data-checked"), "true")
			if !checked {
				checked = hasCheckedInput(child)
			}
			item.SetAttr("checked", checked)
			item.Content = dropCheckboxGlyph(item.Content)
		}
		list.Content = append(list.Content, item)
	}
	if len(list.Content) == 0 {
		return nil
	}
	return []*richdoc.Node{list}
}

func hasCheckedInput(node *xhtml.Node) bool {
	found := false
	var walk func(*xhtml.Node)
	walk = func(current *xhtml.Node) {
		if found || current == nil {
			return
		}
		if current.Type == xhtml.ElementNode && strings.EqualFold(current.Data, "input") {
			if htmlAttr(current, "checked") != "" || strings.EqualFold(htmlAttr(current, "aria-checked"), "true") {
				found = true
				return
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return found
}

// definitionList keeps description lists readable by turning terms into bold
// paragraphs and definitions into the following paragraph.
func (h *htmlConverter) definitionList(node *xhtml.Node, depth int) []*richdoc.Node {
	out := make([]*richdoc.Node, 0, 4)
	for _, child := range childrenOf(node) {
		if child.Type != xhtml.ElementNode {
			continue
		}
		switch strings.ToLower(child.Data) {
		case "dt":
			inline := h.inlineNodes(childrenOf(child), []richdoc.Mark{{Type: "bold"}}, depth+1)
			if len(inline) > 0 {
				out = append(out, richdoc.Paragraph(inline...))
			}
		case "dd":
			inner := h.blocks(childrenOf(child), depth+1)
			out = append(out, inner...)
		}
	}
	return out
}

func (h *htmlConverter) table(node *xhtml.Node, depth int) *richdoc.Node {
	rows := make([]*xhtml.Node, 0, 8)
	var collect func(*xhtml.Node)
	collect = func(current *xhtml.Node) {
		for _, child := range childrenOf(current) {
			if child.Type != xhtml.ElementNode {
				continue
			}
			switch strings.ToLower(child.Data) {
			case "tr":
				rows = append(rows, child)
			case "thead", "tbody", "tfoot":
				collect(child)
			}
		}
	}
	collect(node)
	if len(rows) == 0 {
		return nil
	}
	table := &richdoc.Node{Type: "table"}
	for _, row := range rows {
		rowNode := &richdoc.Node{Type: "tableRow"}
		for _, cell := range childrenOf(row) {
			if cell.Type != xhtml.ElementNode {
				continue
			}
			name := strings.ToLower(cell.Data)
			if name != "td" && name != "th" {
				continue
			}
			cellType := "tableCell"
			if name == "th" {
				cellType = "tableHeader"
			}
			inner := h.blocks(childrenOf(cell), depth+1)
			if len(inner) == 0 {
				inner = []*richdoc.Node{richdoc.Paragraph()}
			}
			cellNode := &richdoc.Node{Type: cellType, Content: inner}
			cellNode.SetAttr("colspan", spanAttr(cell, "colspan"))
			cellNode.SetAttr("rowspan", spanAttr(cell, "rowspan"))
			if width := cellWidth(cell); width > 0 {
				widths := make([]any, spanAttr(cell, "colspan"))
				for index := range widths {
					widths[index] = width
				}
				cellNode.SetAttr("colwidth", widths)
			}
			rowNode.Content = append(rowNode.Content, cellNode)
		}
		if len(rowNode.Content) > 0 {
			table.Content = append(table.Content, rowNode)
		}
	}
	if len(table.Content) == 0 {
		return nil
	}
	return table
}

func spanAttr(node *xhtml.Node, name string) int {
	value, err := strconv.Atoi(strings.TrimSpace(htmlAttr(node, name)))
	if err != nil || value < 1 || value > 64 {
		return 1
	}
	return value
}

func cellWidth(node *xhtml.Node) int {
	raw := strings.TrimSpace(htmlAttr(node, "width"))
	if raw == "" {
		raw = styleProperty(htmlAttr(node, "style"), "width")
	}
	raw = strings.TrimSuffix(strings.TrimSpace(raw), "px")
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 || value > 4000 {
		return 0
	}
	return value
}

func (h *htmlConverter) inlineNodes(nodes []*xhtml.Node, marks []richdoc.Mark, depth int) []*richdoc.Node {
	out := make([]*richdoc.Node, 0, len(nodes))
	for _, node := range nodes {
		if node == nil || depth > maxHTMLDepth {
			continue
		}
		switch node.Type {
		case xhtml.TextNode:
			if text := collapseHTMLWhitespace(node.Data); text != "" {
				out = append(out, richdoc.Text(text, marks...))
			}
		case xhtml.ElementNode:
			if isIgnoredHTMLElement(node.Data) {
				continue
			}
			out = append(out, h.inlineElement(node, marks, depth)...)
		}
	}
	return mergeInlineText(trimEdgeSpace(out))
}

func (h *htmlConverter) inlineElement(node *xhtml.Node, marks []richdoc.Mark, depth int) []*richdoc.Node {
	name := strings.ToLower(node.Data)
	switch name {
	case "br":
		return []*richdoc.Node{{Type: "hardBreak"}}
	case "img":
		if image := h.inline.imageNode(strings.TrimSpace(htmlAttr(node, "src")), htmlAttr(node, "alt")); image != nil {
			if width, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(htmlAttr(node, "width")), "px")); err == nil && width > 0 {
				image.SetAttr("width", width)
			}
			return []*richdoc.Node{image}
		}
		if alt := strings.TrimSpace(htmlAttr(node, "alt")); alt != "" {
			return []*richdoc.Node{richdoc.Text(alt, marks...)}
		}
		return nil
	case "wbr":
		return nil
	}

	next := append([]richdoc.Mark{}, marks...)
	if mark, ok := inlineHTMLMarks[name]; ok {
		switch mark {
		case "highlight":
			color := cssColor(styleProperty(htmlAttr(node, "style"), "background-color"))
			if color == "" {
				color = "#FFF3A3"
			}
			next = append(next, richdoc.Mark{Type: "highlight", Attrs: map[string]any{"color": color}})
		default:
			next = append(next, richdoc.Mark{Type: mark})
		}
	}
	if name == "a" {
		if href := safeLinkTarget(htmlAttr(node, "href")); href != "" {
			next = append(next, richdoc.Mark{Type: "link", Attrs: map[string]any{"href": href, "target": "_blank"}})
		}
	}
	if style := htmlAttr(node, "style"); style != "" || name == "font" {
		if mark, ok := textStyleMark(node, style); ok {
			next = append(next, mark)
		}
		if background := cssColor(styleProperty(style, "background-color")); background != "" && name != "mark" {
			next = append(next, richdoc.Mark{Type: "highlight", Attrs: map[string]any{"color": background}})
		}
		if strings.Contains(strings.ToLower(styleProperty(style, "font-weight")), "bold") {
			next = append(next, richdoc.Mark{Type: "bold"})
		}
		if strings.EqualFold(styleProperty(style, "font-style"), "italic") {
			next = append(next, richdoc.Mark{Type: "italic"})
		}
		if strings.Contains(strings.ToLower(styleProperty(style, "text-decoration")), "underline") {
			next = append(next, richdoc.Mark{Type: "underline"})
		}
		if strings.Contains(strings.ToLower(styleProperty(style, "text-decoration")), "line-through") {
			next = append(next, richdoc.Mark{Type: "strike"})
		}
	}
	return h.inlineNodes(childrenOf(node), next, depth+1)
}

func textStyleMark(node *xhtml.Node, style string) (richdoc.Mark, bool) {
	attrs := map[string]any{}
	if color := cssColor(styleProperty(style, "color")); color != "" {
		attrs["color"] = color
	} else if color := cssColor(htmlAttr(node, "color")); color != "" {
		attrs["color"] = color
	}
	if family := strings.TrimSpace(styleProperty(style, "font-family")); family != "" && isSafeStyleValue(family) {
		attrs["fontFamily"] = family
	}
	if size := strings.TrimSpace(styleProperty(style, "font-size")); size != "" && isSafeStyleValue(size) {
		attrs["fontSize"] = size
	}
	if len(attrs) == 0 {
		return richdoc.Mark{}, false
	}
	return richdoc.Mark{Type: "textStyle", Attrs: attrs}, true
}

// collapseHTMLWhitespace applies the normal (non-pre) whitespace rules: any run
// of whitespace becomes a single space. A whitespace-only node keeps that space
// because it separates two inline elements; trimEdgeSpace drops it again at
// block boundaries.
func collapseHTMLWhitespace(value string) string {
	var out strings.Builder
	space := false
	for _, r := range value {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' || r == 0x00a0 {
			space = true
			continue
		}
		if space {
			out.WriteRune(' ')
			space = false
		}
		out.WriteRune(r)
	}
	if space {
		out.WriteRune(' ')
	}
	return out.String()
}

// trimEdgeSpace removes the leading and trailing whitespace a block picks up
// from indented markup.
func trimEdgeSpace(nodes []*richdoc.Node) []*richdoc.Node {
	for len(nodes) > 0 && nodes[0].Type == "text" {
		trimmed := strings.TrimLeft(nodes[0].Text, " ")
		if trimmed == nodes[0].Text {
			break
		}
		if trimmed == "" {
			nodes = nodes[1:]
			continue
		}
		nodes[0].Text = trimmed
		break
	}
	for len(nodes) > 0 {
		last := nodes[len(nodes)-1]
		if last.Type != "text" {
			break
		}
		trimmed := strings.TrimRight(last.Text, " ")
		if trimmed == last.Text {
			break
		}
		if trimmed == "" {
			nodes = nodes[:len(nodes)-1]
			continue
		}
		last.Text = trimmed
		break
	}
	return nodes
}

// dropCheckboxGlyph removes the decorative check-box character that renderers
// place before a task item's text; the state lives in the checked attribute.
func dropCheckboxGlyph(blocks []*richdoc.Node) []*richdoc.Node {
	for len(blocks) > 1 {
		text := strings.TrimSpace(blocks[0].PlainText())
		if text == "" || !isCheckboxGlyphText(text) {
			break
		}
		blocks = blocks[1:]
	}
	if len(blocks) == 1 {
		if trimmed, ok := stripLeadingCheckbox(blocks[0]); ok {
			blocks[0] = trimmed
		}
	}
	return blocks
}

func isCheckboxGlyphText(text string) bool {
	for _, r := range text {
		switch r {
		case '\u2610', '\u2611', '\u2612', '\u25a1', '\u25a0', '\u2705', ' ':
		default:
			return false
		}
	}
	return true
}

// stripLeadingCheckbox removes a check-box glyph that renderers inlined into
// the item's own first paragraph.
func stripLeadingCheckbox(block *richdoc.Node) (*richdoc.Node, bool) {
	if block == nil || len(block.Content) == 0 || block.Content[0].Type != "text" {
		return block, false
	}
	head := block.Content[0]
	trimmed := strings.TrimLeft(head.Text, " ")
	if trimmed == "" {
		return block, false
	}
	first := []rune(trimmed)[0]
	if !isCheckboxGlyphText(string(first)) || first == ' ' {
		return block, false
	}
	rest := strings.TrimLeft(string([]rune(trimmed)[1:]), " ")
	if rest == "" {
		block.Content = block.Content[1:]
		return block, true
	}
	head.Text = rest
	return block, true
}
