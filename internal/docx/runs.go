package docx

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strconv"
	"strings"

	"github.com/hkjang/muni/internal/richdoc"
)

type runStyle struct {
	bold       bool
	italic     bool
	underline  bool
	strike     bool
	code       bool
	color      string
	highlight  string
	fontFamily string
	sizeHalf   int
	vertAlign  string
}

func (s runStyle) merge(marks []richdoc.Mark) runStyle {
	for _, mark := range marks {
		switch mark.Type {
		case "bold", "strong":
			s.bold = true
		case "italic", "em":
			s.italic = true
		case "underline":
			s.underline = true
		case "strike", "strikethrough", "s":
			s.strike = true
		case "code":
			s.code = true
		case "highlight":
			if color := hexColor(mark.AttrString("color")); color != "" {
				s.highlight = color
			} else {
				s.highlight = "FFF3A3"
			}
		case "textStyle":
			if color := hexColor(mark.AttrString("color")); color != "" {
				s.color = color
			}
			if family := cssFontFamily(mark.AttrString("fontFamily")); family != "" {
				s.fontFamily = family
			}
			if size := cssFontSizeHalfPoints(mark.AttrString("fontSize")); size > 0 {
				s.sizeHalf = size
			}
		case "superscript":
			s.vertAlign = "superscript"
		case "subscript":
			s.vertAlign = "subscript"
		}
	}
	return s
}

// properties emits CT_RPr children in schema order.
func (s runStyle) properties() string {
	var out strings.Builder
	if s.code {
		out.WriteString(`<w:rStyle w:val="CodeChar"/>`)
	}
	if s.fontFamily != "" {
		out.WriteString(`<w:rFonts` + attr("w:ascii", s.fontFamily) + attr("w:hAnsi", s.fontFamily) +
			attr("w:eastAsia", s.fontFamily) + attr("w:cs", s.fontFamily) + `/>`)
	}
	if s.bold {
		out.WriteString(`<w:b/><w:bCs/>`)
	}
	if s.italic {
		out.WriteString(`<w:i/><w:iCs/>`)
	}
	if s.strike {
		out.WriteString(`<w:strike/>`)
	}
	if s.color != "" {
		out.WriteString(`<w:color` + attr("w:val", s.color) + `/>`)
	}
	if s.sizeHalf > 0 {
		out.WriteString(`<w:sz` + intAttr("w:val", s.sizeHalf) + `/><w:szCs` + intAttr("w:val", s.sizeHalf) + `/>`)
	}
	if s.underline {
		out.WriteString(`<w:u w:val="single"/>`)
	}
	// CT_RPr orders shd after u/effect/bdr; Word rejects a package that
	// deviates from the schema sequence.
	if s.highlight != "" {
		out.WriteString(`<w:shd w:val="clear" w:color="auto"` + attr("w:fill", s.highlight) + `/>`)
	}
	if s.vertAlign != "" {
		out.WriteString(`<w:vertAlign` + attr("w:val", s.vertAlign) + `/>`)
	}
	if out.Len() == 0 {
		return ""
	}
	return `<w:rPr>` + out.String() + `</w:rPr>`
}

func (b *builder) textRun(value string, style runStyle) string {
	if value == "" {
		return ""
	}
	properties := style.properties()
	var out strings.Builder
	out.WriteString(`<w:r>` + properties)
	for index, line := range strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n") {
		if index > 0 {
			out.WriteString(`<w:br/>`)
		}
		for tabIndex, segment := range strings.Split(line, "\t") {
			if tabIndex > 0 {
				out.WriteString(`<w:tab/>`)
			}
			if segment != "" {
				out.WriteString(`<w:t xml:space="preserve">` + escapeXML(segment) + `</w:t>`)
			}
		}
	}
	out.WriteString(`</w:r>`)
	return out.String()
}

// inline renders a run of inline nodes, grouping neighbours that share a link
// target into a single w:hyperlink element.
func (b *builder) inline(nodes []*richdoc.Node, base runStyle) string {
	var out strings.Builder
	index := 0
	for index < len(nodes) {
		node := nodes[index]
		if node == nil {
			index++
			continue
		}
		href := linkTarget(node)
		if href == "" {
			out.WriteString(b.inlineNode(node, base))
			index++
			continue
		}
		group := []*richdoc.Node{node}
		next := index + 1
		for next < len(nodes) && nodes[next] != nil && linkTarget(nodes[next]) == href {
			group = append(group, nodes[next])
			next++
		}
		index = next
		relID := b.relationship("http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink", href, "External")
		var inner strings.Builder
		linked := base
		linked.color = "1155CC"
		linked.underline = true
		for _, item := range group {
			inner.WriteString(b.inlineNode(item, linked))
		}
		out.WriteString(`<w:hyperlink` + attr("r:id", relID) + `>` + inner.String() + `</w:hyperlink>`)
	}
	return out.String()
}

func linkTarget(node *richdoc.Node) string {
	mark, ok := node.Mark("link")
	if !ok {
		return ""
	}
	href := strings.TrimSpace(mark.AttrString("href"))
	lower := strings.ToLower(href)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "mailto:") {
		return href
	}
	return ""
}

func (b *builder) inlineNode(node *richdoc.Node, base runStyle) string {
	switch node.Type {
	case "text":
		return b.textRun(node.Text, base.merge(node.Marks))
	case "hardBreak":
		return `<w:r><w:br/></w:r>`
	case "image":
		return b.imageRun(node)
	case richdoc.FootnoteType:
		return b.footnoteRun(node)
	default:
		if len(node.Content) > 0 {
			return b.inline(node.Content, base.merge(node.Marks))
		}
		return ""
	}
}

func (b *builder) imageRun(node *richdoc.Node) string {
	src := strings.TrimSpace(node.AttrString("src"))
	if src == "" || b.opts.ResolveImage == nil {
		return ""
	}
	relID, ok := b.mediaBySrc[src]
	var width, height int
	if !ok {
		picture, found := b.opts.ResolveImage(src)
		if !found || len(picture.Data) == 0 {
			return ""
		}
		extension := imageExtension(picture.MediaType, picture.Data)
		name := fmt.Sprintf("image%d.%s", len(b.media)+1, extension)
		b.media = append(b.media, mediaPart{name: name, data: picture.Data})
		b.extensions = append(b.extensions, extension)
		relID = b.relationship("http://schemas.openxmlformats.org/officeDocument/2006/relationships/image", "media/"+name, "")
		b.mediaBySrc[src] = relID
		width, height = imageSize(picture.Data)
		b.mediaSizes(src, width, height)
	}
	width, height = b.sizeFor(src)
	if width <= 0 || height <= 0 {
		width, height = 600, 400
	}
	if attrWidth := node.AttrInt("width", 0); attrWidth > 0 {
		if attrHeight := node.AttrInt("height", 0); attrHeight > 0 {
			width, height = attrWidth, attrHeight
		} else {
			height = int(float64(height) * float64(attrWidth) / float64(width))
			width = attrWidth
		}
	}
	cx := width * emuPerPixel
	cy := height * emuPerPixel
	// An image is shrunk to the text column, which is wider on a page that has
	// been turned. Measuring against the portrait column would keep a picture
	// small on a landscape page for no reason.
	available := b.contentWidth() * emuPerTwip
	if cx > available {
		cy = int(float64(cy) * float64(available) / float64(cx))
		cx = available
	}
	if cy < 1 {
		cy = 1
	}
	docPrID := b.nextDocPrID
	b.nextDocPrID++
	alt := node.AttrString("alt")
	if alt == "" {
		alt = node.AttrString("title")
	}
	return `<w:r><w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0">` +
		`<wp:extent` + intAttr("cx", cx) + intAttr("cy", cy) + `/>` +
		`<wp:effectExtent l="0" t="0" r="0" b="0"/>` +
		`<wp:docPr` + intAttr("id", docPrID) + attr("name", "Picture "+strconv.Itoa(docPrID)) + attr("descr", alt) + `/>` +
		`<wp:cNvGraphicFramePr><a:graphicFrameLocks xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" noChangeAspect="1"/></wp:cNvGraphicFramePr>` +
		`<a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">` +
		`<a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture">` +
		`<pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture">` +
		`<pic:nvPicPr><pic:cNvPr` + intAttr("id", docPrID) + attr("name", "Picture "+strconv.Itoa(docPrID)) + attr("descr", alt) + `/><pic:cNvPicPr/></pic:nvPicPr>` +
		`<pic:blipFill><a:blip` + attr("r:embed", relID) + `/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>` +
		`<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext` + intAttr("cx", cx) + intAttr("cy", cy) + `/></a:xfrm>` +
		`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr>` +
		`</pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`
}

func (b *builder) mediaSizes(src string, width, height int) {
	if b.sizes == nil {
		b.sizes = map[string][2]int{}
	}
	b.sizes[src] = [2]int{width, height}
}

func (b *builder) sizeFor(src string) (int, int) {
	if b.sizes == nil {
		return 0, 0
	}
	size := b.sizes[src]
	return size[0], size[1]
}

func imageSize(data []byte) (int, int) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return 600, 400
	}
	return config.Width, config.Height
}

func imageExtension(mediaType string, data []byte) string {
	switch strings.ToLower(strings.TrimSpace(strings.SplitN(mediaType, ";", 2)[0])) {
	case "image/png":
		return "png"
	case "image/jpeg", "image/jpg":
		return "jpeg"
	case "image/gif":
		return "gif"
	case "image/bmp":
		return "bmp"
	case "image/tiff":
		return "tiff"
	case "image/webp":
		return "webp"
	}
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG")):
		return "png"
	case bytes.HasPrefix(data, []byte("\xff\xd8\xff")):
		return "jpeg"
	case bytes.HasPrefix(data, []byte("GIF8")):
		return "gif"
	default:
		return "png"
	}
}

// DecodeDataURI extracts the payload of a data: URI used as an image source.
func DecodeDataURI(value string) (Image, bool) {
	if !strings.HasPrefix(value, "data:") {
		return Image{}, false
	}
	comma := strings.Index(value, ",")
	if comma < 0 {
		return Image{}, false
	}
	meta := value[5:comma]
	payload := value[comma+1:]
	mediaType := strings.SplitN(meta, ";", 2)[0]
	if !strings.Contains(meta, "base64") {
		return Image{}, false
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(payload))
	if err != nil || len(data) == 0 {
		return Image{}, false
	}
	return Image{Data: data, MediaType: mediaType}, true
}

// footnoteRun writes the little number in the sentence and files the note
// itself away to be written into word/footnotes.xml.
//
// Without this the default branch below would walk into the note and splice
// its text into the middle of the sentence it annotates, which is how a
// footnote reads when nobody has taught the exporter what one is.
func (b *builder) footnoteRun(node *richdoc.Node) string {
	// Word reserves 0 and 1 for the separator lines it draws above the notes.
	id := len(b.footnotes) + 2
	b.footnotes = append(b.footnotes, footnoteEntry{
		id:   id,
		text: richdoc.FootnoteText(richdoc.Footnote{Content: node.Content}),
	})
	// The reference is a superscript run. muni's styles.xml has no
	// FootnoteReference style, and pointing at a style that is not there is
	// the kind of thing Word tolerates until it does not.
	return `<w:r><w:rPr><w:vertAlign w:val="superscript"/></w:rPr>` +
		`<w:footnoteReference` + intAttr("w:id", id) + `/></w:r>`
}

// footnoteEntry is one note waiting to be written.
type footnoteEntry struct {
	id   int
	text string
}

// footnotesPart builds word/footnotes.xml.
//
// The first two entries are the separator and continuation separator: Word
// draws the rule above the notes from them, and a file without them shows the
// notes with nothing dividing them from the body.
func (b *builder) footnotesPart() string {
	var out strings.Builder
	out.WriteString(xmlHeader + `<w:footnotes` + documentNamespaces + `>`)
	out.WriteString(`<w:footnote w:type="separator" w:id="0"><w:p><w:pPr><w:spacing w:after="0" w:line="240" w:lineRule="auto"/></w:pPr><w:r><w:separator/></w:r></w:p></w:footnote>`)
	out.WriteString(`<w:footnote w:type="continuationSeparator" w:id="1"><w:p><w:pPr><w:spacing w:after="0" w:line="240" w:lineRule="auto"/></w:pPr><w:r><w:continuationSeparator/></w:r></w:p></w:footnote>`)
	for _, note := range b.footnotes {
		out.WriteString(`<w:footnote` + intAttr("w:id", note.id) + `><w:p>` +
			`<w:r><w:rPr><w:vertAlign w:val="superscript"/></w:rPr><w:footnoteRef/></w:r>` +
			b.textRun(" "+note.text, runStyle{}) +
			`</w:p></w:footnote>`)
	}
	out.WriteString(`</w:footnotes>`)
	return out.String()
}
