package httpapi

import (
	"strings"
	"unicode"

	"github.com/hkjang/muni/internal/richdoc"
)

// inlineContext parses Markdown inline syntax and collects the images it finds
// so the caller can turn them into attachments.
type inlineContext struct {
	assets []richdoc.Asset
}

// asset registers embedded image bytes and returns the placeholder source that
// the import handler later rewrites to an attachment URL.
func (c *inlineContext) asset(mediaType string, data []byte, name string) string {
	placeholder := richdoc.Placeholder(len(c.assets) + 1)
	c.assets = append(c.assets, richdoc.Asset{
		Placeholder: placeholder,
		Name:        name,
		MediaType:   mediaType,
		Data:        data,
	})
	return placeholder
}

func (c *inlineContext) parse(text string) []*richdoc.Node {
	return mergeInlineText(c.parseWithMarks(text, nil, 0))
}

const maxInlineDepth = 12

type inlineDelimiter struct {
	token string
	mark  string
}

var inlineDelimiters = []inlineDelimiter{
	{"***", "bold+italic"},
	{"___", "bold+italic"},
	{"**", "bold"},
	{"__", "bold"},
	{"~~", "strike"},
	{"==", "highlight"},
	{"*", "italic"},
	{"_", "italic"},
}

func (c *inlineContext) parseWithMarks(text string, marks []richdoc.Mark, depth int) []*richdoc.Node {
	out := make([]*richdoc.Node, 0, 4)
	var buffer strings.Builder
	runes := []rune(text)

	flush := func() {
		if buffer.Len() > 0 {
			out = append(out, richdoc.Text(buffer.String(), marks...))
			buffer.Reset()
		}
	}

	for index := 0; index < len(runes); {
		r := runes[index]
		switch {
		case r == '\\' && index+1 < len(runes) && isMarkdownPunctuation(runes[index+1]):
			buffer.WriteRune(runes[index+1])
			index += 2

		case r == '\n':
			// Two trailing spaces or a backslash request a hard break; anything
			// else is a soft break that simply joins the lines.
			content := buffer.String()
			trimmed := strings.TrimRight(content, " ")
			hard := len(content)-len(trimmed) >= 2 || strings.HasSuffix(trimmed, "\\")
			trimmed = strings.TrimSuffix(trimmed, "\\")
			buffer.Reset()
			buffer.WriteString(trimmed)
			flush()
			index++
			if hard {
				out = append(out, &richdoc.Node{Type: "hardBreak"})
				continue
			}
			if !softBreakJoinsTightly(trimmed, string(runes[index:])) {
				buffer.WriteString(" ")
			}

		case r == '`':
			node, width := codeSpan(runes[index:], marks)
			if width == 0 {
				buffer.WriteRune(r)
				index++
				continue
			}
			flush()
			out = append(out, node)
			index += width

		case r == '!' && index+1 < len(runes) && runes[index+1] == '[' && depth < maxInlineDepth:
			nodes, width := c.imageOrLink(runes[index:], marks, depth, true)
			if width == 0 {
				buffer.WriteRune(r)
				index++
				continue
			}
			flush()
			out = append(out, nodes...)
			index += width

		case r == '[' && depth < maxInlineDepth:
			nodes, width := c.imageOrLink(runes[index:], marks, depth, false)
			if width == 0 {
				buffer.WriteRune(r)
				index++
				continue
			}
			flush()
			out = append(out, nodes...)
			index += width

		case r == '<':
			if node, width := autolink(runes[index:], marks); width > 0 {
				flush()
				out = append(out, node)
				index += width
				continue
			}
			if nodes, width := c.inlineHTML(runes[index:], marks, depth); width > 0 {
				flush()
				out = append(out, nodes...)
				index += width
				continue
			}
			buffer.WriteRune(r)
			index++

		default:
			if depth < maxInlineDepth {
				previous := rune(0)
				if index > 0 {
					previous = runes[index-1]
				}
				if nodes, width := c.emphasis(runes[index:], previous, marks, depth); width > 0 {
					flush()
					out = append(out, nodes...)
					index += width
					continue
				}
			}
			buffer.WriteRune(r)
			index++
		}
	}
	flush()
	return out
}

// softBreakJoinsTightly reports whether a wrapped line should be rejoined
// without a space, which is what CJK text expects.
func softBreakJoinsTightly(left, right string) bool {
	leftRunes := []rune(strings.TrimRight(left, " "))
	rightRunes := []rune(strings.TrimLeft(right, " "))
	if len(leftRunes) == 0 || len(rightRunes) == 0 {
		return true
	}
	return isCJKRune(leftRunes[len(leftRunes)-1]) && isCJKRune(rightRunes[0])
}

func isCJKRune(r rune) bool {
	return unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hangul, r) ||
		unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r)
}

func isMarkdownPunctuation(r rune) bool {
	return strings.ContainsRune("\\`*_{}[]()#+-.!|~<>=\"'", r)
}

func codeSpan(runes []rune, marks []richdoc.Mark) (*richdoc.Node, int) {
	opening := 0
	for opening < len(runes) && runes[opening] == '`' {
		opening++
	}
	for index := opening; index < len(runes); {
		if runes[index] != '`' {
			index++
			continue
		}
		closing := 0
		for index+closing < len(runes) && runes[index+closing] == '`' {
			closing++
		}
		if closing != opening {
			index += closing
			continue
		}
		body := string(runes[opening:index])
		if strings.HasPrefix(body, " ") && strings.HasSuffix(body, " ") && strings.TrimSpace(body) != "" {
			body = body[1 : len(body)-1]
		}
		body = strings.ReplaceAll(body, "\n", " ")
		return richdoc.Text(body, append(append([]richdoc.Mark{}, marks...), richdoc.Mark{Type: "code"})...), index + closing
	}
	return nil, 0
}

func (c *inlineContext) emphasis(runes []rune, previous rune, marks []richdoc.Mark, depth int) ([]*richdoc.Node, int) {
	for _, delimiter := range inlineDelimiters {
		token := []rune(delimiter.token)
		if !hasRunePrefix(runes, token) {
			continue
		}
		// CommonMark forbids intraword emphasis with underscores, which is what
		// keeps snake_case identifiers intact.
		if token[0] == '_' && isWordRune(previous) {
			continue
		}
		closing := findRunes(runes, token, len(token))
		if closing < 0 {
			continue
		}
		if token[0] == '_' && closing+len(token) < len(runes) && isWordRune(runes[closing+len(token)]) {
			continue
		}
		body := runes[len(token):closing]
		if len(body) == 0 || strings.TrimSpace(string(body)) == "" {
			continue
		}
		if unicode.IsSpace(body[0]) {
			continue
		}
		next := append([]richdoc.Mark{}, marks...)
		switch delimiter.mark {
		case "bold+italic":
			next = append(next, richdoc.Mark{Type: "bold"}, richdoc.Mark{Type: "italic"})
		case "highlight":
			next = append(next, richdoc.Mark{Type: "highlight", Attrs: map[string]any{"color": "#FFF3A3"}})
		default:
			next = append(next, richdoc.Mark{Type: delimiter.mark})
		}
		return c.parseWithMarks(string(body), next, depth+1), closing + len(token)
	}
	return nil, 0
}

func hasRunePrefix(runes, prefix []rune) bool {
	if len(runes) < len(prefix) {
		return false
	}
	for index, r := range prefix {
		if runes[index] != r {
			return false
		}
	}
	return true
}

func findRunes(runes, needle []rune, from int) int {
	for index := from; index+len(needle) <= len(runes); index++ {
		if runes[index] == '\\' {
			index++
			continue
		}
		if hasRunePrefix(runes[index:], needle) {
			return index
		}
	}
	return -1
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// imageOrLink parses "[text](dest)" and "![alt](src)".
func (c *inlineContext) imageOrLink(runes []rune, marks []richdoc.Mark, depth int, isImage bool) ([]*richdoc.Node, int) {
	start := 0
	if isImage {
		start = 1
	}
	if start >= len(runes) || runes[start] != '[' {
		return nil, 0
	}
	depthCount := 0
	labelEnd := -1
	for index := start; index < len(runes); index++ {
		switch runes[index] {
		case '\\':
			index++
		case '[':
			depthCount++
		case ']':
			depthCount--
			if depthCount == 0 {
				labelEnd = index
			}
		}
		if labelEnd >= 0 {
			break
		}
	}
	if labelEnd < 0 || labelEnd+1 >= len(runes) || runes[labelEnd+1] != '(' {
		return nil, 0
	}
	parens := 0
	destEnd := -1
	for index := labelEnd + 1; index < len(runes); index++ {
		switch runes[index] {
		case '\\':
			index++
		case '(':
			parens++
		case ')':
			parens--
			if parens == 0 {
				destEnd = index
			}
		}
		if destEnd >= 0 {
			break
		}
	}
	if destEnd < 0 {
		return nil, 0
	}
	label := string(runes[start+1 : labelEnd])
	destination := strings.TrimSpace(string(runes[labelEnd+2 : destEnd]))
	if fields := strings.Fields(destination); len(fields) > 1 {
		destination = fields[0]
	}
	destination = strings.Trim(destination, "<>")
	width := destEnd + 1

	if isImage {
		if node := c.imageNode(destination, label); node != nil {
			return []*richdoc.Node{node}, width
		}
		// An image muni cannot store keeps its description as linked text.
		return c.linkedText(label, destination, marks, depth), width
	}
	return c.linkedText(label, destination, marks, depth), width
}

func (c *inlineContext) linkedText(label, destination string, marks []richdoc.Mark, depth int) []*richdoc.Node {
	if strings.TrimSpace(label) == "" {
		label = destination
	}
	next := marks
	if href := safeLinkTarget(destination); href != "" {
		next = append(append([]richdoc.Mark{}, marks...), richdoc.Mark{
			Type:  "link",
			Attrs: map[string]any{"href": href, "target": "_blank"},
		})
	}
	return c.parseWithMarks(label, next, depth+1)
}

func safeLinkTarget(value string) string {
	href := strings.TrimSpace(value)
	lower := strings.ToLower(href)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "mailto:") {
		return href
	}
	return ""
}

// imageNode keeps images muni can serve: inline data URIs become attachments
// and existing attachment URLs are preserved.
func (c *inlineContext) imageNode(source, alt string) *richdoc.Node {
	node := &richdoc.Node{Type: "image"}
	switch {
	case strings.HasPrefix(source, "data:image/"):
		picture, ok := decodeDataURIImage(source)
		if !ok {
			return nil
		}
		node.SetAttr("src", c.asset(picture.mediaType, picture.data, imageAssetName(alt, picture.mediaType)))
	case strings.HasPrefix(source, "/api/v1/attachments/"):
		node.SetAttr("src", source)
	default:
		return nil
	}
	if alt != "" {
		node.SetAttr("alt", alt)
	}
	return node
}

func autolink(runes []rune, marks []richdoc.Mark) (*richdoc.Node, int) {
	end := -1
	for index := 1; index < len(runes) && index < 2048; index++ {
		if runes[index] == '>' {
			end = index
			break
		}
		if unicode.IsSpace(runes[index]) {
			return nil, 0
		}
	}
	if end < 0 {
		return nil, 0
	}
	target := string(runes[1:end])
	href := safeLinkTarget(target)
	if href == "" {
		return nil, 0
	}
	linked := append(append([]richdoc.Mark{}, marks...), richdoc.Mark{
		Type:  "link",
		Attrs: map[string]any{"href": href, "target": "_blank"},
	})
	return richdoc.Text(target, linked...), end + 1
}

var inlineHTMLMarks = map[string]string{
	"u": "underline", "ins": "underline",
	"strong": "bold", "b": "bold",
	"em": "italic", "i": "italic",
	"s": "strike", "del": "strike", "strike": "strike",
	"mark": "highlight",
	"code": "code",
	"sup":  "superscript", "sub": "subscript",
}

// inlineHTML understands the handful of tags muni's Markdown export emits for
// styles CommonMark cannot express.
func (c *inlineContext) inlineHTML(runes []rune, marks []richdoc.Mark, depth int) ([]*richdoc.Node, int) {
	text := string(runes)
	lower := strings.ToLower(text)
	for _, form := range []string{"<br>", "<br/>", "<br />"} {
		if strings.HasPrefix(lower, form) {
			return []*richdoc.Node{{Type: "hardBreak"}}, len([]rune(form))
		}
	}
	for tag, mark := range inlineHTMLMarks {
		open := "<" + tag + ">"
		closing := "</" + tag + ">"
		if !strings.HasPrefix(lower, open) {
			continue
		}
		end := strings.Index(lower, closing)
		if end < 0 {
			continue
		}
		body := text[len(open):end]
		next := append([]richdoc.Mark{}, marks...)
		if mark == "highlight" {
			next = append(next, richdoc.Mark{Type: "highlight", Attrs: map[string]any{"color": "#FFF3A3"}})
		} else {
			next = append(next, richdoc.Mark{Type: mark})
		}
		return c.parseWithMarks(body, next, depth+1), len([]rune(text[:end+len(closing)]))
	}
	return nil, 0
}

// mergeInlineText joins neighbouring text nodes that carry identical marks so
// the stored document does not fragment into one node per character.
func mergeInlineText(nodes []*richdoc.Node) []*richdoc.Node {
	out := make([]*richdoc.Node, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.Type == "text" && node.Text == "" {
			continue
		}
		if len(out) > 0 && node.Type == "text" && out[len(out)-1].Type == "text" && sameMarkSet(out[len(out)-1].Marks, node.Marks) {
			out[len(out)-1].Text += node.Text
			continue
		}
		out = append(out, node)
	}
	return out
}

func sameMarkSet(left, right []richdoc.Mark) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Type != right[index].Type || len(left[index].Attrs) != len(right[index].Attrs) {
			return false
		}
		for key, value := range left[index].Attrs {
			if right[index].Attrs[key] != value {
				return false
			}
		}
	}
	return true
}
