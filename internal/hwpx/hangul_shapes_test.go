package hwpx

import (
	"archive/zip"
	"bytes"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/hangul"
	"github.com/hkjang/muni/internal/richdoc"
)

// What Hangul's own files taught, kept as tests. Each shape here was read
// off fifteen files Hangul wrote, after a fixture shaped to the reader had
// let the reader pass while every real file lost the thing in question.

const hangulHeader = `<?xml version="1.0" encoding="UTF-8"?>
<hh:head xmlns:hh="http://www.hancom.co.kr/hwpml/2011/head" xmlns:hp="http://www.hancom.co.kr/hwpml/2011/paragraph" xmlns:hc="http://www.hancom.co.kr/hwpml/2011/core" version="1.4" secCnt="1">
 <hh:refList>
  <hh:fontfaces itemCnt="2">
   <hh:fontface lang="HANGUL" fontCnt="2"><hh:font id="0" face="함초롬바탕" type="TTF"/><hh:font id="1" face="맑은 고딕" type="TTF"/></hh:fontface>
   <hh:fontface lang="LATIN" fontCnt="2"><hh:font id="0" face="Times New Roman" type="TTF"/><hh:font id="1" face="Arial" type="TTF"/></hh:fontface>
  </hh:fontfaces>
  <hh:borderFills itemCnt="4">
   <hh:borderFill id="1" threeD="0" shadow="0" centerLine="NONE" breakCellSeparateLine="0"><hh:slash type="NONE" Crooked="0" isCounter="0"/><hh:backSlash type="NONE" Crooked="0" isCounter="0"/><hh:leftBorder type="SOLID" width="0.12 mm" color="#000000"/><hh:rightBorder type="SOLID" width="0.12 mm" color="#000000"/><hh:topBorder type="SOLID" width="0.12 mm" color="#000000"/><hh:bottomBorder type="SOLID" width="0.12 mm" color="#000000"/><hh:diagonal type="SOLID" width="0.1 mm" color="#000000"/></hh:borderFill>
   <hh:borderFill id="2" threeD="0" shadow="0" centerLine="NONE" breakCellSeparateLine="0"><hh:slash type="NONE" Crooked="0" isCounter="0"/><hh:backSlash type="NONE" Crooked="0" isCounter="0"/><hh:leftBorder type="SOLID" width="0.12 mm" color="#000000"/><hh:rightBorder type="SOLID" width="0.12 mm" color="#000000"/><hh:topBorder type="SOLID" width="0.12 mm" color="#000000"/><hh:bottomBorder type="SOLID" width="0.12 mm" color="#000000"/><hh:diagonal type="SOLID" width="0.1 mm" color="#000000"/><hc:fillBrush><hc:winBrush faceColor="#D9E2F3" hatchColor="#333333" alpha="0"/></hc:fillBrush></hh:borderFill>
   <hh:borderFill id="3" threeD="0" shadow="0" centerLine="NONE" breakCellSeparateLine="0"><hh:slash type="NONE" Crooked="0" isCounter="0"/><hh:backSlash type="NONE" Crooked="0" isCounter="0"/><hh:leftBorder type="SOLID" width="0.12 mm" color="#000000"/><hh:rightBorder type="SOLID" width="0.12 mm" color="#000000"/><hh:topBorder type="SOLID" width="0.12 mm" color="#000000"/><hh:bottomBorder type="SOLID" width="0.12 mm" color="#000000"/><hh:diagonal type="SOLID" width="0.1 mm" color="#000000"/><hc:fillBrush><hc:winBrush faceColor="#FFFFFF" hatchColor="#333333" alpha="0"/></hc:fillBrush></hh:borderFill>
   <hh:borderFill id="4" threeD="0" shadow="0" centerLine="NONE" breakCellSeparateLine="0"><hh:slash type="NONE" Crooked="0" isCounter="0"/><hh:backSlash type="NONE" Crooked="0" isCounter="0"/><hh:leftBorder type="SOLID" width="0.12 mm" color="#000000"/><hh:rightBorder type="SOLID" width="0.12 mm" color="#000000"/><hh:topBorder type="SOLID" width="0.12 mm" color="#000000"/><hh:bottomBorder type="SOLID" width="0.12 mm" color="#000000"/><hh:diagonal type="SOLID" width="0.1 mm" color="#000000"/><hc:fillBrush><hc:gradation type="LINEAR" angle="90" centerX="0" centerY="0" step="50"><hc:color value="#FF0000"/><hc:color value="#0000FF"/></hc:gradation></hc:fillBrush></hh:borderFill>
  </hh:borderFills>
  <hh:charProperties itemCnt="6">
   <hh:charPr id="0" height="1000" shadeColor="none"><hh:fontRef hangul="0" latin="0"/></hh:charPr>
   <hh:charPr id="1" height="1000"><hh:fontRef hangul="1" latin="1"/></hh:charPr>
   <hh:charPr id="2" height="1000"><hh:fontRef hangul="0" latin="0"/><hh:supscript/></hh:charPr>
   <hh:charPr id="3" height="1000"><hh:fontRef hangul="0" latin="0"/><hh:subscript/></hh:charPr>
   <hh:charPr id="4" height="1000" shadeColor="#FFF3A3"><hh:fontRef hangul="0" latin="0"/></hh:charPr>
   <hh:charPr id="5" height="1000" shadeColor="#FFFFFF"><hh:fontRef hangul="0" latin="0"/></hh:charPr>
  </hh:charProperties>
  <hh:paraProperties itemCnt="4">
   <hh:paraPr id="0"><hh:align horizontal="LEFT"/><hh:heading type="NONE" idRef="0" level="0"/></hh:paraPr>
   <hh:paraPr id="1"><hh:align horizontal="LEFT"/><hh:heading type="NONE" idRef="0" level="0"/>
    <hp:switch><hp:case hp:required-namespace="http://www.hancom.co.kr/hwpml/2016/HwpUnitChar"><hh:margin><hc:intent value="1800" unit="HWPUNIT"/><hc:left value="3600" unit="HWPUNIT"/></hh:margin><hh:lineSpacing type="PERCENT" value="200" unit="HWPUNIT"/></hp:case>
    <hp:default><hh:margin><hc:intent value="1800"/><hc:left value="3600"/></hh:margin><hh:lineSpacing type="PERCENT" value="200"/></hp:default></hp:switch>
   </hh:paraPr>
   <hh:paraPr id="2"><hh:align horizontal="LEFT"/><hh:heading type="BULLET" idRef="1" level="0"/></hh:paraPr>
   <hh:paraPr id="3"><hh:align horizontal="LEFT"/><hh:heading type="NUMBER" idRef="1" level="1"/></hh:paraPr>
  </hh:paraProperties>
  <hh:styles itemCnt="1"><hh:style id="0" name="바탕글" engName="Normal" paraPrIDRef="0" charPrIDRef="0"/></hh:styles>
 </hh:refList>
</hh:head>`

func hangulFile(t *testing.T, body string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for name, data := range map[string]string{
		"mimetype":              "application/hwp+zip",
		"Contents/header.xml":   hangulHeader,
		"Contents/section0.xml": section(body),
	} {
		writer, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// A run names its font by its number in the header's table, not by name.
// Read as a name, every run in every Hangul file wore a font called "1".
func TestAFontIsNamedByItsNumberInTheTable(t *testing.T) {
	document, _, _, err := Parse(hangulFile(t, `<hp:p paraPrIDRef="0" styleIDRef="0"><hp:run charPrIDRef="1"><hp:t>고딕으로</hp:t></hp:run></hp:p>`))
	if err != nil {
		t.Fatal(err)
	}
	text := document.Content[0].Content[0]
	family := ""
	for _, mark := range text.Marks {
		if mark.Type == "textStyle" {
			family = mark.AttrString("fontFamily")
		}
	}
	if family != "맑은 고딕" {
		t.Errorf("글꼴 = %q", family)
	}
}

// Hangul keeps a paragraph's margins and line spacing inside a switch, one
// copy for a reader that knows the unit attribute and one for a reader that
// does not; a reader looking only at the paragraph's own children sees none.
func TestMarginsInsideASwitchAreRead(t *testing.T) {
	document, _, _, err := Parse(hangulFile(t, `<hp:p paraPrIDRef="1" styleIDRef="0"><hp:run charPrIDRef="0"><hp:t>들여쓴 문단</hp:t></hp:run></hp:p>`))
	if err != nil {
		t.Fatal(err)
	}
	paragraph := document.Content[0]
	if paragraph.AttrInt("indent", 0) != 2 {
		t.Errorf("들여쓰기 = %v", paragraph.Attr("indent"))
	}
	if first, _ := paragraph.Attr("firstLine").(bool); !first {
		t.Errorf("첫 줄 들여쓰기가 없습니다: %v", paragraph.Attrs)
	}
	if paragraph.AttrString("lineHeight") != "2" {
		t.Errorf("줄간격 = %q", paragraph.AttrString("lineHeight"))
	}
}

// The vertical alignment of a cell is on the cell's paragraph list: 1,294
// cells of Hangul's own put it there and none on the cell.
func TestVerticalAlignmentIsOnTheCellsList(t *testing.T) {
	document, _, _, err := Parse(hangulFile(t, `<hp:p paraPrIDRef="0" styleIDRef="0"><hp:run charPrIDRef="0"><hp:tbl rowCnt="1" colCnt="1"><hp:tr><hp:tc name="" header="0">`+
		`<hp:subList id="" vertAlign="CENTER"><hp:p paraPrIDRef="0" styleIDRef="0"><hp:run charPrIDRef="0"><hp:t>가운데</hp:t></hp:run></hp:p></hp:subList>`+
		`<hp:cellAddr colAddr="0" rowAddr="0"/><hp:cellSpan colSpan="1" rowSpan="1"/></hp:tc></hp:tr></hp:tbl></hp:run></hp:p>`))
	if err != nil {
		t.Fatal(err)
	}
	var cell *richdoc.Node
	for _, block := range document.Content {
		if block.Type == "table" {
			cell = block.Content[0].Content[0]
		}
	}
	if cell == nil {
		t.Fatalf("표가 없습니다: %v", blockTypes(document))
	}
	if cell.AttrString("verticalAlign") != "middle" {
		t.Errorf("세로 정렬 = %q", cell.AttrString("verticalAlign"))
	}
}

// 칸 음영 is not on the cell either. A cell names a borderFill by number and
// the header holds the brush that paints it — Hangul writes the colour there
// and nowhere else — so a reader that looked for a brush on the cell found one
// in no real file, and every shaded table came through white.
func TestACellsShadeIsReadFromTheBorderFillItNames(t *testing.T) {
	cell := func(column int, fill, text string) string {
		return `<hp:tc name="" header="0" borderFillIDRef="` + fill + `">` +
			`<hp:subList id="" vertAlign="TOP"><hp:p paraPrIDRef="0" styleIDRef="0"><hp:run charPrIDRef="0"><hp:t>` + text + `</hp:t></hp:run></hp:p></hp:subList>` +
			`<hp:cellAddr colAddr="` + strconv.Itoa(column) + `" rowAddr="0"/>` +
			`<hp:cellSpan colSpan="1" rowSpan="1"/><hp:cellSz width="1800" height="1900"/></hp:tc>`
	}
	document, _, _, err := Parse(hangulFile(t, `<hp:p paraPrIDRef="0" styleIDRef="0"><hp:run charPrIDRef="0"><hp:tbl rowCnt="1" colCnt="4"><hp:tr>`+
		cell(0, "2", "음영칸")+cell(1, "3", "흰칸")+cell(2, "1", "맨칸")+cell(3, "4", "그러데이션칸")+
		`</hp:tr></hp:tbl></hp:run></hp:p>`))
	if err != nil {
		t.Fatal(err)
	}
	var row *richdoc.Node
	for _, block := range document.Content {
		if block.Type == "table" {
			row = block.Content[0]
		}
	}
	if row == nil || len(row.Content) != 4 {
		t.Fatalf("표가 없습니다: %v", blockTypes(document))
	}
	if shade := row.Content[0].AttrString("backgroundColor"); shade != "#d9e2f3" {
		t.Errorf("음영 = %q", shade)
	}
	// White is what an unshaded cell is already drawn in, a fill that paints
	// no colour paints none, and a gradation is not a colour muni can hold.
	for index, name := range map[int]string{1: "흰칸", 2: "맨칸", 3: "그러데이션칸"} {
		if shade := row.Content[index].AttrString("backgroundColor"); shade != "" {
			t.Errorf("%s의 음영 = %q", name, shade)
		}
	}
}

// <hp:cellSz> is the width of the cell, not of the column: a merged cell
// gives the total across the columns it covers, so the columns are measured
// from the cells that cover one apiece. Reading a merged cell's own width as
// its column's made the first column of every merged table too wide.
func TestColumnWidthsAreReadFromTheCells(t *testing.T) {
	cell := func(column, span int, width int, text string) string {
		return `<hp:tc name="" header="0" borderFillIDRef="1">` +
			`<hp:subList id="" vertAlign="TOP"><hp:p paraPrIDRef="0" styleIDRef="0"><hp:run charPrIDRef="0"><hp:t>` + text + `</hp:t></hp:run></hp:p></hp:subList>` +
			`<hp:cellAddr colAddr="` + strconv.Itoa(column) + `" rowAddr="0"/>` +
			`<hp:cellSpan colSpan="` + strconv.Itoa(span) + `" rowSpan="1"/>` +
			`<hp:cellSz width="` + strconv.Itoa(width) + `" height="1900"/></hp:tc>`
	}
	const half, inchAndAHalf = hangul.UnitsPerInch / 2, hangul.UnitsPerInch * 3 / 2
	document, _, _, err := Parse(hangulFile(t, `<hp:p paraPrIDRef="0" styleIDRef="0"><hp:run charPrIDRef="0"><hp:tbl rowCnt="2" colCnt="2">`+
		`<hp:tr>`+cell(0, 2, half+inchAndAHalf, "머리글")+`</hp:tr>`+
		`<hp:tr>`+cell(0, 1, half, "좁은칸")+cell(1, 1, inchAndAHalf, "넓은칸")+`</hp:tr>`+
		`</hp:tbl></hp:run></hp:p>`))
	if err != nil {
		t.Fatal(err)
	}
	var table *richdoc.Node
	for _, block := range document.Content {
		if block.Type == "table" {
			table = block
		}
	}
	if table == nil || len(table.Content) != 2 {
		t.Fatalf("표가 없습니다: %v", blockTypes(document))
	}
	if width := table.Content[1].Content[0].Attr("colwidth"); !reflect.DeepEqual(width, []any{48}) {
		t.Errorf("좁은 칸의 너비 = %v", width)
	}
	if width := table.Content[0].Content[0].Attr("colwidth"); !reflect.DeepEqual(width, []any{48, 144}) {
		t.Errorf("병합된 칸의 너비 = %v", width)
	}
}

// A header or footer rides on the first paragraph as a control, with its
// own paragraph list; its words are the document's, not the body's.
func TestAHeaderAndFooterAreRead(t *testing.T) {
	document, _, meta, err := Parse(hangulFile(t, `<hp:p paraPrIDRef="0" styleIDRef="0"><hp:run charPrIDRef="0">`+
		`<hp:ctrl><hp:header id="0" applyPageType="BOTH"><hp:subList id="" vertAlign="TOP"><hp:p paraPrIDRef="0" styleIDRef="0"><hp:run charPrIDRef="0"><hp:t>머리말입니다</hp:t></hp:run></hp:p></hp:subList></hp:header></hp:ctrl>`+
		`<hp:ctrl><hp:footer id="0" applyPageType="BOTH"><hp:subList id="" vertAlign="BOTTOM"><hp:p paraPrIDRef="0" styleIDRef="0"><hp:run charPrIDRef="0"><hp:t>꼬리말입니다</hp:t></hp:run></hp:p></hp:subList></hp:footer></hp:ctrl>`+
		`<hp:t>본문</hp:t></hp:run></hp:p>`))
	if err != nil {
		t.Fatal(err)
	}
	if meta.Header != "머리말입니다" || meta.Footer != "꼬리말입니다" {
		t.Errorf("머리말/꼬리말 = %q / %q", meta.Header, meta.Footer)
	}
	if text := document.PlainText(); text != "본문" {
		t.Errorf("본문 = %q", text)
	}
}

// The raised and lowered runs are elements of the charPr, and the format
// spells the raised one "supscript" — looking for "superscript" finds it in
// no file Hangul wrote.
func TestRaisedAndLoweredRunsAreRead(t *testing.T) {
	document, _, _, err := Parse(hangulFile(t, `<hp:p paraPrIDRef="0" styleIDRef="0">`+
		`<hp:run charPrIDRef="2"><hp:t>위첨자</hp:t></hp:run>`+
		`<hp:run charPrIDRef="3"><hp:t>아래첨자</hp:t></hp:run>`+
		`<hp:run charPrIDRef="0"><hp:t>보통글</hp:t></hp:run></hp:p>`))
	if err != nil {
		t.Fatal(err)
	}
	if !hasMarkOn(document, "위첨자", "superscript") {
		t.Errorf("위 첨자가 오지 않았습니다: %v", document.Content[0].Content[0].Marks)
	}
	if !hasMarkOn(document, "아래첨자", "subscript") {
		t.Errorf("아래 첨자가 오지 않았습니다: %v", document.Content[0].Content[1].Marks)
	}
	if hasMarkOn(document, "보통글", "superscript") || hasMarkOn(document, "보통글", "subscript") {
		t.Errorf("첨자가 아닌 글에 첨자가 붙었습니다: %v", document.Content[0].Content[2].Marks)
	}
}

// 글자 음영 is an attribute of the charPr, not an element beside the bold and
// the italic, and Hangul writes it on every charPr it makes — "none" for the
// runs nobody marked, white for the ones a writer set to the paper's own
// colour. Reading the elements alone left every marked sentence unmarked,
// while the same sentence out of a .docx came through highlighted.
func TestARunKeepsTheShadeBehindItsWords(t *testing.T) {
	document, _, _, err := Parse(hangulFile(t, `<hp:p paraPrIDRef="0" styleIDRef="0">`+
		`<hp:run charPrIDRef="4"><hp:t>형광펜친글</hp:t></hp:run>`+
		`<hp:run charPrIDRef="5"><hp:t>흰바탕글</hp:t></hp:run>`+
		`<hp:run charPrIDRef="0"><hp:t>음영없는글</hp:t></hp:run></hp:p>`))
	if err != nil {
		t.Fatal(err)
	}
	if !hasMarkOn(document, "형광펜친글", "highlight") {
		t.Fatalf("음영이 오지 않았습니다: %v", document.Content[0].Content[0].Marks)
	}
	if color := markAttr(t, document, "형광펜친글", "highlight", "color"); color != "#FFF3A3" {
		t.Errorf("음영 색 = %q", color)
	}
	for _, phrase := range []string{"흰바탕글", "음영없는글"} {
		if hasMarkOn(document, phrase, "highlight") {
			t.Errorf("%q 에 음영이 붙었습니다", phrase)
		}
	}
}

// markAttr is one attribute of one kind of mark on the text holding a phrase.
func markAttr(t *testing.T, document *richdoc.Node, phrase, mark, name string) string {
	t.Helper()
	out := ""
	found := false
	var walk func(*richdoc.Node)
	walk = func(node *richdoc.Node) {
		if node == nil || found {
			return
		}
		if node.Type == "text" && strings.Contains(node.Text, phrase) {
			found = true
			for _, current := range node.Marks {
				if current.Type == mark {
					out = current.AttrString(name)
				}
			}
			return
		}
		for _, child := range node.Content {
			walk(child)
		}
	}
	walk(document)
	if !found {
		t.Fatalf("%q 를 찾지 못했습니다: %q", phrase, document.PlainText())
	}
	return out
}

// A list is a run of paragraphs whose shape says bullet or number and how
// deep, and nothing else marks it.
func TestBulletAndNumberShapesBecomeLists(t *testing.T) {
	document, _, _, err := Parse(hangulFile(t,
		`<hp:p paraPrIDRef="2" styleIDRef="0"><hp:run charPrIDRef="0"><hp:t>첫째</hp:t></hp:run></hp:p>`+
			`<hp:p paraPrIDRef="3" styleIDRef="0"><hp:run charPrIDRef="0"><hp:t>첫째의 하나</hp:t></hp:run></hp:p>`+
			`<hp:p paraPrIDRef="2" styleIDRef="0"><hp:run charPrIDRef="0"><hp:t>둘째</hp:t></hp:run></hp:p>`+
			`<hp:p paraPrIDRef="0" styleIDRef="0"><hp:run charPrIDRef="0"><hp:t>목록 뒤</hp:t></hp:run></hp:p>`))
	if err != nil {
		t.Fatal(err)
	}
	if types := blockTypes(document); len(types) != 2 || types[0] != "bulletList" || types[1] != "paragraph" {
		t.Fatalf("블록 = %v", types)
	}
	list := document.Content[0]
	if len(list.Content) != 2 {
		t.Fatalf("항목 = %d개", len(list.Content))
	}
	inner := list.Content[0].Content
	if len(inner) != 2 || inner[1].Type != "orderedList" || !strings.Contains(inner[1].PlainText(), "첫째의 하나") {
		t.Errorf("안쪽 목록 = %v", blockTypes(list.Content[0]))
	}
}
