package hwp

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

// What real files taught, kept as tests: each of these is a shape a fixture
// written from the reader would never have had, found by comparing the
// reader's words against another implementation's over seventeen files.

// listShapeRecord writes a PARA_SHAPE whose first word says the paragraph
// heads with a number (2) or a bullet (3) in bits 23-24, at a depth in 25-27.
func listShapeRecord(kind, level uint32) []byte {
	data := make([]byte, 54)
	binary.LittleEndian.PutUint32(data[0:], kind<<23|level<<25)
	return append(recordHeader(tagParaShape, 0, len(data)), data...)
}

// controlWithList writes a control of one id with a single list of one
// paragraph beneath it — the shape of a header, a footer, a note.
func controlWithList(id string, level uint16, text string) []byte {
	control := []byte{id[3], id[2], id[1], id[0]} // stored back to front
	control = append(control, make([]byte, 8)...)
	out := append(recordHeader(tagCtrlHeader, level, len(control)), control...)
	list := make([]byte, 8)
	binary.LittleEndian.PutUint32(list[0:], 1)
	out = append(out, recordHeader(tagListHeader, level+1, len(list))...)
	out = append(out, list...)
	return append(out, shiftLevels(paragraphRecords(units(text)), level+1)...)
}

func TestBulletParagraphsBecomeAListAndDeeperOnesNest(t *testing.T) {
	docInfo := append(listShapeRecord(3, 0), listShapeRecord(3, 1)...)
	docInfo = append(docInfo, paraShapeRecord(0, 0, 0)...)
	body := styledParagraph(units("첫째"), 0, 0)
	body = append(body, styledParagraph(units("첫째의 아래"), 1, 0)...)
	body = append(body, styledParagraph(units("둘째"), 0, 0)...)
	body = append(body, styledParagraph(units("목록 뒤"), 2, 0)...)
	document, _, _, err := Parse(hwpFileWithDocInfo(t, docInfo, body))
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Content) != 2 || document.Content[0].Type != "bulletList" || document.Content[1].Type != "paragraph" {
		t.Fatalf("블록 = %v", blockTypes(document))
	}
	list := document.Content[0]
	if len(list.Content) != 2 {
		t.Fatalf("항목 = %d개", len(list.Content))
	}
	first := list.Content[0]
	if len(first.Content) != 2 || first.Content[1].Type != "bulletList" {
		t.Fatalf("첫 항목 안 = %v", blockTypes(first))
	}
	if text := first.Content[1].PlainText(); !strings.Contains(text, "첫째의 아래") {
		t.Errorf("안쪽 목록 = %q", text)
	}
}

func TestNumberedParagraphsBecomeAnOrderedList(t *testing.T) {
	docInfo := listShapeRecord(2, 0)
	body := styledParagraph(units("하나"), 0, 0)
	body = append(body, styledParagraph(units("둘"), 0, 0)...)
	document, _, _, err := Parse(hwpFileWithDocInfo(t, docInfo, body))
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Content) != 1 || document.Content[0].Type != "orderedList" || len(document.Content[0].Content) != 2 {
		t.Fatalf("블록 = %v", blockTypes(document))
	}
}

func TestTheHeaderAndFooterAreKeptAsTheDocumentsOwn(t *testing.T) {
	body := paragraphWithControl("본문", controlWithList("head", 1, "이것은 머리말"))
	body = append(body, paragraphWithControl("", controlWithList("foot", 1, "이것은 꼬리말"))...)
	document, _, meta, err := Parse(hwpFileWithDocInfo(t, nil, body))
	if err != nil {
		t.Fatal(err)
	}
	if meta.Header != "이것은 머리말" || meta.Footer != "이것은 꼬리말" {
		t.Errorf("머리말/꼬리말 = %q / %q", meta.Header, meta.Footer)
	}
	if text := document.PlainText(); strings.Contains(text, "머리말") || strings.Contains(text, "꼬리말") {
		t.Errorf("머리말이 본문에 섞였습니다: %q", text)
	}
}

func TestATableCaptionFollowsTheTable(t *testing.T) {
	records := tableRecords(1, []tableCellSpec{
		{row: 0, column: 0, span: 1, rowSpan: 1, text: "칸"},
	})
	// The caption is a list with no cell address, after the cells.
	records = append(records, recordHeader(tagListHeader, 2, 0)...)
	records = append(records, shiftLevels(paragraphRecords(units("[표 캡션]")), 2)...)
	document, _, _, err := Parse(hwpFileWithDocInfo(t, nil, paragraphWithControl("표 앞", records)))
	if err != nil {
		t.Fatal(err)
	}
	types := blockTypes(document)
	if len(types) < 3 || types[1] != "table" || types[2] != "paragraph" {
		t.Fatalf("블록 = %v", types)
	}
	if text := document.Content[2].PlainText(); text != "[표 캡션]" {
		t.Errorf("캡션 = %q", text)
	}
}

func blockTypes(node *richdoc.Node) []string {
	out := []string{}
	for _, child := range node.Content {
		if child != nil {
			out = append(out, child.Type)
		}
	}
	return out
}

// A shaded heading row is how a Korean report tells its table's headings from
// its body, and it is the one thing about a .hwp table muni threw away while
// keeping the same table's shading out of a .hwpx. The colour is not on the
// cell: the cell names a BORDER_FILL by number, and DocInfo holds the fill.
func TestATableCellTakesItsShadeFromTheBorderFillItNames(t *testing.T) {
	docInfo := borderFillRecord(1, colorRefBytes(0xD9, 0xE2, 0xF3)) // 첫째
	docInfo = append(docInfo, borderFillRecord(1, colorRefBytes(0xFF, 0xFF, 0xFF))...)
	body := paragraphWithControl("", tableRecords(1, []tableCellSpec{
		{row: 0, column: 0, span: 1, rowSpan: 1, borderFill: 1, text: "머리글"},
		{row: 1, column: 0, span: 1, rowSpan: 1, borderFill: 2, text: "본문"},
	}))
	document, _, _, err := Parse(hwpFileWithDocInfo(t, docInfo, body))
	if err != nil {
		t.Fatal(err)
	}
	table := firstTable(t, document)
	if shade := table.Content[0].Content[0].AttrString("backgroundColor"); shade != "#d9e2f3" {
		t.Errorf("머리글 칸의 음영 = %q", shade)
	}
	// White is what an unshaded cell is drawn in already, so recording it
	// would put a colour on every cell of every table.
	if shade := table.Content[1].Content[0].AttrString("backgroundColor"); shade != "" {
		t.Errorf("흰 칸에 음영이 붙었습니다: %q", shade)
	}
}

// A fill that is a picture or a gradient is not a colour, and reading the
// bytes after the kind regardless would paint the cell in whatever the
// gradient's first stop happens to be.
func TestACellFilledWithSomethingOtherThanAColourTakesNoShade(t *testing.T) {
	const gradient = 4
	docInfo := borderFillRecord(gradient, colorRefBytes(0x00, 0x66, 0xCC))
	body := paragraphWithControl("", tableRecords(1, []tableCellSpec{
		{row: 0, column: 0, span: 1, rowSpan: 1, borderFill: 1, text: "칸"},
	}))
	document, _, _, err := Parse(hwpFileWithDocInfo(t, docInfo, body))
	if err != nil {
		t.Fatal(err)
	}
	table := firstTable(t, document)
	if shade := table.Content[0].Content[0].AttrString("backgroundColor"); shade != "" {
		t.Errorf("그러데이션이 음영으로 들어왔습니다: %q", shade)
	}
}

// Where the words sit in a cell is in the property word every paragraph list
// opens with, not in the cell's address. Every cell muni read out of a .hwp
// was aligned to the top, however the document had it.
func TestACellKeepsWhereItsWordsSit(t *testing.T) {
	body := paragraphWithControl("", tableRecords(1, []tableCellSpec{
		{row: 0, column: 0, span: 1, rowSpan: 1, verticalAlign: 1, text: "가운데"},
		{row: 0, column: 1, span: 1, rowSpan: 1, verticalAlign: 2, text: "아래"},
		{row: 0, column: 2, span: 1, rowSpan: 1, verticalAlign: 0, text: "위"},
	}))
	document, _, _, err := Parse(hwpFileWithDocInfo(t, nil, body))
	if err != nil {
		t.Fatal(err)
	}
	table := firstTable(t, document)
	want := []string{"middle", "bottom", "top"}
	for index, expected := range want {
		if got := table.Content[0].Content[index].AttrString("verticalAlign"); got != expected {
			t.Errorf("%d번째 칸의 세로 정렬 = %q, 원하는 것 = %q", index, got, expected)
		}
	}
}

// A cell naming a fill the document does not have is a cell with no shade,
// not a read past the end of the list.
func TestACellNamingAFillTheDocumentHasNotGotTakesNoShade(t *testing.T) {
	body := paragraphWithControl("", tableRecords(1, []tableCellSpec{
		{row: 0, column: 0, span: 1, rowSpan: 1, borderFill: 9, text: "칸"},
	}))
	document, _, _, err := Parse(hwpFileWithDocInfo(t, nil, body))
	if err != nil {
		t.Fatal(err)
	}
	table := firstTable(t, document)
	if shade := table.Content[0].Content[0].AttrString("backgroundColor"); shade != "" {
		t.Errorf("음영 = %q", shade)
	}
}

func firstTable(t *testing.T, document *richdoc.Node) *richdoc.Node {
	t.Helper()
	for _, block := range document.Content {
		if block.Type == "table" {
			return block
		}
	}
	t.Fatalf("표가 나오지 않았습니다: %v", blockTypes(document))
	return nil
}
