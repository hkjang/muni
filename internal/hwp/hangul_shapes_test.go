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
