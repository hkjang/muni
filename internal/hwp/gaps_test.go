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

// faceNameRecord writes one FACE_NAME: a property byte saying which optional
// tails follow, then the name as a length-prefixed string. muni asks for none
// of the tails, and a fixture that leaves the property byte out would let the
// reader read the name one byte early and still pass.
func faceNameRecord(name string) []byte {
	data := []byte{0}
	code := units(name)
	data = binary.LittleEndian.AppendUint16(data, uint16(len(code)))
	for _, unit := range code {
		data = binary.LittleEndian.AppendUint16(data, unit)
	}
	return append(recordHeader(tagFaceName, 0, len(data)), data...)
}

// dressedCharShapeRecord writes a CHAR_SHAPE with a face, a size and a
// colour, laid out from the format's own field list rather than from the
// reader's constants: UINT16[7] faces, UINT8[7] ratios, INT8[7] spacings,
// UINT8[7] relative sizes, INT8[7] offsets, INT32 base size, UINT32
// properties, INT8 shadow x, INT8 shadow y, then the colours.
func dressedCharShapeRecord(fontID uint16, hundredthsOfPoint int32, color uint32) []byte {
	const faces, ratios, spacings, relativeSizes, offsets = 7 * 2, 7, 7, 7, 7
	baseSizeAt := faces + ratios + spacings + relativeSizes + offsets
	colorAt := baseSizeAt + 4 + 4 + 1 + 1
	data := make([]byte, colorAt+4*4)
	binary.LittleEndian.PutUint16(data[0:], fontID)
	binary.LittleEndian.PutUint32(data[baseSizeAt:], uint32(hundredthsOfPoint))
	binary.LittleEndian.PutUint32(data[colorAt:], color)
	return append(recordHeader(tagCharShape, 0, len(data)), data...)
}

func TestACharShapeCarriesItsSizeColourAndFace(t *testing.T) {
	docInfo := faceNameRecord("함초롬바탕")
	docInfo = append(docInfo, faceNameRecord("맑은 고딕")...)
	// 0x00BBGGRR: a plain red is 0x0000FF, blue first.
	docInfo = append(docInfo, dressedCharShapeRecord(1, 1400, 0x0000FF)...)
	document, _, _, err := Parse(hwpFileWithDocInfo(t, docInfo, paragraphRecords(units("붉은 제목"), charRun{at: 0, shape: 0})))
	if err != nil {
		t.Fatal(err)
	}
	style := textStyleOf(t, document, "붉은 제목")
	if style["color"] != "#FF0000" {
		t.Errorf("글자색 = %v", style["color"])
	}
	if style["fontSize"] != "14pt" {
		t.Errorf("글자 크기 = %v", style["fontSize"])
	}
	if style["fontFamily"] != "맑은 고딕" {
		t.Errorf("글꼴 = %v", style["fontFamily"])
	}
}

// Hangul writes every run's shape out in full, so a document nobody dressed
// still names black and ten point. Marking those would put a textStyle on
// every word of every file and overrule the reader muni's editor uses.
func TestTheDefaultBlackAndTenPointAreNotMarked(t *testing.T) {
	docInfo := append(faceNameRecord("함초롬바탕"), dressedCharShapeRecord(0, 1000, 0)...)
	document, _, _, err := Parse(hwpFileWithDocInfo(t, docInfo, paragraphRecords(units("보통 글"), charRun{at: 0, shape: 0})))
	if err != nil {
		t.Fatal(err)
	}
	style := textStyleOf(t, document, "보통 글")
	if _, ok := style["color"]; ok {
		t.Errorf("검은 글자에 색이 붙었습니다: %v", style)
	}
	if _, ok := style["fontSize"]; ok {
		t.Errorf("열 포인트에 크기가 붙었습니다: %v", style)
	}
	if style["fontFamily"] != "함초롬바탕" {
		t.Errorf("글꼴 = %v", style["fontFamily"])
	}
}

// A face number with no FACE_NAME behind it names nothing. Falling back to
// the number would set the run in a font called "9".
func TestAFaceNumberWithNoTableNamesNothing(t *testing.T) {
	document, _, _, err := Parse(hwpFileWithDocInfo(t, dressedCharShapeRecord(9, 1000, 0), paragraphRecords(units("이름 없는 글꼴"), charRun{at: 0, shape: 0})))
	if err != nil {
		t.Fatal(err)
	}
	if style := textStyleOf(t, document, "이름 없는 글꼴"); len(style) != 0 {
		t.Errorf("서식 = %v", style)
	}
}

// textStyleOf returns the textStyle attributes on the text holding a phrase,
// empty when it carries none.
func textStyleOf(t *testing.T, document *richdoc.Node, phrase string) map[string]string {
	t.Helper()
	out := map[string]string{}
	found := false
	var walk func(*richdoc.Node)
	walk = func(node *richdoc.Node) {
		if node == nil || found {
			return
		}
		if node.Type == "text" && strings.Contains(node.Text, phrase) {
			found = true
			for _, mark := range node.Marks {
				if mark.Type != "textStyle" {
					continue
				}
				for _, name := range []string{"color", "fontSize", "fontFamily"} {
					if value := mark.AttrString(name); value != "" {
						out[name] = value
					}
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
