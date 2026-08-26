package docx

import (
	"fmt"
	"strings"
)

const xmlHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

const documentNamespaces = ` xmlns:wpc="http://schemas.microsoft.com/office/word/2010/wordprocessingCanvas"` +
	` xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"` +
	` xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"` +
	` xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math"` +
	` xmlns:wp14="http://schemas.microsoft.com/office/word/2010/wordprocessingDrawing"` +
	` xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"` +
	` xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"` +
	` xmlns:w14="http://schemas.microsoft.com/office/word/2010/wordml"` +
	` xmlns:wpg="http://schemas.microsoft.com/office/word/2010/wordprocessingGroup"` +
	` xmlns:wpi="http://schemas.microsoft.com/office/word/2010/wordprocessingInk"` +
	` xmlns:wne="http://schemas.microsoft.com/office/word/2006/wordml"` +
	` xmlns:wps="http://schemas.microsoft.com/office/word/2010/wordprocessingShape"` +
	` mc:Ignorable="w14 wp14"`

const (
	latinFont    = "Calibri"
	eastAsiaFont = "맑은 고딕"
	monoFont     = "Consolas"
	monoEastAsia = "D2Coding"
)

func contentTypes(mediaExtensions []string, furniture []furniturePart) string {
	var defaults strings.Builder
	seen := map[string]bool{"rels": true, "xml": true}
	defaults.WriteString(`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`)
	defaults.WriteString(`<Default Extension="xml" ContentType="application/xml"/>`)
	for _, extension := range mediaExtensions {
		if seen[extension] {
			continue
		}
		seen[extension] = true
		defaults.WriteString(`<Default` + attr("Extension", extension) + attr("ContentType", mediaContentType(extension)) + `/>`)
	}
	return xmlHeader + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		defaults.String() +
		`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
		`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
		`<Override PartName="/word/numbering.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"/>` +
		`<Override PartName="/word/settings.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"/>` +
		`<Override PartName="/word/fontTable.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.fontTable+xml"/>` +
		`<Override PartName="/word/theme/theme1.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml"/>` +
		`<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>` +
		`<Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>` +
		furnitureOverrides(furniture) +
		`</Types>`
}

func mediaContentType(extension string) string {
	switch extension {
	case "png":
		return "image/png"
	case "jpeg", "jpg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "bmp":
		return "image/bmp"
	case "tiff", "tif":
		return "image/tiff"
	case "svg":
		return "image/svg+xml"
	case "webp":
		return "image/webp"
	case "emf":
		return "image/x-emf"
	case "wmf":
		return "image/x-wmf"
	default:
		return "application/octet-stream"
	}
}

func packageRels() string {
	return xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
		`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>` +
		`<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>` +
		`</Relationships>`
}

func coreProperties(title, author, created string) string {
	return xmlHeader + `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"` +
		` xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/"` +
		` xmlns:dcmitype="http://purl.org/dc/dcmitype/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
		`<dc:title>` + escapeXML(title) + `</dc:title>` +
		`<dc:creator>` + escapeXML(author) + `</dc:creator>` +
		`<cp:lastModifiedBy>` + escapeXML(author) + `</cp:lastModifiedBy>` +
		`<dcterms:created xsi:type="dcterms:W3CDTF">` + escapeXML(created) + `</dcterms:created>` +
		`<dcterms:modified xsi:type="dcterms:W3CDTF">` + escapeXML(created) + `</dcterms:modified>` +
		`</cp:coreProperties>`
}

func appProperties(generator string) string {
	return xmlHeader + `<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"` +
		` xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes">` +
		`<Application>` + escapeXML(generator) + `</Application><DocSecurity>0</DocSecurity><ScaleCrop>false</ScaleCrop>` +
		`<SharedDoc>false</SharedDoc><HyperlinksChanged>false</HyperlinksChanged><AppVersion>16.0000</AppVersion>` +
		`</Properties>`
}

func settingsPart() string {
	return xmlHeader + `<w:settings` + documentNamespaces + `>` +
		`<w:zoom w:percent="100"/><w:proofState w:spelling="clean" w:grammar="clean"/>` +
		`<w:defaultTabStop w:val="720"/><w:characterSpacingControl w:val="compressPunctuation"/>` +
		`<w:compat><w:compatSetting w:name="compatibilityMode" w:uri="http://schemas.microsoft.com/office/word" w:val="15"/></w:compat>` +
		`<w:themeFontLang w:val="en-US" w:eastAsia="ko-KR"/>` +
		`</w:settings>`
}

func fontTablePart() string {
	font := func(name, family, pitch string) string {
		return `<w:font` + attr("w:name", name) + `><w:family w:val="` + family + `"/><w:pitch w:val="` + pitch + `"/></w:font>`
	}
	return xmlHeader + `<w:fonts` + documentNamespaces + `>` +
		font(latinFont, "swiss", "variable") +
		font(eastAsiaFont, "swiss", "variable") +
		font(monoFont, "modern", "fixed") +
		font(monoEastAsia, "modern", "fixed") +
		font("Symbol", "roman", "variable") +
		font("Courier New", "modern", "fixed") +
		font("Wingdings", "decorative", "variable") +
		`</w:fonts>`
}

// themePart is intentionally minimal: Word requires the part to exist when
// styles reference theme fonts, and a stripped theme keeps the package small.
func themePart() string {
	scheme := func(tag string) string {
		return `<a:` + tag + `><a:latin typeface="` + latinFont + `"/><a:ea typeface="` + eastAsiaFont + `"/><a:cs typeface=""/></a:` + tag + `>`
	}
	return xmlHeader + `<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="muni">` +
		`<a:themeElements><a:clrScheme name="muni">` +
		`<a:dk1><a:sysClr val="windowText" lastClr="000000"/></a:dk1><a:lt1><a:sysClr val="window" lastClr="FFFFFF"/></a:lt1>` +
		`<a:dk2><a:srgbClr val="202124"/></a:dk2><a:lt2><a:srgbClr val="F3F4FA"/></a:lt2>` +
		`<a:accent1><a:srgbClr val="5151C6"/></a:accent1><a:accent2><a:srgbClr val="6B6BD6"/></a:accent2>` +
		`<a:accent3><a:srgbClr val="4A9E8F"/></a:accent3><a:accent4><a:srgbClr val="D08C34"/></a:accent4>` +
		`<a:accent5><a:srgbClr val="C25450"/></a:accent5><a:accent6><a:srgbClr val="7A7A8C"/></a:accent6>` +
		`<a:hlink><a:srgbClr val="1155CC"/></a:hlink><a:folHlink><a:srgbClr val="7B4FBF"/></a:folHlink>` +
		`</a:clrScheme><a:fontScheme name="muni"><a:majorFont>` + scheme("latin") + `</a:majorFont>` +
		`<a:minorFont>` + scheme("latin") + `</a:minorFont></a:fontScheme>` +
		`<a:fmtScheme name="muni"><a:fillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill>` +
		`<a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:fillStyleLst>` +
		`<a:lnStyleLst><a:ln w="6350"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln>` +
		`<a:ln w="12700"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln>` +
		`<a:ln w="19050"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln></a:lnStyleLst>` +
		`<a:effectStyleLst><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle>` +
		`<a:effectStyle><a:effectLst/></a:effectStyle></a:effectStyleLst>` +
		`<a:bgFillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill>` +
		`<a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:bgFillStyleLst></a:fmtScheme>` +
		`</a:themeElements></a:theme>`
}

func stylesPart() string {
	var out strings.Builder
	out.WriteString(xmlHeader + `<w:styles` + documentNamespaces + `>`)
	out.WriteString(`<w:docDefaults><w:rPrDefault><w:rPr>` +
		`<w:rFonts` + attr("w:ascii", latinFont) + attr("w:hAnsi", latinFont) + attr("w:eastAsia", eastAsiaFont) + attr("w:cs", latinFont) + `/>` +
		`<w:color w:val="202124"/><w:sz w:val="22"/><w:szCs w:val="22"/>` +
		`<w:lang w:val="en-US" w:eastAsia="ko-KR" w:bidi="ar-SA"/>` +
		`</w:rPr></w:rPrDefault><w:pPrDefault><w:pPr>` +
		`<w:spacing w:before="0" w:after="160" w:line="288" w:lineRule="auto"/>` +
		`</w:pPr></w:pPrDefault></w:docDefaults>`)

	style := func(kind, id, name string, def bool, body string) {
		out.WriteString(`<w:style` + attr("w:type", kind) + attr("w:styleId", id))
		if def {
			out.WriteString(` w:default="1"`)
		}
		out.WriteString(`><w:name` + attr("w:val", name) + `/>`)
		out.WriteString(body)
		out.WriteString(`</w:style>`)
	}

	style("paragraph", "Normal", "Normal", true, `<w:qFormat/>`)
	style("character", "DefaultParagraphFont", "Default Paragraph Font", true, `<w:uiPriority w:val="1"/><w:semiHidden/><w:unhideWhenUsed/>`)
	style("table", "TableNormal", "Normal Table", true, `<w:uiPriority w:val="99"/><w:semiHidden/><w:unhideWhenUsed/>`+
		`<w:tblPr><w:tblInd w:w="0" w:type="dxa"/><w:tblCellMar><w:top w:w="0" w:type="dxa"/><w:left w:w="108" w:type="dxa"/>`+
		`<w:bottom w:w="0" w:type="dxa"/><w:right w:w="108" w:type="dxa"/></w:tblCellMar></w:tblPr>`)
	style("numbering", "NoList", "No List", true, `<w:uiPriority w:val="99"/><w:semiHidden/><w:unhideWhenUsed/>`)

	style("paragraph", "Title", "Title", false,
		`<w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/>`+
			`<w:pPr><w:keepNext/><w:spacing w:before="0" w:after="320"/><w:contextualSpacing/></w:pPr>`+
			`<w:rPr><w:b/><w:color w:val="14142B"/><w:sz w:val="56"/><w:szCs w:val="56"/></w:rPr>`)

	headingSizes := []int{40, 34, 28, 25, 23, 22}
	for level := 1; level <= 6; level++ {
		id := fmt.Sprintf("Heading%d", level)
		before := 320
		if level > 2 {
			before = 240
		}
		body := `<w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/>` +
			`<w:pPr><w:keepNext/><w:keepLines/><w:spacing` + intAttr("w:before", before) + ` w:after="120"/>` +
			`<w:outlineLvl` + intAttr("w:val", level-1) + `/></w:pPr>` +
			`<w:rPr><w:b/><w:color w:val="14142B"/><w:sz` + intAttr("w:val", headingSizes[level-1]) + `/><w:szCs` + intAttr("w:val", headingSizes[level-1]) + `/></w:rPr>`
		style("paragraph", id, fmt.Sprintf("heading %d", level), false, body)
	}

	style("paragraph", "Quote", "Quote", false,
		`<w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/>`+
			`<w:pPr><w:pBdr><w:left w:val="single" w:sz="18" w:space="10" w:color="6B6BD6"/></w:pBdr>`+
			`<w:spacing w:before="120" w:after="120"/><w:ind w:left="360"/></w:pPr>`+
			`<w:rPr><w:i/><w:color w:val="555555"/></w:rPr>`)

	style("paragraph", "CodeBlock", "Code Block", false,
		`<w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/>`+
			`<w:pPr><w:pBdr><w:top w:val="single" w:sz="4" w:space="6" w:color="D8DAE5"/>`+
			`<w:left w:val="single" w:sz="4" w:space="6" w:color="D8DAE5"/>`+
			`<w:bottom w:val="single" w:sz="4" w:space="6" w:color="D8DAE5"/>`+
			`<w:right w:val="single" w:sz="4" w:space="6" w:color="D8DAE5"/></w:pBdr>`+
			`<w:shd w:val="clear" w:color="auto" w:fill="F5F6FA"/>`+
			`<w:spacing w:before="120" w:after="120" w:line="240" w:lineRule="auto"/></w:pPr>`+
			`<w:rPr><w:rFonts`+attr("w:ascii", monoFont)+attr("w:hAnsi", monoFont)+attr("w:eastAsia", monoEastAsia)+attr("w:cs", monoFont)+`/>`+
			`<w:sz w:val="20"/><w:szCs w:val="20"/></w:rPr>`)

	style("character", "CodeChar", "Code Char", false,
		`<w:basedOn w:val="DefaultParagraphFont"/><w:qFormat/>`+
			`<w:rPr><w:rFonts`+attr("w:ascii", monoFont)+attr("w:hAnsi", monoFont)+attr("w:eastAsia", monoEastAsia)+attr("w:cs", monoFont)+`/>`+
			`<w:color w:val="A6314C"/><w:sz w:val="20"/><w:szCs w:val="20"/>`+
			`<w:shd w:val="clear" w:color="auto" w:fill="F2F2F7"/></w:rPr>`)

	style("character", "Hyperlink", "Hyperlink", false,
		`<w:basedOn w:val="DefaultParagraphFont"/><w:uiPriority w:val="99"/><w:unhideWhenUsed/>`+
			`<w:rPr><w:color w:val="1155CC"/><w:u w:val="single"/></w:rPr>`)

	style("paragraph", "ListParagraph", "List Paragraph", false,
		`<w:basedOn w:val="Normal"/><w:uiPriority w:val="34"/><w:qFormat/>`+
			`<w:pPr><w:spacing w:after="60"/><w:ind w:left="720"/><w:contextualSpacing/></w:pPr>`)

	style("paragraph", "Caption", "caption", false,
		`<w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/>`+
			`<w:pPr><w:spacing w:before="0" w:after="200"/></w:pPr>`+
			`<w:rPr><w:i/><w:color w:val="6B6B7B"/><w:sz w:val="18"/><w:szCs w:val="18"/></w:rPr>`)

	style("table", "TableGrid", "Table Grid", false,
		`<w:basedOn w:val="TableNormal"/><w:uiPriority w:val="39"/>`+
			`<w:pPr><w:spacing w:before="40" w:after="40" w:line="240" w:lineRule="auto"/></w:pPr>`+
			`<w:tblPr><w:tblBorders>`+
			`<w:top w:val="single" w:sz="4" w:space="0" w:color="C7C9D1"/>`+
			`<w:left w:val="single" w:sz="4" w:space="0" w:color="C7C9D1"/>`+
			`<w:bottom w:val="single" w:sz="4" w:space="0" w:color="C7C9D1"/>`+
			`<w:right w:val="single" w:sz="4" w:space="0" w:color="C7C9D1"/>`+
			`<w:insideH w:val="single" w:sz="4" w:space="0" w:color="C7C9D1"/>`+
			`<w:insideV w:val="single" w:sz="4" w:space="0" w:color="C7C9D1"/>`+
			`</w:tblBorders><w:tblCellMar><w:top w:w="60" w:type="dxa"/><w:left w:w="108" w:type="dxa"/>`+
			`<w:bottom w:w="60" w:type="dxa"/><w:right w:w="108" w:type="dxa"/></w:tblCellMar></w:tblPr>`)

	out.WriteString(`</w:styles>`)
	return out.String()
}

const (
	abstractBullet  = 0
	abstractOrdered = 1
)

// numberingPart declares one bullet and one ordered abstract list, then a
// concrete w:num per list found in the document so each ordered list restarts.
func numberingPart(instances []numInstance) string {
	var out strings.Builder
	out.WriteString(xmlHeader + `<w:numbering` + documentNamespaces + `>`)

	bulletGlyphs := []struct{ text, font string }{
		{"\uf0b7", "Symbol"}, {"o", "Courier New"}, {"\uf0a7", "Wingdings"},
		{"\uf0b7", "Symbol"}, {"o", "Courier New"}, {"\uf0a7", "Wingdings"},
		{"\uf0b7", "Symbol"}, {"o", "Courier New"}, {"\uf0a7", "Wingdings"},
	}
	out.WriteString(`<w:abstractNum` + intAttr("w:abstractNumId", abstractBullet) + `><w:multiLevelType w:val="hybridMultilevel"/>`)
	for level := 0; level < 9; level++ {
		glyph := bulletGlyphs[level]
		out.WriteString(`<w:lvl` + intAttr("w:ilvl", level) + `>` +
			`<w:start w:val="1"/><w:numFmt w:val="bullet"/>` +
			`<w:lvlText` + attr("w:val", glyph.text) + `/><w:lvlJc w:val="left"/>` +
			`<w:pPr><w:ind` + intAttr("w:left", listIndentTwips*(level+1)) + intAttr("w:hanging", listHangingTwips) + `/></w:pPr>` +
			`<w:rPr><w:rFonts` + attr("w:ascii", glyph.font) + attr("w:hAnsi", glyph.font) + attr("w:hint", "default") + `/></w:rPr>` +
			`</w:lvl>`)
	}
	out.WriteString(`</w:abstractNum>`)

	orderedFormats := []string{"decimal", "lowerLetter", "lowerRoman", "decimal", "lowerLetter", "lowerRoman", "decimal", "lowerLetter", "lowerRoman"}
	out.WriteString(`<w:abstractNum` + intAttr("w:abstractNumId", abstractOrdered) + `><w:multiLevelType w:val="hybridMultilevel"/>`)
	for level := 0; level < 9; level++ {
		out.WriteString(`<w:lvl` + intAttr("w:ilvl", level) + `>` +
			`<w:start w:val="1"/><w:numFmt` + attr("w:val", orderedFormats[level]) + `/>` +
			`<w:lvlText` + attr("w:val", fmt.Sprintf("%%%d.", level+1)) + `/><w:lvlJc w:val="left"/>` +
			`<w:pPr><w:ind` + intAttr("w:left", listIndentTwips*(level+1)) + intAttr("w:hanging", listHangingTwips) + `/></w:pPr>` +
			`</w:lvl>`)
	}
	out.WriteString(`</w:abstractNum>`)

	for _, instance := range instances {
		out.WriteString(`<w:num` + intAttr("w:numId", instance.id) + `><w:abstractNumId` + intAttr("w:val", instance.abstract) + `/>`)
		if instance.start > 1 {
			out.WriteString(`<w:lvlOverride w:ilvl="0"><w:startOverride` + intAttr("w:val", instance.start) + `/></w:lvlOverride>`)
		}
		out.WriteString(`</w:num>`)
	}
	out.WriteString(`</w:numbering>`)
	return out.String()
}

type numInstance struct {
	id       int
	abstract int
	start    int
}

// furnitureOverrides declares the header and footer parts. A part that is in
// the package but not declared here is one Word reports as corrupt.
func furnitureOverrides(furniture []furniturePart) string {
	var out strings.Builder
	for _, part := range furniture {
		out.WriteString(`<Override` + attr("PartName", "/word/"+part.name) +
			attr("ContentType", "application/vnd.openxmlformats-officedocument.wordprocessingml."+part.local+"+xml") + `/>`)
	}
	return out.String()
}
