// Package docx converts muni documents to and from WordprocessingML so that
// exported files keep their headings, lists, tables, images and inline styling
// instead of collapsing into flat paragraphs.
package docx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hkjang/muni/internal/richdoc"
)

// Image is a picture referenced by the document that should be embedded.
type Image struct {
	Data      []byte
	MediaType string
	Name      string
}

type Options struct {
	Title     string
	Author    string
	Generator string
	Created   time.Time
	// ResolveImage turns an image node's src into embeddable bytes. Returning
	// false drops the image rather than failing the whole export.
	ResolveImage func(src string) (Image, bool)
}

type relationship struct {
	id     string
	kind   string
	target string
	mode   string
}

type mediaPart struct {
	name string
	data []byte
}

type builder struct {
	opts        Options
	body        strings.Builder
	rels        []relationship
	media       []mediaPart
	mediaBySrc  map[string]string // src -> relationship id
	sizes       map[string][2]int // src -> intrinsic pixel size
	extensions  []string
	nums        []numInstance
	nextRelID   int
	nextDocPrID int
	nextNumID   int
}

type listContext struct {
	numID int
	level int
	kind  string
}

type blockContext struct {
	list    *listContext
	style   string
	indent  int
	inTable bool
	header  bool
}

// Build renders a document tree into a .docx package.
func Build(doc *richdoc.Node, opts Options) ([]byte, error) {
	if opts.Generator == "" {
		opts.Generator = "muni"
	}
	if opts.Created.IsZero() {
		opts.Created = time.Now().UTC()
	}
	b := &builder{opts: opts, mediaBySrc: map[string]string{}, nextRelID: 100, nextDocPrID: 1, nextNumID: 1}
	b.relationship("http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles", "styles.xml", "")
	b.relationship("http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering", "numbering.xml", "")
	b.relationship("http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings", "settings.xml", "")
	b.relationship("http://schemas.openxmlformats.org/officeDocument/2006/relationships/fontTable", "fontTable.xml", "")
	b.relationship("http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme", "theme/theme1.xml", "")

	if strings.TrimSpace(opts.Title) != "" {
		b.body.WriteString(`<w:p><w:pPr><w:pStyle w:val="Title"/></w:pPr>` + b.textRun(opts.Title, runStyle{}) + `</w:p>`)
	}
	if doc != nil {
		b.blocks(doc.Content, blockContext{})
	}
	if b.body.Len() == 0 {
		b.body.WriteString(`<w:p/>`)
	}
	b.body.WriteString(sectionProperties())

	return b.pack()
}

func sectionProperties() string {
	return `<w:sectPr><w:pgSz` + intAttr("w:w", pageWidthTwips) + intAttr("w:h", pageHeightTwips) + `/>` +
		`<w:pgMar` + intAttr("w:top", pageMarginTwips) + intAttr("w:right", pageMarginTwips) +
		intAttr("w:bottom", pageMarginTwips) + intAttr("w:left", pageMarginTwips) +
		` w:header="708" w:footer="708" w:gutter="0"/>` +
		`<w:cols w:space="708"/><w:docGrid w:linePitch="360"/></w:sectPr>`
}

func (b *builder) relationship(kind, target, mode string) string {
	id := "rId" + strconv.Itoa(b.nextRelID)
	b.nextRelID++
	b.rels = append(b.rels, relationship{id: id, kind: kind, target: target, mode: mode})
	return id
}

func (b *builder) pack() ([]byte, error) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	write := func(name string, data []byte) error {
		writer, err := archive.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: b.opts.Created})
		if err != nil {
			return err
		}
		_, err = writer.Write(data)
		return err
	}
	parts := []struct {
		name string
		data []byte
	}{
		{"[Content_Types].xml", []byte(contentTypes(b.extensions))},
		{"_rels/.rels", []byte(packageRels())},
		{"docProps/core.xml", []byte(coreProperties(b.opts.Title, b.opts.Author, b.opts.Created.Format(time.RFC3339)))},
		{"docProps/app.xml", []byte(appProperties(b.opts.Generator))},
		{"word/document.xml", []byte(xmlHeader + `<w:document` + documentNamespaces + `><w:body>` + b.body.String() + `</w:body></w:document>`)},
		{"word/_rels/document.xml.rels", []byte(b.documentRels())},
		{"word/styles.xml", []byte(stylesPart())},
		{"word/numbering.xml", []byte(numberingPart(b.nums))},
		{"word/settings.xml", []byte(settingsPart())},
		{"word/fontTable.xml", []byte(fontTablePart())},
		{"word/theme/theme1.xml", []byte(themePart())},
	}
	for _, part := range parts {
		if err := write(part.name, part.data); err != nil {
			return nil, err
		}
	}
	for _, media := range b.media {
		if err := write("word/media/"+media.name, media.data); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (b *builder) documentRels() string {
	var out strings.Builder
	out.WriteString(xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for _, rel := range b.rels {
		out.WriteString(`<Relationship` + attr("Id", rel.id) + attr("Type", rel.kind) + attr("Target", rel.target))
		if rel.mode != "" {
			out.WriteString(attr("TargetMode", rel.mode))
		}
		out.WriteString(`/>`)
	}
	out.WriteString(`</Relationships>`)
	return out.String()
}

func (b *builder) newList(kind string, start int) *listContext {
	abstract := abstractBullet
	if kind == "ordered" {
		abstract = abstractOrdered
	}
	id := b.nextNumID
	b.nextNumID++
	b.nums = append(b.nums, numInstance{id: id, abstract: abstract, start: start})
	return &listContext{numID: id, level: 0, kind: kind}
}

func (b *builder) blocks(nodes []*richdoc.Node, ctx blockContext) {
	for _, node := range nodes {
		b.block(node, ctx)
	}
}

func (b *builder) block(node *richdoc.Node, ctx blockContext) {
	if node == nil {
		return
	}
	switch node.Type {
	case "paragraph":
		b.paragraph(node, ctx)
	case "heading":
		level := node.AttrInt("level", 1)
		if level < 1 {
			level = 1
		}
		if level > 6 {
			level = 6
		}
		child := ctx
		child.style = fmt.Sprintf("Heading%d", level)
		b.paragraph(node, child)
	case "blockquote":
		child := ctx
		child.style = "Quote"
		child.list = nil
		if len(node.Content) == 0 {
			b.emitParagraph("", child, nil, "")
			return
		}
		b.blocks(node.Content, child)
	case "codeBlock":
		b.codeBlock(node, ctx)
	case "bulletList", "orderedList":
		kind := "bullet"
		if node.Type == "orderedList" {
			kind = "ordered"
		}
		b.list(node, ctx, kind)
	case "taskList":
		b.taskList(node, ctx, 0)
	case "horizontalRule":
		b.body.WriteString(`<w:p><w:pPr><w:pBdr><w:bottom w:val="single" w:sz="6" w:space="1" w:color="C7C9D1"/></w:pBdr>` +
			`<w:spacing w:before="160" w:after="160"/></w:pPr></w:p>`)
	case "table":
		b.table(node, ctx)
	case "image":
		run := b.imageRun(node)
		if run == "" {
			return
		}
		b.body.WriteString(`<w:p><w:pPr>` + b.paragraphPropertiesWithAlign(ctx, node.AttrString("textAlign")) + `</w:pPr>` + run + `</w:p>`)
	case "doc":
		b.blocks(node.Content, ctx)
	case "text":
		b.emitParagraph(``, ctx, []*richdoc.Node{node}, "")
	default:
		// Unknown block: keep its children rather than dropping content.
		if len(node.Content) > 0 {
			b.blocks(node.Content, ctx)
		}
	}
}

func (b *builder) paragraph(node *richdoc.Node, ctx blockContext) {
	b.emitParagraph(node.AttrString("textAlign"), ctx, node.Content, "")
}

func (b *builder) emitParagraph(align string, ctx blockContext, inline []*richdoc.Node, prefix string) {
	b.body.WriteString(`<w:p>`)
	if properties := b.paragraphPropertiesWithAlign(ctx, align); properties != "" {
		b.body.WriteString(`<w:pPr>` + properties + `</w:pPr>`)
	}
	if prefix != "" {
		b.body.WriteString(prefix)
	}
	b.body.WriteString(b.inline(inline, runStyle{bold: ctx.header}))
	b.body.WriteString(`</w:p>`)
}

// paragraphPropertiesWithAlign emits CT_PPr children in schema order; Word
// rejects the part outright when the sequence is wrong.
func (b *builder) paragraphPropertiesWithAlign(ctx blockContext, align string) string {
	var out strings.Builder
	style := ctx.style
	if style == "" && ctx.list != nil {
		style = "ListParagraph"
	}
	if style != "" {
		out.WriteString(`<w:pStyle` + attr("w:val", style) + `/>`)
	}
	if ctx.list != nil {
		out.WriteString(`<w:numPr><w:ilvl` + intAttr("w:val", ctx.list.level) + `/><w:numId` + intAttr("w:val", ctx.list.numID) + `/></w:numPr>`)
	}
	if ctx.inTable {
		out.WriteString(`<w:spacing w:before="40" w:after="40" w:line="252" w:lineRule="auto"/>`)
	}
	if ctx.indent > 0 {
		out.WriteString(`<w:ind` + intAttr("w:left", ctx.indent) + `/>`)
	}
	if jc := alignmentValue(align); jc != "" {
		out.WriteString(`<w:jc` + attr("w:val", jc) + `/>`)
	}
	return out.String()
}

func alignmentValue(align string) string {
	switch strings.ToLower(strings.TrimSpace(align)) {
	case "center":
		return "center"
	case "right", "end":
		return "right"
	case "justify":
		return "both"
	case "left", "start":
		return "left"
	default:
		return ""
	}
}

func (b *builder) codeBlock(node *richdoc.Node, ctx blockContext) {
	child := ctx
	child.style = "CodeBlock"
	child.list = nil
	text := node.PlainText()
	if text == "" {
		for _, item := range node.Content {
			text += item.Text
		}
	}
	var runs strings.Builder
	for index, line := range strings.Split(text, "\n") {
		if index > 0 {
			runs.WriteString(`<w:r><w:br/></w:r>`)
		}
		if line != "" {
			runs.WriteString(b.textRun(line, runStyle{}))
		}
	}
	b.body.WriteString(`<w:p><w:pPr>` + b.paragraphPropertiesWithAlign(child, "") + `</w:pPr>` + runs.String() + `</w:p>`)
}

func (b *builder) list(node *richdoc.Node, ctx blockContext, kind string) {
	list := ctx.list
	if list == nil || list.kind != kind {
		list = b.newList(kind, node.AttrInt("start", 1))
	} else {
		list = &listContext{numID: list.numID, level: list.level + 1, kind: kind}
	}
	if list.level > 8 {
		list.level = 8
	}
	for _, item := range node.Content {
		if item == nil {
			continue
		}
		if item.Type != "listItem" {
			b.block(item, ctx)
			continue
		}
		b.listItem(item, ctx, list)
	}
}

func (b *builder) listItem(item *richdoc.Node, ctx blockContext, list *listContext) {
	first := true
	blocks := item.Content
	if len(blocks) == 0 {
		blocks = []*richdoc.Node{richdoc.Paragraph()}
	}
	for _, child := range blocks {
		if child == nil {
			continue
		}
		switch child.Type {
		case "bulletList", "orderedList":
			nested := ctx
			nested.list = list
			kind := "bullet"
			if child.Type == "orderedList" {
				kind = "ordered"
			}
			// A nested list of a different kind starts its own numbering chain.
			if kind != list.kind {
				nested.list = nil
				nested.indent = listIndentTwips * (list.level + 1)
			}
			b.list(child, nested, kind)
		case "taskList":
			b.taskList(child, ctx, list.level+1)
		case "paragraph", "heading":
			inner := ctx
			if first {
				inner.list = list
			} else {
				inner.list = nil
				inner.style = "ListParagraph"
				inner.indent = listIndentTwips * (list.level + 1)
			}
			b.block(child, inner)
			first = false
		default:
			inner := ctx
			inner.list = nil
			inner.indent = listIndentTwips * (list.level + 1)
			b.block(child, inner)
			first = false
		}
	}
}

func (b *builder) taskList(node *richdoc.Node, ctx blockContext, level int) {
	for _, item := range node.Content {
		if item == nil {
			continue
		}
		if item.Type != "taskItem" {
			b.block(item, ctx)
			continue
		}
		glyph := "☐ "
		if item.AttrBool("checked") {
			glyph = "☒ "
		}
		child := ctx
		child.list = nil
		child.style = "ListParagraph"
		child.indent = listIndentTwips * (level + 1)
		first := true
		blocks := item.Content
		if len(blocks) == 0 {
			blocks = []*richdoc.Node{richdoc.Paragraph()}
		}
		for _, inner := range blocks {
			if inner == nil {
				continue
			}
			switch inner.Type {
			case "taskList":
				b.taskList(inner, ctx, level+1)
			case "bulletList", "orderedList":
				nested := child
				nested.indent = listIndentTwips * (level + 2)
				kind := "bullet"
				if inner.Type == "orderedList" {
					kind = "ordered"
				}
				b.list(inner, nested, kind)
			default:
				prefix := ""
				if first {
					prefix = b.textRun(glyph, runStyle{fontFamily: "Segoe UI Symbol"})
					first = false
				}
				b.emitParagraph(inner.AttrString("textAlign"), child, inner.Content, prefix)
			}
		}
	}
}
