package pdfx

import (
	"bytes"
	"context"
	"math"
	"strings"
)

type matrix [6]float64

var identityMatrix = matrix{1, 0, 0, 1, 0, 0}

func multiply(m, n matrix) matrix {
	return matrix{
		m[0]*n[0] + m[1]*n[2],
		m[0]*n[1] + m[1]*n[3],
		m[2]*n[0] + m[3]*n[2],
		m[2]*n[1] + m[3]*n[3],
		m[4]*n[0] + m[5]*n[2] + n[4],
		m[4]*n[1] + m[5]*n[3] + n[5],
	}
}

func translation(tx, ty float64) matrix {
	return matrix{1, 0, 0, 1, tx, ty}
}

type textItem struct {
	x      float64
	y      float64
	endX   float64
	size   float64
	text   string
	bold   bool
	italic bool
	mono   bool
}

type imageItem struct {
	x         float64
	y         float64
	width     float64
	height    float64
	data      []byte
	mediaType string
}

type pageContent struct {
	texts  []textItem
	images []imageItem
	width  float64
	height float64
}

type graphicsState struct {
	ctm matrix
}

type interpreter struct {
	doc        *Document
	ctx        context.Context
	page       *pageContent
	fonts      map[string]*fontInfo
	stack      []graphicsState
	state      graphicsState
	textMat    matrix
	lineMat    matrix
	font       *fontInfo
	fontKey    string
	fontSize   float64
	charSpace  float64
	wordSpace  float64
	horizontal float64
	leading    float64
	rise       float64
	render     int
	depth      int
	unmapped   int
	mapped     int
}

func (d *Document) renderPage(ctx context.Context, page Dict, rotate int, mediaBox [4]float64) *pageContent {
	width := mediaBox[2] - mediaBox[0]
	height := mediaBox[3] - mediaBox[1]
	base := translation(-mediaBox[0], -mediaBox[1])
	switch ((rotate % 360) + 360) % 360 {
	case 90:
		base = multiply(base, matrix{0, 1, -1, 0, height, 0})
		width, height = height, width
	case 180:
		base = multiply(base, matrix{-1, 0, 0, -1, width, height})
	case 270:
		base = multiply(base, matrix{0, -1, 1, 0, 0, width})
		width, height = height, width
	}
	content := &pageContent{width: width, height: height}
	machine := &interpreter{
		doc:        d,
		ctx:        ctx,
		page:       content,
		fonts:      map[string]*fontInfo{},
		state:      graphicsState{ctm: base},
		horizontal: 1,
	}
	machine.run(d.pageContentBytes(page), d.dict(page.get("Resources")))
	return content
}

func (d *Document) pageContentBytes(page Dict) []byte {
	var out bytes.Buffer
	switch contents := d.resolve(page.get("Contents")).(type) {
	case *Stream:
		if data, err := d.StreamData(contents); err == nil {
			out.Write(data)
		}
	case Array:
		for _, item := range contents {
			if stream, ok := d.resolve(item).(*Stream); ok {
				if data, err := d.StreamData(stream); err == nil {
					out.Write(data)
					out.WriteByte('\n')
				}
			}
		}
	}
	return out.Bytes()
}

func (m *interpreter) lookupFont(resources Dict, name string) *fontInfo {
	key := name
	if font, ok := m.fonts[key]; ok {
		return font
	}
	fonts := m.doc.dict(resources.get("Font"))
	font := m.doc.loadFont(m.doc.dict(fonts.get(Name(name))))
	m.fonts[key] = font
	return font
}

func (m *interpreter) run(content []byte, resources Dict) {
	if m.depth > 8 || len(content) == 0 {
		return
	}
	parser := newParser(content, 0)
	operands := make([]Object, 0, 8)
	steps := 0
	for {
		// Interpreting a content stream is the expensive part of an import;
		// check the deadline periodically so a crafted file cannot hold a
		// request open indefinitely.
		if steps++; steps&1023 == 0 && m.ctx != nil && m.ctx.Err() != nil {
			return
		}
		current := parser.read()
		if current.kind == tokenEOF {
			return
		}
		if current.kind != tokenKeyword {
			parser.unread(current)
			value, err := parser.object(0)
			if err != nil {
				return
			}
			if len(operands) < 64 {
				operands = append(operands, value)
			}
			continue
		}
		m.operator(current.text, operands, resources, parser)
		operands = operands[:0]
	}
}

func numberAt(operands []Object, index int) float64 {
	if index < 0 || index >= len(operands) {
		return 0
	}
	value, _ := toFloat(operands[index])
	return value
}

func (m *interpreter) operator(name string, operands []Object, resources Dict, parser *parser) {
	switch name {
	case "q":
		m.stack = append(m.stack, m.state)
	case "Q":
		if len(m.stack) > 0 {
			m.state = m.stack[len(m.stack)-1]
			m.stack = m.stack[:len(m.stack)-1]
		}
	case "cm":
		if len(operands) >= 6 {
			m.state.ctm = multiply(matrix{numberAt(operands, 0), numberAt(operands, 1), numberAt(operands, 2), numberAt(operands, 3), numberAt(operands, 4), numberAt(operands, 5)}, m.state.ctm)
		}
	case "BT":
		m.textMat, m.lineMat = identityMatrix, identityMatrix
	case "ET":
	case "Tf":
		if len(operands) >= 2 {
			if fontName, ok := operands[0].(Name); ok {
				m.font = m.lookupFont(resources, string(fontName))
				m.fontKey = string(fontName)
			}
			m.fontSize = numberAt(operands, 1)
		}
	case "Td":
		m.lineMat = multiply(translation(numberAt(operands, 0), numberAt(operands, 1)), m.lineMat)
		m.textMat = m.lineMat
	case "TD":
		m.leading = -numberAt(operands, 1)
		m.lineMat = multiply(translation(numberAt(operands, 0), numberAt(operands, 1)), m.lineMat)
		m.textMat = m.lineMat
	case "Tm":
		if len(operands) >= 6 {
			m.lineMat = matrix{numberAt(operands, 0), numberAt(operands, 1), numberAt(operands, 2), numberAt(operands, 3), numberAt(operands, 4), numberAt(operands, 5)}
			m.textMat = m.lineMat
		}
	case "T*":
		m.nextLine()
	case "TL":
		m.leading = numberAt(operands, 0)
	case "Tc":
		m.charSpace = numberAt(operands, 0)
	case "Tw":
		m.wordSpace = numberAt(operands, 0)
	case "Tz":
		if value := numberAt(operands, 0); value != 0 {
			m.horizontal = value / 100
		}
	case "Ts":
		m.rise = numberAt(operands, 0)
	case "Tr":
		m.render = int(numberAt(operands, 0))
	case "Tj":
		if len(operands) >= 1 {
			if value, ok := operands[len(operands)-1].(String); ok {
				m.show(Array{value})
			}
		}
	case "TJ":
		if len(operands) >= 1 {
			if array, ok := operands[len(operands)-1].(Array); ok {
				m.show(array)
			}
		}
	case "'":
		m.nextLine()
		if len(operands) >= 1 {
			if value, ok := operands[len(operands)-1].(String); ok {
				m.show(Array{value})
			}
		}
	case `"`:
		if len(operands) >= 3 {
			m.wordSpace = numberAt(operands, 0)
			m.charSpace = numberAt(operands, 1)
			m.nextLine()
			if value, ok := operands[2].(String); ok {
				m.show(Array{value})
			}
		}
	case "Do":
		if len(operands) >= 1 {
			if xobject, ok := operands[0].(Name); ok {
				m.doXObject(string(xobject), resources)
			}
		}
	case "BI":
		skipInlineImage(parser)
	}
}

func (m *interpreter) nextLine() {
	m.lineMat = multiply(translation(0, -m.leading), m.lineMat)
	m.textMat = m.lineMat
}

// skipInlineImage jumps past a BI/ID/EI block whose binary payload would
// otherwise be lexed as garbage operators.
func skipInlineImage(parser *parser) {
	data := parser.data
	index := parser.pos
	for index+1 < len(data) {
		if data[index] == 'I' && data[index+1] == 'D' {
			index += 2
			break
		}
		index++
	}
	for index+1 < len(data) {
		if data[index] == 'E' && data[index+1] == 'I' &&
			(index == 0 || isWhitespace(data[index-1])) &&
			(index+2 >= len(data) || isWhitespace(data[index+2]) || isDelimiter(data[index+2])) {
			index += 2
			break
		}
		index++
	}
	parser.pos = index
	parser.peeked = nil
}

// maxTextRuns bounds the work a single page can cause; generated PDFs
// occasionally contain millions of tiny runs.
const maxTextRuns = 200000

func (m *interpreter) show(items Array) {
	if m.font == nil || m.render == 7 || len(m.page.texts) >= maxTextRuns {
		return
	}
	combined := multiply(m.textMat, m.state.ctm)
	size := m.fontSize * math.Hypot(combined[2], combined[3])
	if size <= 0 {
		size = math.Abs(m.fontSize)
	}
	startX, startY := combined[4], combined[5]

	var builder strings.Builder
	for _, item := range items {
		switch typed := item.(type) {
		case float64, int64:
			value, _ := toFloat(typed)
			shift := -value / 1000 * m.fontSize * m.horizontal
			m.textMat = multiply(translation(shift, 0), m.textMat)
			if value <= -100 && builder.Len() > 0 && !strings.HasSuffix(builder.String(), " ") {
				builder.WriteString(" ")
			}
		case String:
			for _, code := range m.font.decode(typed) {
				text, ok := m.font.text(code)
				if ok {
					m.mapped++
					builder.WriteString(text)
				} else {
					m.unmapped++
				}
				advance := (m.font.width(code)*m.fontSize + m.charSpace)
				if m.font.isSpace(code) {
					advance += m.wordSpace
				}
				m.textMat = multiply(translation(advance*m.horizontal, 0), m.textMat)
			}
		}
	}
	text := builder.String()
	if strings.TrimSpace(text) == "" {
		return
	}
	end := multiply(m.textMat, m.state.ctm)
	m.page.texts = append(m.page.texts, textItem{
		x:      startX,
		y:      startY,
		endX:   end[4],
		size:   size,
		text:   text,
		bold:   m.font.bold,
		italic: m.font.italic,
		mono:   m.font.mono,
	})
}

func (m *interpreter) doXObject(name string, resources Dict) {
	xobjects := m.doc.dict(resources.get("XObject"))
	if xobjects == nil {
		return
	}
	stream, ok := m.doc.resolve(xobjects.get(Name(name))).(*Stream)
	if !ok {
		return
	}
	switch m.doc.name(stream.Dict.get("Subtype")) {
	case "Form":
		saved := m.state
		savedFonts := m.fonts
		if form := m.doc.array(stream.Dict.get("Matrix")); len(form) == 6 {
			var transform matrix
			for index := 0; index < 6; index++ {
				transform[index], _ = toFloat(m.doc.resolve(form[index]))
			}
			m.state.ctm = multiply(transform, m.state.ctm)
		}
		formResources := m.doc.dict(stream.Dict.get("Resources"))
		if formResources == nil {
			formResources = resources
		}
		if data, err := m.doc.StreamData(stream); err == nil {
			m.depth++
			m.fonts = map[string]*fontInfo{}
			m.run(data, formResources)
			m.depth--
		}
		m.fonts = savedFonts
		m.state = saved
	case "Image":
		m.addImage(stream)
	}
}

func (m *interpreter) addImage(stream *Stream) {
	if len(m.page.images) >= 64 {
		return
	}
	data, mediaType, ok := m.doc.imageBytes(stream)
	if !ok {
		return
	}
	ctm := m.state.ctm
	width := math.Hypot(ctm[0], ctm[1])
	height := math.Hypot(ctm[2], ctm[3])
	if width < 24 || height < 24 {
		return
	}
	m.page.images = append(m.page.images, imageItem{
		x:         ctm[4],
		y:         ctm[5],
		width:     width,
		height:    height,
		data:      data,
		mediaType: mediaType,
	})
}
