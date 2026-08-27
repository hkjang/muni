package hwpx

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

// Every test here builds the file rather than reading one muni wrote, because
// muni does not write .hwpx at all. That is the only way to find out what muni
// makes of a file Hangul wrote — and the lesson from the Word importer, where
// every test fed the reader something the writer had produced and so could
// only find disagreements muni had with itself.

const headerXML = `<?xml version="1.0" encoding="UTF-8"?>
<hh:head xmlns:hh="http://www.hancom.co.kr/hwpml/2011/head">
 <hh:refList>
  <hh:charProperties itemCnt="3">
   <hh:charPr id="0" height="1000" textColor="#000000"><hh:fontRef hangul="함초롬바탕"/></hh:charPr>
   <hh:charPr id="1" height="1000"><hh:bold/><hh:underline type="BOTTOM"/></hh:charPr>
   <hh:charPr id="2" height="1400" textColor="#C00000"><hh:italic/><hh:fontRef hangul="맑은 고딕"/></hh:charPr>
  </hh:charProperties>
  <hh:paraProperties itemCnt="2">
   <hh:paraPr id="0"><hh:align horizontal="LEFT"/></hh:paraPr>
   <hh:paraPr id="1">
    <hh:align horizontal="CENTER"/>
    <hh:margin><hh:left value="3600"/><hh:indent value="1000"/></hh:margin>
    <hh:lineSpacing type="PERCENT" value="160"/>
   </hh:paraPr>
  </hh:paraProperties>
  <hh:styles itemCnt="2">
   <hh:style id="0" name="바탕글" engName="Normal" paraPrIDRef="0" charPrIDRef="0"/>
   <hh:style id="1" name="개요 1" engName="Outline 1" paraPrIDRef="0" charPrIDRef="1"/>
  </hh:styles>
 </hh:refList>
</hh:head>`

// hwpxFile builds a .hwpx around one section body.
func hwpxFile(t *testing.T, sectionXML string, binaries map[string][]byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	write := func(name string, data []byte) {
		writer, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	write("mimetype", []byte("application/hwp+zip"))
	write("version.xml", []byte(`<?xml version="1.0"?><hv:HCFVersion xmlns:hv="http://www.hancom.co.kr/hwpml/2011/version" tagetApplication="WORDPROCESSOR"/>`))
	write("Contents/header.xml", []byte(headerXML))
	write("Contents/section0.xml", []byte(sectionXML))
	for name, data := range binaries {
		write("BinData/"+name, data)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func section(body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<hs:sec xmlns:hs="http://www.hancom.co.kr/hwpml/2011/section"
        xmlns:hp="http://www.hancom.co.kr/hwpml/2011/paragraph"
        xmlns:hc="http://www.hancom.co.kr/hwpml/2011/core">` + body + `</hs:sec>`
}

func parseFile(t *testing.T, sectionXML string, binaries map[string][]byte) *richdoc.Node {
	t.Helper()
	document, _, _, err := Parse(hwpxFile(t, section(sectionXML), binaries))
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func blockTypes(document *richdoc.Node) []string {
	out := make([]string, 0, len(document.Content))
	for _, block := range document.Content {
		out = append(out, block.Type)
	}
	return out
}

func markedText(t *testing.T, document *richdoc.Node, phrase string) []string {
	t.Helper()
	var found []string
	var walk func(*richdoc.Node) bool
	walk = func(current *richdoc.Node) bool {
		if current == nil {
			return false
		}
		if current.Type == "text" && strings.Contains(current.Text, phrase) {
			for _, mark := range current.Marks {
				found = append(found, mark.Type)
			}
			return true
		}
		for _, child := range current.Content {
			if walk(child) {
				return true
			}
		}
		return false
	}
	if !walk(document) {
		t.Fatalf("%q 를 찾지 못했습니다: %q", phrase, document.PlainText())
	}
	return found
}

func has(marks []string, want string) bool {
	for _, mark := range marks {
		if mark == want {
			return true
		}
	}
	return false
}

func TestAParagraphComesThroughWithItsWords(t *testing.T) {
	document := parseFile(t, `<hp:p paraPrIDRef="0" styleIDRef="0"><hp:run charPrIDRef="0">`+
		`<hp:t>첫 문단입니다</hp:t></hp:run></hp:p>`, nil)
	if text := document.PlainText(); !strings.Contains(text, "첫 문단입니다") {
		t.Fatalf("본문 = %q", text)
	}
	if got := blockTypes(document); len(got) != 1 || got[0] != "paragraph" {
		t.Errorf("블록 = %v", got)
	}
}

// A run in HWPX names a charPr id and carries no formatting itself. Reading
// only the run finds none at all.
func TestARunTakesItsFormattingFromTheHeader(t *testing.T) {
	document := parseFile(t, `<hp:p styleIDRef="0"><hp:run charPrIDRef="1"><hp:t>굵고밑줄</hp:t></hp:run>`+
		`<hp:run charPrIDRef="2"><hp:t>기울이고빨강</hp:t></hp:run></hp:p>`, nil)
	bold := markedText(t, document, "굵고밑줄")
	if !has(bold, "bold") || !has(bold, "underline") {
		t.Errorf("굵기와 밑줄이 오지 않았습니다: %v", bold)
	}
	styled := markedText(t, document, "기울이고빨강")
	if !has(styled, "italic") || !has(styled, "textStyle") {
		t.Errorf("기울임과 색이 오지 않았습니다: %v", styled)
	}
}

// 개요 1 is Hangul's own outline style, and a document translated from Word
// carries "Heading 1". Both say the same thing.
func TestAnOutlineStyleBecomesAHeading(t *testing.T) {
	document := parseFile(t, `<hp:p styleIDRef="1"><hp:run charPrIDRef="1"><hp:t>제1장 총칙</hp:t></hp:run></hp:p>`+
		`<hp:p styleIDRef="0"><hp:run charPrIDRef="0"><hp:t>본문입니다</hp:t></hp:run></hp:p>`, nil)
	got := blockTypes(document)
	if len(got) != 2 || got[0] != "heading" || got[1] != "paragraph" {
		t.Fatalf("블록 = %v", got)
	}
	if level := document.Content[0].AttrInt("level", 0); level != 1 {
		t.Errorf("제목 단계 = %d", level)
	}
}

// The shape of a paragraph lives in the header too.
func TestAParagraphTakesItsLayoutFromTheHeader(t *testing.T) {
	document := parseFile(t, `<hp:p paraPrIDRef="1" styleIDRef="0"><hp:run charPrIDRef="0">`+
		`<hp:t>가운데 문단</hp:t></hp:run></hp:p>`, nil)
	paragraph := document.Content[0]
	if got := paragraph.AttrString("textAlign"); got != "center" {
		t.Errorf("정렬 = %q", got)
	}
	if got := paragraph.AttrString("lineHeight"); got != "1.6" {
		t.Errorf("줄간격 = %q", got)
	}
	if paragraph.AttrInt("indent", 0) == 0 {
		t.Errorf("들여쓰기가 오지 않았습니다: %v", paragraph.Attrs)
	}
}

// HWPX puts a table inside the paragraph that positions it. muni's table is a
// block of its own, so it has to come out — the same lift a picture needs.
func TestATableComesOutOfItsParagraph(t *testing.T) {
	document := parseFile(t, `<hp:p styleIDRef="0"><hp:run charPrIDRef="0"><hp:tbl rowCnt="2" colCnt="2" repeatHeader="true">`+
		`<hp:tr>`+
		`<hp:tc><hp:cellSpan colSpan="2" rowSpan="1"/><hp:subList><hp:p><hp:run charPrIDRef="0"><hp:t>표머리글</hp:t></hp:run></hp:p></hp:subList></hp:tc>`+
		`</hp:tr>`+
		`<hp:tr>`+
		`<hp:tc><hp:cellSpan colSpan="1" rowSpan="1"/><hp:subList><hp:p><hp:run charPrIDRef="0"><hp:t>왼쪽칸</hp:t></hp:run></hp:p></hp:subList></hp:tc>`+
		`<hp:tc><hp:cellSpan colSpan="1" rowSpan="1"/><hp:subList><hp:p><hp:run charPrIDRef="0"><hp:t>오른쪽칸</hp:t></hp:run></hp:p></hp:subList></hp:tc>`+
		`</hp:tr>`+
		`</hp:tbl></hp:run></hp:p>`, nil)

	var table *richdoc.Node
	for _, block := range document.Content {
		if block.Type == "table" {
			table = block
		}
	}
	if table == nil {
		t.Fatalf("표가 블록으로 올라오지 않았습니다: %v", blockTypes(document))
	}
	if len(table.Content) != 2 {
		t.Fatalf("행 = %d개", len(table.Content))
	}
	head := table.Content[0].Content[0]
	if head.Type != "tableHeader" {
		t.Errorf("첫 행이 머리글이 아닙니다: %s", head.Type)
	}
	if span := head.AttrInt("colspan", 1); span != 2 {
		t.Errorf("가로 병합 = %d", span)
	}
	for _, phrase := range []string{"표머리글", "왼쪽칸", "오른쪽칸"} {
		if !strings.Contains(document.PlainText(), phrase) {
			t.Errorf("%q 가 사라졌습니다", phrase)
		}
	}
}

// A picture is a block of its own in muni, and a paragraph holding one is a
// document the editor refuses outright.
func TestAPictureBecomesABlockWithItsBytes(t *testing.T) {
	pixel := []byte{
		0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0a, 'I', 'D', 'A', 'T', 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 'I', 'E', 'N', 'D', 0xae,
		0x42, 0x60, 0x82,
	}
	body := section(`<hp:p styleIDRef="0"><hp:run charPrIDRef="0"><hp:t>사진 앞</hp:t>` +
		`<hp:pic alt="현장 사진"><hc:img binaryItemIDRef="image1"/></hp:pic>` +
		`<hp:t>사진 뒤</hp:t></hp:run></hp:p>`)
	document, assets, _, err := Parse(hwpxFile(t, body, map[string][]byte{"image1.png": pixel}))
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || !bytes.Equal(assets[0].Data, pixel) {
		t.Fatalf("그림의 바이트가 남지 않았습니다: %+v", assets)
	}
	images := 0
	for _, block := range document.Content {
		if block.Type == "image" {
			images++
		}
		for _, child := range block.Content {
			if child != nil && child.Type == "image" {
				t.Fatalf("그림이 %s 안에 갇혔습니다", block.Type)
			}
		}
	}
	if images != 1 {
		t.Errorf("그림 블록 = %d개: %v", images, blockTypes(document))
	}
	if text := document.PlainText(); !strings.Contains(text, "사진 앞") || !strings.Contains(text, "사진 뒤") {
		t.Errorf("주위 글자가 사라졌습니다: %q", text)
	}
}

// A file with no section is not a document, and saying so beats returning an
// empty one.
func TestAFileWithNoSectionIsRefused(t *testing.T) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	writer, _ := archive.Create("mimetype")
	_, _ = writer.Write([]byte("application/hwp+zip"))
	_ = archive.Close()
	if _, _, _, err := Parse(buffer.Bytes()); err == nil {
		t.Fatal("본문이 없는 파일을 받아들였습니다")
	}
}

// Something that is not a zip at all is refused rather than panicking.
func TestRubbishIsRefused(t *testing.T) {
	if _, _, _, err := Parse([]byte("이건 hwpx 가 아닙니다")); err == nil {
		t.Fatal("압축이 아닌 파일을 받아들였습니다")
	}
}
