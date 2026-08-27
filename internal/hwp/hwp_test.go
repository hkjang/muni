package hwp

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

func hwpFile(t *testing.T, compressed, encrypted bool, sections ...[]byte) []byte {
	t.Helper()
	streams := []streamSpec{{path: "FileHeader", data: hwpFileHeader(compressed, encrypted)}}
	for index, section := range sections {
		data := section
		if compressed {
			data = deflateRaw(t, section)
		}
		streams = append(streams, streamSpec{
			path: "BodyText/Section" + string(rune('0'+index)), data: data,
		})
	}
	return buildCompound(t, streams)
}

func TestAParagraphComesThrough(t *testing.T) {
	body := paragraphRecords(units("첫 문단입니다"))
	document, _, meta, err := Parse(hwpFile(t, false, false, body))
	if err != nil {
		t.Fatal(err)
	}
	if text := document.PlainText(); !strings.Contains(text, "첫 문단입니다") {
		t.Fatalf("본문 = %q", text)
	}
	if !strings.HasPrefix(meta.Version, "5.") {
		t.Errorf("판 = %q", meta.Version)
	}
}

// A .hwp stream is raw deflate with no zlib wrapper, which is why a reader
// that reaches for zlib gets "invalid header" on a perfectly good file.
func TestACompressedFileIsRead(t *testing.T) {
	body := paragraphRecords(units("압축된 문단입니다"))
	document, _, _, err := Parse(hwpFile(t, true, false, body))
	if err != nil {
		t.Fatal(err)
	}
	if text := document.PlainText(); !strings.Contains(text, "압축된 문단입니다") {
		t.Fatalf("본문 = %q", text)
	}
}

// The trap the format's own documentation sets: a control mark's size is
// given as 8, and it means 8 WCHAR — sixteen bytes. A reader that skips eight
// lands inside the mark and reads its second half as text, which comes out as
// ideographs that were never in the document.
func TestAControlMarkIsSkippedWholeNotHalf(t *testing.T) {
	// 앞 [table mark, 8 WCHAR] 뒤
	code := make([]uint16, 0, 16)
	code = append(code, units("앞말")...)
	code = append(code, 11) // an extended control
	for filler := 0; filler < 6; filler++ {
		code = append(code, 0x4E00) // bytes inside the mark that must not be read
	}
	code = append(code, 11)
	code = append(code, units("뒷말")...)

	document, _, _, err := Parse(hwpFile(t, false, false, paragraphRecords(code)))
	if err != nil {
		t.Fatal(err)
	}
	text := document.PlainText()
	for _, want := range []string{"앞말", "뒷말"} {
		if !strings.Contains(text, want) {
			t.Errorf("%q 가 사라졌습니다: %q", want, text)
		}
	}
	if strings.Contains(text, "一") {
		t.Errorf("표시 안쪽 바이트를 글자로 읽었습니다: %q", text)
	}
}

// A tab is a control mark too, and the same width.
func TestATabIsKeptAndSkippedWhole(t *testing.T) {
	code := make([]uint16, 0, 16)
	code = append(code, units("앞")...)
	code = append(code, 9)
	for filler := 0; filler < 6; filler++ {
		code = append(code, 0x4E00)
	}
	code = append(code, 9)
	code = append(code, units("뒤")...)

	document, _, _, err := Parse(hwpFile(t, false, false, paragraphRecords(code)))
	if err != nil {
		t.Fatal(err)
	}
	text := document.PlainText()
	if !strings.Contains(text, "앞\t뒤") {
		t.Errorf("탭이 남지 않았습니다: %q", text)
	}
}

// A line break inside a paragraph is one WCHAR, not eight.
func TestALineBreakIsOneCharacter(t *testing.T) {
	code := append(units("첫 줄"), 10)
	code = append(code, units("둘째 줄")...)
	document, _, _, err := Parse(hwpFile(t, false, false, paragraphRecords(code)))
	if err != nil {
		t.Fatal(err)
	}
	text := document.PlainText()
	for _, want := range []string{"첫 줄", "둘째 줄"} {
		if !strings.Contains(text, want) {
			t.Errorf("%q 가 사라졌습니다: %q", want, text)
		}
	}
}

// Saying so beats importing a document of mojibake.
func TestAnEncryptedFileIsRefusedClearly(t *testing.T) {
	_, _, _, err := Parse(hwpFile(t, false, true, paragraphRecords(units("안 보임"))))
	if err == nil {
		t.Fatal("암호가 걸린 문서를 받아들였습니다")
	}
	if !strings.Contains(err.Error(), "암호") {
		t.Errorf("무엇이 문제인지 말하지 않습니다: %v", err)
	}
}

func TestSomethingThatIsNotAnHwpIsRefused(t *testing.T) {
	if _, _, _, err := Parse([]byte("이건 hwp 가 아닙니다")); err == nil {
		t.Fatal("HWP 가 아닌 파일을 받아들였습니다")
	}
}

// Several sections are read in the order their names give.
func TestSectionsAreReadInOrder(t *testing.T) {
	document, _, _, err := Parse(hwpFile(t, false, false,
		paragraphRecords(units("첫 구역")),
		paragraphRecords(units("둘째 구역"))))
	if err != nil {
		t.Fatal(err)
	}
	text := document.PlainText()
	first, second := strings.Index(text, "첫 구역"), strings.Index(text, "둘째 구역")
	if first < 0 || second < 0 {
		t.Fatalf("구역이 사라졌습니다: %q", text)
	}
	if first > second {
		t.Errorf("구역 순서가 뒤바뀌었습니다: %q", text)
	}
}

// hwpFileWithDocInfo builds a file carrying the shapes the body refers to by
// number.
func hwpFileWithDocInfo(t *testing.T, docInfo []byte, sections ...[]byte) []byte {
	t.Helper()
	streams := []streamSpec{
		{path: "FileHeader", data: hwpFileHeader(false, false)},
		{path: "DocInfo", data: docInfo},
	}
	for index, section := range sections {
		streams = append(streams, streamSpec{
			path: "BodyText/Section" + string(rune('0'+index)), data: section,
		})
	}
	return buildCompound(t, streams)
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

// A .hwp keeps a run's formatting in DocInfo and points at it by number from a
// list of positions. Reading only the text finds none of it.
func TestFormattingReachesTheWordsItCovers(t *testing.T) {
	docInfo := append(charShapeRecord(false, false, false), charShapeRecord(true, false, false)...)
	docInfo = append(docInfo, charShapeRecord(false, true, true)...)

	// "보통글자굵은글자기울인글자" with the shape changing at each word.
	text := units("보통글자굵은글자기울인글자")
	body := paragraphRecords(text,
		charRun{at: 0, shape: 0},
		charRun{at: 4, shape: 1},
		charRun{at: 8, shape: 2})

	document, _, _, err := Parse(hwpFileWithDocInfo(t, docInfo, body))
	if err != nil {
		t.Fatal(err)
	}
	if marks := markedText(t, document, "보통글자"); len(marks) != 0 {
		t.Errorf("보통 글자에 서식이 붙었습니다: %v", marks)
	}
	if marks := markedText(t, document, "굵은글자"); !has(marks, "bold") {
		t.Errorf("굵기가 오지 않았습니다: %v", marks)
	}
	italic := markedText(t, document, "기울인글자")
	if !has(italic, "italic") || !has(italic, "underline") {
		t.Errorf("기울임과 밑줄이 오지 않았습니다: %v", italic)
	}
}

// The positions a shape list refers to count every code unit the paragraph
// held, including the eight a control mark occupies. Counting only the
// characters that survived puts every shape after the first table or picture
// on the wrong words.
func TestShapePositionsCountTheControlMarksToo(t *testing.T) {
	docInfo := append(charShapeRecord(false, false, false), charShapeRecord(true, false, false)...)

	code := make([]uint16, 0, 24)
	code = append(code, units("앞말")...) // positions 0-1
	code = append(code, 11)             // a control mark: positions 2-9
	for filler := 0; filler < 6; filler++ {
		code = append(code, 0x4E00)
	}
	code = append(code, 11)
	code = append(code, units("뒷말")...) // positions 10-11

	body := paragraphRecords(code, charRun{at: 0, shape: 0}, charRun{at: 10, shape: 1})
	document, _, _, err := Parse(hwpFileWithDocInfo(t, docInfo, body))
	if err != nil {
		t.Fatal(err)
	}
	if marks := markedText(t, document, "앞말"); has(marks, "bold") {
		t.Errorf("앞말에 굵기가 잘못 붙었습니다: %v", marks)
	}
	if marks := markedText(t, document, "뒷말"); !has(marks, "bold") {
		t.Errorf("표시 뒤의 굵기가 어긋났습니다: %v", marks)
	}
}

// A table hangs beneath the control mark that positions it: a TABLE record and
// one LIST_HEADER per cell, each carrying where it sits and how far it reaches,
// with its paragraphs beneath. Reading the records flat finds the cells' words
// and nothing about the table they are in.
func TestATableIsRebuiltFromWhereItsCellsSay(t *testing.T) {
	body := paragraphWithControl("표 앞", tableRecords(1, []tableCellSpec{
		{row: 0, column: 0, span: 2, rowSpan: 1, text: "표머리글"},
		{row: 1, column: 0, span: 1, rowSpan: 1, text: "왼쪽칸"},
		{row: 1, column: 1, span: 1, rowSpan: 1, text: "오른쪽칸"},
	}))
	document, _, _, err := Parse(hwpFileWithDocInfo(t, nil, body))
	if err != nil {
		t.Fatal(err)
	}
	var table *richdoc.Node
	for _, block := range document.Content {
		if block.Type == "table" {
			table = block
		}
	}
	if table == nil {
		t.Fatalf("표가 나오지 않았습니다: %q", document.PlainText())
	}
	if len(table.Content) != 2 {
		t.Fatalf("행 = %d개", len(table.Content))
	}
	if span := table.Content[0].Content[0].AttrInt("colspan", 1); span != 2 {
		t.Errorf("가로 병합 = %d", span)
	}
	if len(table.Content[1].Content) != 2 {
		t.Errorf("둘째 행의 칸 = %d개", len(table.Content[1].Content))
	}
	for _, phrase := range []string{"표 앞", "표머리글", "왼쪽칸", "오른쪽칸"} {
		if !strings.Contains(document.PlainText(), phrase) {
			t.Errorf("%q 가 사라졌습니다", phrase)
		}
	}
}

// Cells written out of order still read left to right.
func TestCellsOutOfOrderStillReadLeftToRight(t *testing.T) {
	body := paragraphWithControl("", tableRecords(1, []tableCellSpec{
		{row: 0, column: 1, span: 1, rowSpan: 1, text: "둘째칸"},
		{row: 0, column: 0, span: 1, rowSpan: 1, text: "첫째칸"},
	}))
	document, _, _, err := Parse(hwpFileWithDocInfo(t, nil, body))
	if err != nil {
		t.Fatal(err)
	}
	text := document.PlainText()
	first, second := strings.Index(text, "첫째칸"), strings.Index(text, "둘째칸")
	if first < 0 || second < 0 {
		t.Fatalf("칸이 사라졌습니다: %q", text)
	}
	if first > second {
		t.Errorf("칸 순서가 뒤바뀌었습니다: %q", text)
	}
}

// A picture's bytes live in a stream of their own, named for the id the
// document gave it. The control says which.
func TestAPictureKeepsItsBytes(t *testing.T) {
	pixel := []byte{
		0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0a, 'I', 'D', 'A', 'T', 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 'I', 'E', 'N', 'D', 0xae,
		0x42, 0x60, 0x82,
	}
	// A picture control with the picture's own record beneath it, carrying the
	// BinData id where the format puts it — after the border, the four
	// corners, the crop and the margins.
	control := []byte{'c', 'i', 'p', '$'} // "$pic" back to front
	control = append(control, make([]byte, 8)...)
	body := append(recordHeader(tagCtrlHeader, 1, len(control)), control...)
	shape := make([]byte, pictureBinIDOffset+8)
	binary.LittleEndian.PutUint16(shape[pictureBinIDOffset:], 1)
	body = append(body, recordHeader(tagShapePicture, 2, len(shape))...)
	body = append(body, shape...)

	streams := []streamSpec{
		{path: "FileHeader", data: hwpFileHeader(false, false)},
		{path: "BodyText/Section0", data: append(paragraphRecords(units("사진 앞")), body...)},
		{path: "BinData/BIN0001.png", data: pixel},
	}
	document, assets, _, err := Parse(buildCompound(t, streams))
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
		t.Errorf("그림 블록 = %d개", images)
	}
	if !strings.Contains(document.PlainText(), "사진 앞") {
		t.Errorf("주위 글자가 사라졌습니다: %q", document.PlainText())
	}
}

// A paragraph names its shape and its style by number, and both live in
// DocInfo. Reading only the body finds a document with no alignment, no
// indentation and no headings — most of what a Korean report's layout is.
func TestAParagraphTakesItsShapeAndStyleByNumber(t *testing.T) {
	docInfo := paraShapeRecord(1, 0, 0)                       // shape 0: left, plain
	docInfo = append(docInfo, paraShapeRecord(3, 3600, 0)...) // shape 1: centred, indented
	docInfo = append(docInfo, styleRecord("바탕글", "Normal")...)
	docInfo = append(docInfo, styleRecord("개요 1", "Outline 1")...)

	body := styledParagraph(units("제1장 총칙"), 0, 1)
	body = append(body, styledParagraph(units("가운데 문단"), 1, 0)...)

	document, _, _, err := Parse(hwpFileWithDocInfo(t, docInfo, body))
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Content) != 2 {
		t.Fatalf("블록 = %d개: %q", len(document.Content), document.PlainText())
	}
	heading := document.Content[0]
	if heading.Type != "heading" || heading.AttrInt("level", 0) != 1 {
		t.Errorf("개요 1 이 제목이 되지 않았습니다: %s %v", heading.Type, heading.Attrs)
	}
	body2 := document.Content[1]
	if got := body2.AttrString("textAlign"); got != "center" {
		t.Errorf("정렬 = %q", got)
	}
	if body2.AttrInt("indent", 0) == 0 {
		t.Errorf("들여쓰기가 오지 않았습니다: %v", body2.Attrs)
	}
}

// A heading draws its own weight and spacing. The shape it names describes a
// paragraph, and applying it would fight what muni already does.
func TestAHeadingDoesNotTakeAParagraphsIndent(t *testing.T) {
	docInfo := paraShapeRecord(3, 3600, 0)
	docInfo = append(docInfo, styleRecord("개요 2", "Outline 2")...)
	body := styledParagraph(units("제1절 목적"), 0, 0)

	document, _, _, err := Parse(hwpFileWithDocInfo(t, docInfo, body))
	if err != nil {
		t.Fatal(err)
	}
	heading := document.Content[0]
	if heading.Type != "heading" {
		t.Fatalf("제목이 아닙니다: %s", heading.Type)
	}
	if heading.AttrInt("level", 0) != 2 {
		t.Errorf("제목 단계 = %d", heading.AttrInt("level", 0))
	}
	if heading.AttrInt("indent", 0) != 0 || heading.AttrString("textAlign") != "" {
		t.Errorf("제목이 문단 서식을 가져갔습니다: %v", heading.Attrs)
	}
}

// A tab occupies eight positions like any other control mark. Counting it as
// one puts every shape after it seven positions early, so the words after a
// tab wear the wrong formatting.
func TestShapePositionsCountATabAsEight(t *testing.T) {
	docInfo := append(charShapeRecord(false, false, false), charShapeRecord(true, false, false)...)

	code := make([]uint16, 0, 24)
	code = append(code, units("앞말")...) // positions 0-1
	code = append(code, 9)              // a tab: positions 2-9
	for filler := 0; filler < 6; filler++ {
		code = append(code, 0x4E00)
	}
	code = append(code, 9)
	code = append(code, units("뒷말")...) // positions 10-11

	body := paragraphRecords(code, charRun{at: 0, shape: 0}, charRun{at: 10, shape: 1})
	document, _, _, err := Parse(hwpFileWithDocInfo(t, docInfo, body))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(document.PlainText(), "앞말\t뒷말") {
		t.Errorf("탭이 자리를 지키지 않았습니다: %q", document.PlainText())
	}
	if marks := markedText(t, document, "앞말"); has(marks, "bold") {
		t.Errorf("앞말에 굵기가 잘못 붙었습니다: %v", marks)
	}
	if marks := markedText(t, document, "뒷말"); !has(marks, "bold") {
		t.Errorf("탭 뒤의 굵기가 어긋났습니다: %v", marks)
	}
}
