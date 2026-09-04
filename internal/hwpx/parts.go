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
//
// Every shape here is copied from what Hangul itself writes, read off fifteen
// of its files rather than the specification — Hangul is the reader that
// matters, and it has never met a package that looks any other way.

const xmlHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

const (
	headNamespace      = "http://www.hancom.co.kr/hwpml/2011/head"
	paragraphNamespace = "http://www.hancom.co.kr/hwpml/2011/paragraph"
	sectionNamespace   = "http://www.hancom.co.kr/hwpml/2011/section"
	coreNamespace      = "http://www.hancom.co.kr/hwpml/2011/core"
)

// namespaces is the full set Hangul declares on every root it writes. Four of
// them are used here; the rest cost a line each and spare a loader the one
// root it has never seen.
const namespaces = `xmlns:ha="http://www.hancom.co.kr/hwpml/2011/app"` +
	` xmlns:hp="` + paragraphNamespace + `"` +
	` xmlns:hp10="http://www.hancom.co.kr/hwpml/2016/paragraph"` +
	` xmlns:hs="` + sectionNamespace + `"` +
	` xmlns:hc="` + coreNamespace + `"` +
	` xmlns:hh="` + headNamespace + `"` +
	` xmlns:hhs="http://www.hancom.co.kr/hwpml/2011/history"` +
	` xmlns:hm="http://www.hancom.co.kr/hwpml/2011/master-page"` +
	` xmlns:hpf="http://www.hancom.co.kr/schema/2011/hpf"` +
	` xmlns:dc="http://purl.org/dc/elements/1.1/"` +
	` xmlns:opf="http://www.idpf.org/2007/opf/"` +
	` xmlns:ooxmlchart="http://www.hancom.co.kr/hwpml/2016/ooxmlchart"` +
	` xmlns:hwpunitchar="http://www.hancom.co.kr/hwpml/2016/HwpUnitChar"` +
	` xmlns:epub="http://www.idpf.org/2007/ops"` +
	` xmlns:config="urn:oasis:names:tc:opendocument:xmlns:config:1.0"`

// The border definitions every file carries, by the numbers the body uses.
// Hangul's own first two are these — no lines round a paragraph, none round
// a character — and a table refers to the third.
const (
	paragraphBorder = 1
	characterBorder = 2
	tableBorder     = 3
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
		{"META-INF/container.rdf", []byte(containerRDF)},
		{"META-INF/manifest.xml", []byte(b.manifestXML())},
		{"Contents/content.hpf", []byte(b.contentHPF())},
		{"Contents/header.xml", []byte(b.headerXML())},
		{"Contents/section0.xml", []byte(b.sectionXML())},
		{"Preview/PrvText.txt", []byte(b.previewText())},
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

// The version Hangul stamps on what it writes today. A lower one names an
// older dialect, and a loader is entitled to read it as one.
const versionXML = xmlHeader +
	`<hv:HCFVersion xmlns:hv="http://www.hancom.co.kr/hwpml/2011/version" tagetApplication="WORDPROCESSOR" major="5" minor="1" micro="0" buildNumber="1" os="1" xmlVersion="1.4" application="muni" appVersion="1"/>`

const containerXML = xmlHeader +
	`<ocf:container xmlns:ocf="urn:oasis:names:tc:opendocument:xmlns:container" xmlns:hpf="http://www.hancom.co.kr/schema/2011/hpf">` +
	`<ocf:rootfiles>` +
	`<ocf:rootfile full-path="Contents/content.hpf" media-type="application/hwpml-package+xml"/>` +
	`<ocf:rootfile full-path="Preview/PrvText.txt" media-type="text/plain"/>` +
	`<ocf:rootfile full-path="META-INF/container.rdf" media-type="application/rdf+xml"/>` +
	`</ocf:rootfiles></ocf:container>`

// containerRDF says which part is the header and which the section, which
// the manifest already says; Hangul writes both and reads for both.
const containerRDF = xmlHeader +
	`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">` +
	`<rdf:Description rdf:about=""><ns0:hasPart xmlns:ns0="http://www.hancom.co.kr/hwpml/2016/meta/pkg#" rdf:resource="Contents/header.xml"/></rdf:Description>` +
	`<rdf:Description rdf:about="Contents/header.xml"><rdf:type rdf:resource="http://www.hancom.co.kr/hwpml/2016/meta/pkg#HeaderFile"/></rdf:Description>` +
	`<rdf:Description rdf:about=""><ns0:hasPart xmlns:ns0="http://www.hancom.co.kr/hwpml/2016/meta/pkg#" rdf:resource="Contents/section0.xml"/></rdf:Description>` +
	`<rdf:Description rdf:about="Contents/section0.xml"><rdf:type rdf:resource="http://www.hancom.co.kr/hwpml/2016/meta/pkg#SectionFile"/></rdf:Description>` +
	`<rdf:Description rdf:about=""><rdf:type rdf:resource="http://www.hancom.co.kr/hwpml/2016/meta/pkg#Document"/></rdf:Description>` +
	`</rdf:RDF>`

const settingsXML = xmlHeader +
	`<ha:HWPApplicationSetting xmlns:ha="http://www.hancom.co.kr/hwpml/2011/app" xmlns:config="urn:oasis:names:tc:opendocument:xmlns:config:1.0">` +
	`<ha:CaretPosition listIDRef="0" paraIDRef="0" pos="0"/></ha:HWPApplicationSetting>`

// previewText is the opening text of the document, which is what a file
// manager shows before the document is opened.
func (b *builder) previewText() string {
	return strings.TrimSpace(b.preview.String())
}

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
	out.WriteString(xmlHeader + `<opf:package ` + namespaces + ` version="" unique-identifier="" id="">`)
	out.WriteString(`<opf:metadata><opf:title>` + escape(b.opts.Title) + `</opf:title><opf:language>ko</opf:language>`)
	if !b.opts.Created.IsZero() {
		stamp := b.opts.Created.UTC().Format("2006-01-02T15:04:05Z")
		out.WriteString(`<opf:meta name="CreatedDate" content="text">` + stamp + `</opf:meta>`)
		out.WriteString(`<opf:meta name="ModifiedDate" content="text">` + stamp + `</opf:meta>`)
	}
	out.WriteString(`</opf:metadata>`)
	out.WriteString(`<opf:manifest>`)
	out.WriteString(`<opf:item id="header" href="Contents/header.xml" media-type="application/xml"/>`)
	out.WriteString(`<opf:item id="section0" href="Contents/section0.xml" media-type="application/xml"/>`)
	out.WriteString(`<opf:item id="settings" href="settings.xml" media-type="application/xml"/>`)
	for _, item := range b.binData {
		out.WriteString(`<opf:item id="` + item.id + `" href="BinData/` + item.id + "." + item.extension +
			`" media-type="` + escape(item.mediaType) + `" isEmbeded="1"/>`)
	}
	out.WriteString(`</opf:manifest><opf:spine><opf:itemref idref="header"/><opf:itemref idref="section0" linear="yes"/></opf:spine>`)
	out.WriteString(`</opf:package>`)
	return out.String()
}

// fontID numbers a face, handing out the next number the first time it is
// seen. A run refers to its font by this number, not by name.
func (b *builder) fontID(face string) int {
	for id, known := range b.fonts {
		if known == face {
			return id
		}
	}
	b.fonts = append(b.fonts, face)
	return len(b.fonts) - 1
}

// headerXML writes the shapes the body refers to by number, in the order the
// numbers were handed out.
func (b *builder) headerXML() string {
	// The runs are written first, so every face they use has its number
	// before the font table that lists them is written.
	charPrs := make([]string, len(b.charOrder))
	for id, key := range b.charOrder {
		charPrs[id] = b.charPrXML(id, key)
	}

	var out strings.Builder
	out.WriteString(xmlHeader + `<hh:head ` + namespaces + ` version="1.4" secCnt="1">`)
	out.WriteString(`<hh:beginNum page="1" footnote="1" endnote="1" pic="1" tbl="1" equation="1"/>`)
	out.WriteString(`<hh:refList>`)

	// The same faces serve every script; Hangul keeps one list per script
	// and a run names an entry in each.
	out.WriteString(`<hh:fontfaces itemCnt="7">`)
	for _, lang := range []string{"HANGUL", "LATIN", "HANJA", "JAPANESE", "OTHER", "SYMBOL", "USER"} {
		out.WriteString(`<hh:fontface lang="` + lang + `" fontCnt="` + strconv.Itoa(len(b.fonts)) + `">`)
		for id, face := range b.fonts {
			out.WriteString(`<hh:font id="` + strconv.Itoa(id) + `" face="` + escape(face) + `" type="TTF" isEmbedded="0">` +
				`<hh:typeInfo familyType="FCAT_GOTHIC" weight="6" proportion="9" contrast="0" strokeVariation="1" armStyle="1" letterform="1" midline="1" xHeight="1"/>` +
				`</hh:font>`)
		}
		out.WriteString(`</hh:fontface>`)
	}
	out.WriteString(`</hh:fontfaces>`)

	out.WriteString(`<hh:borderFills itemCnt="` + strconv.Itoa(tableBorder+len(b.cellFillOrder)) + `">`)
	out.WriteString(borderFillXML(paragraphBorder, "NONE", "0.1 mm", ""))
	out.WriteString(borderFillXML(characterBorder, "NONE", "0.1 mm", `<hc:fillBrush><hc:winBrush faceColor="none" hatchColor="#000000" alpha="0"/></hc:fillBrush>`))
	out.WriteString(borderFillXML(tableBorder, "SOLID", "0.12 mm", ""))
	// A shaded cell's borderFill draws the table's own lines and fills the
	// inside with the colour; the cell names it and says nothing else about
	// the colour, which is the only place HWPX keeps one. The brush is shaped
	// like the one Hangul writes, hatch colour and all — nothing hatches
	// without a hatch style, and a loader still expects the attribute.
	for index, shade := range b.cellFillOrder {
		out.WriteString(borderFillXML(tableBorder+1+index, "SOLID", "0.12 mm",
			`<hc:fillBrush><hc:winBrush faceColor="`+escape(shade)+`" hatchColor="#999999" alpha="0"/></hc:fillBrush>`))
	}
	out.WriteString(`</hh:borderFills>`)

	out.WriteString(`<hh:charProperties itemCnt="` + strconv.Itoa(len(charPrs)) + `">`)
	for _, charPr := range charPrs {
		out.WriteString(charPr)
	}
	out.WriteString(`</hh:charProperties>`)

	// Every paragraph shape names tab definition 0, so there is one.
	out.WriteString(`<hh:tabProperties itemCnt="1"><hh:tabPr id="0" autoTabLeft="0" autoTabRight="0"/></hh:tabProperties>`)

	// One numbering and one bullet, which every list refers to: a number
	// at each depth in the form Hangul's own default uses, and a dot.
	out.WriteString(`<hh:numberings itemCnt="1"><hh:numbering id="1" start="1">`)
	for level := 1; level <= 7; level++ {
		out.WriteString(`<hh:paraHead start="1" level="` + strconv.Itoa(level) + `" align="LEFT" useInstWidth="1" autoIndent="1" widthAdjust="0" textOffsetType="PERCENT" textOffset="50" numFormat="DIGIT" charPrIDRef="4294967295" checkable="0">^` + strconv.Itoa(level) + `.</hh:paraHead>`)
	}
	out.WriteString(`</hh:numbering></hh:numberings>`)
	out.WriteString(`<hh:bullets itemCnt="1"><hh:bullet id="1" char="●" useImage="0">` +
		`<hh:paraHead level="0" align="LEFT" useInstWidth="0" autoIndent="1" widthAdjust="0" textOffsetType="PERCENT" textOffset="50" numFormat="DIGIT" charPrIDRef="4294967295" checkable="0"/>` +
		`</hh:bullet></hh:bullets>`)

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
			`" engName="Outline ` + strconv.Itoa(level) + `" paraPrIDRef="0" charPrIDRef="0" nextStyleIDRef="` + strconv.Itoa(level) + `" langID="1042" lockForm="0"/>`)
	}
	out.WriteString(`</hh:styles>`)

	out.WriteString(`</hh:refList>`)
	out.WriteString(`<hh:compatibleDocument targetProgram="HWP201X"><hh:layoutCompatibility/></hh:compatibleDocument>`)
	out.WriteString(`<hh:docOption><hh:linkinfo path="" pageInherit="0" footnoteInherit="0"/></hh:docOption>`)
	out.WriteString(`<hh:trackchageConfig flags="0"/>`)
	out.WriteString(`</hh:head>`)
	return out.String()
}

func borderFillXML(id int, line, width, fill string) string {
	side := func(name string) string {
		return `<hh:` + name + ` type="` + line + `" width="` + width + `" color="#000000"/>`
	}
	return `<hh:borderFill id="` + strconv.Itoa(id) + `" threeD="0" shadow="0" centerLine="NONE" breakCellSeparateLine="0">` +
		`<hh:slash type="NONE" Crooked="0" isCounter="0"/><hh:backSlash type="NONE" Crooked="0" isCounter="0"/>` +
		side("leftBorder") + side("rightBorder") + side("topBorder") + side("bottomBorder") +
		`<hh:diagonal type="SOLID" width="0.1 mm" color="#000000"/>` + fill +
		`</hh:borderFill>`
}

func (b *builder) charPrXML(id int, key charKey) string {
	height := "1000"
	if key.size != "" {
		height = key.size
	}
	color := "#000000"
	if key.color != "" {
		color = strings.ToUpper(key.color)
	}
	family := "함초롬바탕"
	if key.mono {
		family = "D2Coding"
	}
	if key.family != "" {
		family = key.family
	}
	font := strconv.Itoa(b.fontID(family))
	// "none" is what Hangul writes for a run nothing is drawn behind, and it
	// is what a shade the reader would refuse comes back as.
	shade := "none"
	if key.shade != "" {
		shade = strings.ToUpper(key.shade)
	}
	var out strings.Builder
	out.WriteString(`<hh:charPr id="` + strconv.Itoa(id) + `" height="` + height + `" textColor="` + escape(color) +
		`" shadeColor="` + escape(shade) + `" useFontSpace="0" useKerning="0" symMark="NONE" borderFillIDRef="` + strconv.Itoa(characterBorder) + `">`)
	out.WriteString(`<hh:fontRef hangul="` + font + `" latin="` + font + `" hanja="` + font +
		`" japanese="` + font + `" other="` + font + `" symbol="` + font + `" user="` + font + `"/>`)
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
	// Last of the switches, and in this order: the format lists the outline,
	// shadow, emboss and engrave muni does not write between the strikeout and
	// these two, and Hangul reads the children in the order it declares them.
	switch key.script {
	case "superscript":
		out.WriteString(`<hh:supscript/>`)
	case "subscript":
		out.WriteString(`<hh:subscript/>`)
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
	firstLine := "0"
	if key.firstLine {
		firstLine = strconv.Itoa(indentUnits)
	}
	spacing := "160"
	if key.lineRate != "" {
		if ratio, err := strconv.ParseFloat(key.lineRate, 64); err == nil && ratio > 0 {
			spacing = strconv.Itoa(int(ratio * 100))
		}
	}
	// The margins and line spacing are written twice, in a switch: once for
	// a reader that knows the 2016 unit attribute and once for one that does
	// not. It is how Hangul writes them, and a loader reads whichever it is.
	metrics := `<hh:margin><hc:intent value="` + firstLine + `" unit="HWPUNIT"/><hc:left value="` + strconv.Itoa(key.indent) +
		`" unit="HWPUNIT"/><hc:right value="0" unit="HWPUNIT"/><hc:prev value="0" unit="HWPUNIT"/><hc:next value="0" unit="HWPUNIT"/></hh:margin>` +
		`<hh:lineSpacing type="PERCENT" value="` + spacing + `" unit="HWPUNIT"/>`

	var out strings.Builder
	out.WriteString(`<hh:paraPr id="` + strconv.Itoa(id) + `" tabPrIDRef="0" condense="0" fontLineHeight="0" snapToGrid="1" suppressLineNumbers="0" checked="0">`)
	out.WriteString(`<hh:align horizontal="` + align + `" vertical="BASELINE"/>`)
	switch key.list {
	case "bulletList":
		out.WriteString(`<hh:heading type="BULLET" idRef="1" level="` + strconv.Itoa(key.level) + `"/>`)
	case "orderedList":
		out.WriteString(`<hh:heading type="NUMBER" idRef="1" level="` + strconv.Itoa(key.level) + `"/>`)
	default:
		out.WriteString(`<hh:heading type="NONE" idRef="0" level="0"/>`)
	}
	out.WriteString(`<hh:breakSetting breakLatinWord="KEEP_WORD" breakNonLatinWord="KEEP_WORD" widowOrphan="0" keepWithNext="0" keepLines="0" pageBreakBefore="0" lineWrap="BREAK"/>`)
	out.WriteString(`<hh:autoSpacing eAsianEng="0" eAsianNum="0"/>`)
	out.WriteString(`<hp:switch><hp:case hp:required-namespace="http://www.hancom.co.kr/hwpml/2016/HwpUnitChar">` + metrics + `</hp:case>` +
		`<hp:default>` + metrics + `</hp:default></hp:switch>`)
	out.WriteString(`<hh:border borderFillIDRef="` + strconv.Itoa(paragraphBorder) + `" offsetLeft="0" offsetRight="0" offsetTop="0" offsetBottom="0" connect="0" ignoreMargin="0"/>`)
	out.WriteString(`</hh:paraPr>`)
	return out.String()
}

// furniture writes a header or footer as a control on the first paragraph,
// one line of text on every page, the way Hangul carries its own.
func (b *builder) furniture(kind, vertAlign, text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return `<hp:ctrl><hp:` + kind + ` id="0" applyPageType="BOTH">` + openSubList(vertAlign) +
		b.openParagraph(0, 0, false) + `<hp:run charPrIDRef="0"><hp:t>` + escape(text) + `</hp:t></hp:run></hp:p>` +
		`</hp:subList></hp:` + kind + `></hp:ctrl>`
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
	pageBorder := func(kind string) string {
		return `<hp:pageBorderFill type="` + kind + `" borderFillIDRef="` + strconv.Itoa(paragraphBorder) +
			`" textBorder="PAPER" headerInside="0" footerInside="0" fillArea="PAPER"><hp:offset left="1417" right="1417" top="1417" bottom="1417"/></hp:pageBorderFill>`
	}
	sectionDef := `<hp:secPr id="" textDirection="HORIZONTAL" spaceColumns="1134" tabStop="8000" tabStopVal="4000" tabStopUnit="HWPUNIT" outlineShapeIDRef="0" memoShapeIDRef="0" textVerticalWidthHead="0" masterPageCnt="0">` +
		`<hp:grid lineGrid="0" charGrid="0" wonggojiFormat="0"/>` +
		`<hp:startNum pageStartsOn="BOTH" page="0" pic="0" tbl="0" equation="0"/>` +
		`<hp:visibility hideFirstHeader="0" hideFirstFooter="0" hideFirstMasterPage="0" border="SHOW_ALL" fill="SHOW_ALL" hideFirstPageNum="0" hideFirstEmptyLine="0" showLineNumber="0"/>` +
		`<hp:lineNumberShape restartType="0" countBy="0" distance="0" startNumber="0"/>` +
		`<hp:pagePr landscape="` + landscape + `" width="` + strconv.Itoa(width) + `" height="` + strconv.Itoa(height) + `" gutterType="LEFT_ONLY">` +
		`<hp:margin header="4252" footer="4252" gutter="0" left="8504" right="8504" top="5668" bottom="4252"/></hp:pagePr>` +
		`<hp:footNotePr><hp:autoNumFormat type="DIGIT" userChar="" prefixChar="" suffixChar=")" supscript="0"/>` +
		`<hp:noteLine length="-1" type="SOLID" width="0.12 mm" color="#000000"/><hp:noteSpacing betweenNotes="284" belowLine="568" aboveLine="852"/>` +
		`<hp:numbering type="CONTINUOUS" newNum="1"/><hp:placement place="EACH_COLUMN" beneathText="0"/></hp:footNotePr>` +
		`<hp:endNotePr><hp:autoNumFormat type="DIGIT" userChar="" prefixChar="" suffixChar=")" supscript="0"/>` +
		`<hp:noteLine length="0" type="NONE" width="0.12 mm" color="#000000"/><hp:noteSpacing betweenNotes="0" belowLine="576" aboveLine="864"/>` +
		`<hp:numbering type="CONTINUOUS" newNum="1"/><hp:placement place="END_OF_DOCUMENT" beneathText="0"/></hp:endNotePr>` +
		pageBorder("BOTH") + pageBorder("EVEN") + pageBorder("ODD") +
		`</hp:secPr>` +
		`<hp:ctrl><hp:colPr id="" type="NEWSPAPER" layout="LEFT" colCount="1" sameSz="1" sameGap="0"/></hp:ctrl>` +
		b.furniture("header", "TOP", b.opts.Header) + b.furniture("footer", "BOTTOM", b.opts.Footer)

	var out strings.Builder
	out.WriteString(xmlHeader + `<hs:sec ` + namespaces + `>`)
	body := b.body.String()
	// The section definition rides on the first paragraph. If the document
	// begins with one, it goes in there; otherwise an empty paragraph carries
	// it, which is what Hangul does too.
	if strings.HasPrefix(body, "<hp:p ") {
		cut := strings.Index(body, ">") + 1
		out.WriteString(body[:cut] + `<hp:run charPrIDRef="0">` + sectionDef + `</hp:run>` + body[cut:])
	} else {
		out.WriteString(b.openParagraph(0, 0, false) + `<hp:run charPrIDRef="0">` + sectionDef + `</hp:run></hp:p>` + body)
	}
	out.WriteString(`</hs:sec>`)
	return out.String()
}
