package hwp

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

// spacedShapeRecord writes a PARA_SHAPE in the current layout — 54 bytes,
// the line spacing at 50 and its kind in the third property word.
func spacedShapeRecord(percent uint32) []byte {
	data := make([]byte, 54)
	binary.LittleEndian.PutUint32(data[24:], percent)
	binary.LittleEndian.PutUint32(data[50:], percent)
	return append(recordHeader(tagParaShape, 0, len(data)), data...)
}

func TestLineSpacingIsReadAsAMultipleOfTheFont(t *testing.T) {
	document, _, _, err := Parse(hwpFileWithDocInfo(t, spacedShapeRecord(200), styledParagraph(units("넓은 줄"), 0, 0)))
	if err != nil {
		t.Fatal(err)
	}
	if rate := document.Content[0].AttrString("lineHeight"); rate != "2" {
		t.Errorf("줄간격 = %q (%v)", rate, document.Content[0].Attrs)
	}
}

// linkControl writes a hyperlink field control: the id, a property word,
// a flag byte, then the command as a length-prefixed string.
func linkControl(command string) []byte {
	data := []byte{'k', 'l', 'h', '%'}
	data = append(data, make([]byte, 5)...)
	code := units(command)
	data = binary.LittleEndian.AppendUint16(data, uint16(len(code)))
	for _, unit := range code {
		data = binary.LittleEndian.AppendUint16(data, unit)
	}
	data = append(data, make([]byte, 4)...)
	return append(recordHeader(tagCtrlHeader, 1, len(data)), data...)
}

func TestAHyperlinkFieldBecomesALinkOnItsWords(t *testing.T) {
	// 앞 [field begin] 한컴 [field end] 뒤
	code := units("앞 ")
	code = append(code, 3)
	code = append(code, make([]uint16, 7)...)
	code = append(code, units("한컴")...)
	code = append(code, 4)
	code = append(code, make([]uint16, 7)...)
	code = append(code, units(" 뒤")...)
	body := append(paragraphRecords(code), linkControl(`http\://www.hancom.co.kr;1;0;0;`)...)
	document, _, _, err := Parse(hwpFileWithDocInfo(t, nil, body))
	if err != nil {
		t.Fatal(err)
	}
	linked := map[string]string{}
	var walk func(*richdoc.Node)
	walk = func(node *richdoc.Node) {
		if node.Type == "text" {
			for _, mark := range node.Marks {
				if mark.Type == "link" {
					linked[node.Text] = mark.AttrString("href")
				}
			}
		}
		for _, child := range node.Content {
			walk(child)
		}
	}
	walk(document)
	if linked["한컴"] != "http://www.hancom.co.kr" || len(linked) != 1 {
		t.Errorf("링크 = %v; 본문 %q", linked, document.PlainText())
	}
}

func TestAWiderPaperMeansLandscape(t *testing.T) {
	control := []byte{'d', 'c', 'e', 's'}
	control = append(control, make([]byte, 8)...)
	records := append(recordHeader(tagCtrlHeader, 1, len(control)), control...)
	page := make([]byte, 40)
	binary.LittleEndian.PutUint32(page[0:], 84188)
	binary.LittleEndian.PutUint32(page[4:], 59528)
	records = append(records, recordHeader(tagPageDef, 2, len(page))...)
	records = append(records, page...)
	_, _, meta, err := Parse(hwpFileWithDocInfo(t, nil, paragraphWithControl("가로 문서", records)))
	if err != nil {
		t.Fatal(err)
	}
	if !meta.Landscape {
		t.Errorf("가로가 아닙니다")
	}
	_, _, portrait, _ := Parse(hwpFileWithDocInfo(t, nil, paragraphRecords(units("세로"))))
	if portrait.Landscape {
		t.Errorf("세로 문서가 가로로 읽혔습니다")
	}
	_ = strings.TrimSpace
}
