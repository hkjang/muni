package hwpx

import (
	"archive/zip"
	"bytes"
	"strconv"
	"strings"
)

// The parts of a .hwpx package besides the body: the header the body refers
// to by number, the manifest that names every part, and the small files that
// say what kind of package this is.

const xmlHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

const (
	headNamespace      = "http://www.hancom.co.kr/hwpml/2011/head"
	paragraphNamespace = "http://www.hancom.co.kr/hwpml/2011/paragraph"
	sectionNamespace   = "http://www.hancom.co.kr/hwpml/2011/section"
	coreNamespace      = "http://www.hancom.co.kr/hwpml/2011/core"
)

// pack writes every part into a zip, the mimetype first and uncompressed, the
// way the format asks.
func (b *builder) pack() ([]byte, error) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)

	// The mimetype is stored rather than deflated, and first, so a reader can
	// tell what it has from the opening bytes without inflating anything.
	mime, err := archive.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	if err != nil {
		return nil, err
	}
	if _, err := mime.Write([]byte("application/hwp+zip")); err != nil {
		return nil, err
	}

	parts := []struct {
		name string
		data []byte
	}{
		{"version.xml", []byte(versionXML)},
		{"META-INF/container.xml", []byte(containerXML)},
		{"META-INF/manifest.xml", []byte(b.manifestXML())},
		{"Contents/content.hpf", []byte(b.contentHPF())},
		{"Contents/header.xml", []byte(b.headerXML())},
		{"Contents/section0.xml", []byte(b.sectionXML())},
		{"settings.xml", []byte(settingsXML)},
	}
	for _, item := range b.binData {
		parts = append(parts, struct {
			name string
			data []byte
		}{"BinData/" + item.id + "." + item.extension, item.data})
	}
	for _, part := range parts {
		writer, err := archive.CreateHeader(&zip.FileHeader{
			Name: part.name, Method: zip.Deflate, Modified: b.opts.Created,
		})
		if err != nil {
			return nil, err
		}
		if _, err := writer.Write(part.data); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

const versionXML = xmlHeader +
	`<hv:HCFVersion xmlns:hv="http://www.hancom.co.kr/hwpml/2011/version" tagetApplication="WORDPROCESSOR" major="5" minor="0" micro="5" buildNumber="0" os="1" xmlVersion="1.4" application="muni" appVersion="1"/>`

const containerXML = xmlHeader +
	`<ocf:container xmlns:ocf="urn:oasis:names:tc:opendocument:xmlns:container" xmlns:hpf="http://www.hancom.co.kr/schema/2011/hpf">` +
	`<ocf:rootfiles><ocf:rootfile full-path="Contents/content.hpf" media-type="application/hwpml-package+xml"/></ocf:rootfiles>` +
	`</ocf:container>`

const settingsXML = xmlHeader +
	`<ha:HWPApplicationSetting xmlns:ha="http://www.hancom.co.kr/hwpml/2011/app" xmlns:config="urn:oasis:names:tc:opendocument:xmlns:config:1.0">` +
	`<ha:CaretPosition listIDRef="0" paraIDRef="0" pos="0"/></ha:HWPApplicationSetting>`

func (b *builder) manifestXML() string {
	var out strings.Builder
	out.WriteString(xmlHeader + `<odf:manifest xmlns:odf="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0">`)
	out.WriteString(`<odf:file-entry odf:full-path="/" odf:media-type="application/hwp+zip"/>`)
	for _, name := range []string{"version.xml", "Contents/content.hpf", "Contents/header.xml", "Contents/section0.xml", "settings.xml"} {
		out.WriteString(`<odf:file-entry odf:full-path="` + name + `" odf:media-type="application/xml"/>`)
	}
	for _, item := range b.binData {
		out.WriteString(`<odf:file-entry odf:full-path="BinData/` + item.id + "." + item.extension +
			`" odf:media-type="` + escape(item.mediaType) + `"/>`)
	}
	out.WriteString(`</odf:manifest>`)
	return out.String()
}

// contentHPF is the package's own table of contents: what the parts are and
// the order the sections are read in.
func (b *builder) contentHPF() string {
	var out strings.Builder
	out.WriteString(xmlHeader + `<opf:package xmlns:opf="http://www.idpf.org/2007/opf/" version="" unique-identifier="" id="">`)
	out.WriteString(`<opf:metadata><opf:title>` + escape(b.opts.Title) + `</opf:title><opf:language>ko</opf:language></opf:metadata>`)
	out.WriteString(`<opf:manifest>`)
	out.WriteString(`<opf:item id="header" href="Contents/header.xml" media-type="application/xml"/>`)
	out.WriteString(`<opf:item id="section0" href="Contents/section0.xml" media-type="application/xml"/>`)
	out.WriteString(`<opf:item id="settings" href="settings.xml" media-type="application/xml"/>`)
	for _, item := range b.binData {
		out.WriteString(`<opf:item id="` + item.id + `" href="BinData/` + item.id + "." + item.extension +
			`" media-type="` + escape(item.mediaType) + `" isEmbeded="1"/>`)
	}
	out.WriteString(`</opf:manifest><opf:spine><opf:itemref idref="header" linear="yes"/><opf:itemref idref="section0" linear="yes"/></opf:spine>`)
	out.WriteString(`</opf:package>`)
	return out.String()
}

// headerXML writes the shapes the body refers to by number, in the order the
// numbers were handed out.
func (b *builder) headerXML() string {
	var out strings.Builder
	out.WriteString(xmlHeader + `<hh:head xmlns:hh="` + headNamespace + `" xmlns:hp="` + paragraphNamespace +
		`" xmlns:hc="` + coreNamespace + `" version="1.4" secCnt="1">`)
	out.WriteString(`<hh:beginNum page="1" footnote="1" endnote="1" pic="1" tbl="1" equation="1"/>`)
	out.WriteString(`<hh:refList>`)

	out.WriteString(`<hh:fontfaces itemCnt="7">`)
	for _, lang := range []string{"HANGUL", "LATIN", "HANJA", "JAPANESE", "OTHER", "SYMBOL", "USER"} {
		out.WriteString(`<hh:fontface lang="` + lang + `" fontCnt="1"><hh:font id="0" face="함초롬바탕" type="TTF" isEmbedded="0"/></hh:fontface>`)
	}
	out.WriteString(`</hh:fontfaces>`)

	// One border definition, for the tables: a thin line all round.
	out.WriteString(`<hh:borderFills itemCnt="1"><hh:borderFill id="1" threeD="0" shadow="0" centerLine="NONE" breakCellSeparateLine="0">` +
		`<hh:slash type="NONE" Crooked="0" isCounter="0"/><hh:backSlash type="NONE" Crooked="0" isCounter="0"/>` +
		`<hh:leftBorder type="SOLID" width="0.12 mm" color="#000000"/><hh:rightBorder type="SOLID" width="0.12 mm" color="#000000"/>` +
		`<hh:topBorder type="SOLID" width="0.12 mm" color="#000000"/><hh:bottomBorder type="SOLID" width="0.12 mm" color="#000000"/>` +
		`<hh:diagonal type="SOLID" width="0.1 mm" color="#000000"/></hh:borderFill></hh:borderFills>`)

	out.WriteString(`<hh:charProperties itemCnt="` + strconv.Itoa(len(b.charOrder)) + `">`)
	for id, key := range b.charOrder {
		out.WriteString(charPrXML(id, key))
	}
	out.WriteString(`</hh:charProperties>`)

	out.WriteString(`<hh:paraProperties itemCnt="` + strconv.Itoa(len(b.paraOrder)) + `">`)
	for id, key := range b.paraOrder {
		out.WriteString(paraPrXML(id, key))
	}
	out.WriteString(`</hh:paraProperties>`)

	// Style 0 is body text; 1 to 6 are the outline levels, named the way
	// Hangul names its own so a reader — muni's or Hangul's — knows them.
	out.WriteString(`<hh:styles itemCnt="7">`)
	out.WriteString(`<hh:style id="0" type="PARA" name="바탕글" engName="Normal" paraPrIDRef="0" charPrIDRef="0" nextStyleIDRef="0" langID="1042" lockForm="0"/>`)
	for level := 1; level <= 6; level++ {
		out.WriteString(`<hh:style id="` + strconv.Itoa(level) + `" type="PARA" name="개요 ` + strconv.Itoa(level) +
			`" engName="Outline ` + strconv.Itoa(level) + `" paraPrIDRef="0" charPrIDRef="0" nextStyleIDRef="0" langID="1042" lockForm="0"/>`)
	}
	out.WriteString(`</hh:styles>`)

	out.WriteString(`</hh:refList></hh:head>`)
	return out.String()
}

func charPrXML(id int, key charKey) string {
	height := "1000"
	if key.size != "" {
		height = key.size
	}
	color := "#000000"
	if key.color != "" {
		color = strings.ToUpper(key.color)
	}
	var out strings.Builder
	out.WriteString(`<hh:charPr id="` + strconv.Itoa(id) + `" height="` + height + `" textColor="` + escape(color) +
		`" shadeColor="none" useFontSpace="0" useKerning="0" symMark="NONE" borderFillIDRef="0">`)
	face := "0"
	_ = face
	family := "함초롬바탕"
	if key.mono {
		family = "D2Coding"
	}
	if key.family != "" {
		family = key.family
	}
	out.WriteString(`<hh:fontRef hangul="` + escape(family) + `" latin="` + escape(family) + `" hanja="` + escape(family) +
		`" japanese="` + escape(family) + `" other="` + escape(family) + `" symbol="` + escape(family) + `" user="` + escape(family) + `"/>`)
	out.WriteString(`<hh:ratio hangul="100" latin="100" hanja="100" japanese="100" other="100" symbol="100" user="100"/>`)
	out.WriteString(`<hh:spacing hangul="0" latin="0" hanja="0" japanese="0" other="0" symbol="0" user="0"/>`)
	out.WriteString(`<hh:relSz hangul="100" latin="100" hanja="100" japanese="100" other="100" symbol="100" user="100"/>`)
	out.WriteString(`<hh:offset hangul="0" latin="0" hanja="0" japanese="0" other="0" symbol="0" user="0"/>`)
	if key.bold {
		out.WriteString(`<hh:bold/>`)
	}
	if key.italic {
		out.WriteString(`<hh:italic/>`)
	}
	if key.underline {
		out.WriteString(`<hh:underline type="BOTTOM" shape="SOLID" color="#000000"/>`)
	}
	if key.strike {
		out.WriteString(`<hh:strikeout shape="SOLID" color="#000000"/>`)
	}
	out.WriteString(`</hh:charPr>`)
	return out.String()
}

func paraPrXML(id int, key paraKey) string {
	align := "JUSTIFY"
	switch key.align {
	case "center":
		align = "CENTER"
	case "right":
		align = "RIGHT"
	case "justify":
		align = "JUSTIFY"
	case "":
		align = "LEFT"
	}
	var out strings.Builder
	out.WriteString(`<hh:paraPr id="` + strconv.Itoa(id) + `" tabPrIDRef="0" condense="0" fontLineHeight="0" snapToGrid="1" suppressLineNumbers="0" checked="0">`)
	out.WriteString(`<hh:align horizontal="` + align + `" vertical="BASELINE"/>`)
	out.WriteString(`<hh:heading type="NONE" idRef="0" level="0"/>`)
	out.WriteString(`<hh:breakSetting breakLatinWord="KEEP_WORD" breakNonLatinWord="KEEP_WORD" widowOrphan="0" keepWithNext="0" keepLines="0" pageBreakBefore="0" lineWrap="BREAK"/>`)
	firstLine := "0"
	if key.firstLine {
		firstLine = strconv.Itoa(indentUnits)
	}
	out.WriteString(`<hh:margin><hc:intent value="` + firstLine + `" unit="HWPUNIT"/><hc:left value="` + strconv.Itoa(key.indent) +
		`" unit="HWPUNIT"/><hc:right value="0" unit="HWPUNIT"/><hc:prev value="0" unit="HWPUNIT"/><hc:next value="0" unit="HWPUNIT"/></hh:margin>`)
	spacing := "160"
	if key.lineRate != "" {
		if ratio, err := strconv.ParseFloat(key.lineRate, 64); err == nil && ratio > 0 {
			spacing = strconv.Itoa(int(ratio * 100))
		}
	}
	out.WriteString(`<hh:lineSpacing type="PERCENT" value="` + spacing + `" unit="HWPUNIT"/>`)
	out.WriteString(`<hh:border borderFillIDRef="0" offsetLeft="0" offsetRight="0" offsetTop="0" offsetBottom="0" connect="0" ignoreMargin="0"/>`)
	out.WriteString(`<hh:autoSpacing eAsianEng="0" eAsianNum="0"/>`)
	out.WriteString(`</hh:paraPr>`)
	return out.String()
}

// sectionXML wraps the body in the section element, with the paper it asks
// for on the first paragraph the way Hangul writes it.
func (b *builder) sectionXML() string {
	width, height := 59528, 84188 // A4 in HWPUNIT
	// "WIDELY" is what Hangul writes on a portrait page — real files say so
	// — and the reader goes by the dimensions either way. Written to match
	// what Hangul writes rather than what the word suggests.
	landscape := "WIDELY"
	if b.opts.Landscape {
		width, height = height, width
		landscape = "NARROWLY"
	}
	sectionDef := `<hp:secPr id="" textDirection="HORIZONTAL" spaceColumns="1134" tabStop="8000" tabStopVal="4000" tabStopUnit="HWPUNIT" outlineShapeIDRef="1" memoShapeIDRef="0" textVerticalWidthHead="0" masterPageCnt="0">` +
		`<hp:grid lineGrid="0" charGrid="0" wonggojiFormat="0"/>` +
		`<hp:startNum pageStartsOn="BOTH" page="0" pic="0" tbl="0" equation="0"/>` +
		`<hp:visibility hideFirstHeader="0" hideFirstFooter="0" hideFirstMasterPage="0" border="SHOW_ALL" fill="SHOW_ALL" hideFirstPageNum="0" hideFirstEmptyLine="0" showLineNumber="0"/>` +
		`<hp:lineNumberShape restartType="0" countBy="0" distance="0" startNumber="0"/>` +
		`<hp:pagePr landscape="` + landscape + `" width="` + strconv.Itoa(width) + `" height="` + strconv.Itoa(height) + `" gutterType="LEFT_ONLY">` +
		`<hp:margin header="4252" footer="4252" gutter="0" left="8504" right="8504" top="5668" bottom="4252"/></hp:pagePr>` +
		`</hp:secPr>`

	var out strings.Builder
	out.WriteString(xmlHeader + `<hs:sec xmlns:hs="` + sectionNamespace + `" xmlns:hp="` + paragraphNamespace +
		`" xmlns:hc="` + coreNamespace + `" xmlns:hh="` + headNamespace + `">`)
	body := b.body.String()
	// The section definition rides on the first paragraph. If the document
	// begins with one, it goes in there; otherwise an empty paragraph carries
	// it, which is what Hangul does too.
	if strings.HasPrefix(body, "<hp:p ") {
		cut := strings.Index(body, ">") + 1
		out.WriteString(body[:cut] + `<hp:run charPrIDRef="0">` + sectionDef + `</hp:run>` + body[cut:])
	} else {
		out.WriteString(`<hp:p paraPrIDRef="0" styleIDRef="0"><hp:run charPrIDRef="0">` + sectionDef + `</hp:run></hp:p>` + body)
	}
	out.WriteString(`</hs:sec>`)
	return out.String()
}
