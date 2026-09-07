package pdfx

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

// What real PDFs taught. Each of these is a shape a hand-made fixture would
// never have had: they come from reading what a browser and a word processor
// actually write when they print a Korean document.

// trueTypeWithCmap builds the smallest font this reader cares about: an sfnt
// carrying nothing but a character map, in the format every Windows font
// uses. Each character gets a segment of its own.
func trueTypeWithCmap(t *testing.T, mapping map[rune]uint32) []byte {
	t.Helper()
	codes := make([]rune, 0, len(mapping))
	for code := range mapping {
		codes = append(codes, code)
	}
	for index := 1; index < len(codes); index++ {
		for back := index; back > 0 && codes[back] < codes[back-1]; back-- {
			codes[back], codes[back-1] = codes[back-1], codes[back]
		}
	}
	segments := len(codes) + 1 // the format demands a final 0xFFFF segment

	var body bytes.Buffer
	put := func(value uint16) { _ = binary.Write(&body, binary.BigEndian, value) }
	put(4)                       // format
	put(uint16(16 + segments*8)) // length
	put(0)                       // language
	put(uint16(segments * 2))    // segCountX2
	put(0)                       // searchRange
	put(0)                       // entrySelector
	put(0)                       // rangeShift
	for _, code := range codes { // endCode
		put(uint16(code))
	}
	put(0xFFFF)
	put(0)                       // reservedPad
	for _, code := range codes { // startCode
		put(uint16(code))
	}
	put(0xFFFF)
	for _, code := range codes { // idDelta
		put(uint16(mapping[code] - uint32(code)))
	}
	put(1)
	for range codes { // idRangeOffset
		put(0)
	}
	put(0)

	var table bytes.Buffer
	_ = binary.Write(&table, binary.BigEndian, uint16(0)) // version
	_ = binary.Write(&table, binary.BigEndian, uint16(1)) // one subtable
	_ = binary.Write(&table, binary.BigEndian, uint16(3)) // Windows
	_ = binary.Write(&table, binary.BigEndian, uint16(1)) // basic plane
	_ = binary.Write(&table, binary.BigEndian, uint32(12))
	table.Write(body.Bytes())

	var font bytes.Buffer
	_ = binary.Write(&font, binary.BigEndian, uint32(0x00010000))
	_ = binary.Write(&font, binary.BigEndian, uint16(1)) // one table
	_ = binary.Write(&font, binary.BigEndian, uint16(0))
	_ = binary.Write(&font, binary.BigEndian, uint16(0))
	_ = binary.Write(&font, binary.BigEndian, uint16(0))
	font.WriteString("cmap")
	_ = binary.Write(&font, binary.BigEndian, uint32(0))
	_ = binary.Write(&font, binary.BigEndian, uint32(28))
	_ = binary.Write(&font, binary.BigEndian, uint32(table.Len()))
	font.Write(table.Bytes())
	return font.Bytes()
}

// subsetFontPDF assembles a PDF around one subset CID font, the way a browser
// prints one: the text is addressed by glyph number and the /ToUnicode map
// says what the glyphs mean.
func subsetFontPDF(t *testing.T, fontFile []byte, toUnicode string, content string, actualText bool) []byte {
	t.Helper()
	var out bytes.Buffer
	object := func(number int, body string) {
		out.WriteString(fmt.Sprintf("%d 0 obj %s endobj\n", number, body))
	}
	stream := func(number int, extra string, data []byte) {
		out.WriteString(fmt.Sprintf("%d 0 obj <</Length %d%s>>\nstream\n", number, len(data), extra))
		out.Write(data)
		out.WriteString("\nendstream endobj\n")
	}
	out.WriteString("%PDF-1.7\n")
	object(1, "<</Type/Catalog/Pages 2 0 R>>")
	object(2, "<</Type/Pages/Kids[3 0 R]/Count 1>>")
	properties := ""
	if actualText {
		properties = "/Properties<</MC0<</ActualText(\\376\\377\\000 )>>>>"
	}
	object(3, "<</Type/Page/Parent 2 0 R/MediaBox[0 0 595 842]/Resources<</Font<</F1 4 0 R>>"+properties+">>/Contents 8 0 R>>")
	object(4, "<</Type/Font/Subtype/Type0/BaseFont/AAAAAA+Test/Encoding/Identity-H/DescendantFonts[5 0 R]/ToUnicode 7 0 R>>")
	object(5, "<</Type/Font/Subtype/CIDFontType2/BaseFont/AAAAAA+Test"+
		"/CIDSystemInfo<</Registry(Adobe)/Ordering(Identity)/Supplement 0>>"+
		"/FontDescriptor 6 0 R/DW 1000/CIDToGIDMap/Identity>>")
	object(6, "<</Type/FontDescriptor/FontName/AAAAAA+Test/Flags 4/FontFile2 9 0 R>>")
	stream(7, "", []byte(toUnicode))
	stream(8, "", []byte(content))
	stream(9, "", fontFile)
	out.WriteString("trailer <</Root 1 0 R/Size 10>>\n%%EOF\n")
	return out.Bytes()
}

const selfMappedSpaceCMap = `/CIDInit /ProcSet findresource begin
begincmap
1 begincodespacerange
<0000> <FFFF>
endcodespacerange
3 beginbfchar
<0024> <0024>
<0100> <AC00>
<0101> <B098>
endbfchar
endcmap
end
`

// A producer that cannot name a glyph writes the glyph's own number as its
// Unicode. The space of this font sits at glyph 36, so the map says "$" — and
// a reader that believes it puts a dollar sign between every two words. The
// font's own character map says glyph 36 draws a space.
func TestASelfMappedGlyphIsReadFromTheFontNotTheProducersGuess(t *testing.T) {
	font := trueTypeWithCmap(t, map[rune]uint32{' ': 0x24, '가': 0x100, '나': 0x101})
	content := "BT /F1 12 Tf 72 700 Td <0100> Tj <0024> Tj <0101> Tj ET"
	result, err := Import(context.Background(), subsetFontPDF(t, font, selfMappedSpaceCMap, content, false))
	if err != nil {
		t.Fatal(err)
	}
	text := result.Document.PlainText()
	if strings.Contains(text, "$") {
		t.Errorf("띄어쓰기가 달러 기호로 나왔습니다: %q", text)
	}
	if !strings.Contains(text, "가 나") {
		t.Errorf("본문 = %q", text)
	}
}

// The same failure wearing another mask: the glyph number lands in the
// control range, and the words arrive glued together around a U+0001.
func TestAControlCharacterIsNeverText(t *testing.T) {
	cmap := strings.Replace(selfMappedSpaceCMap, "<0024> <0024>", "<0024> <0001>", 1)
	font := trueTypeWithCmap(t, map[rune]uint32{' ': 0x24, '가': 0x100, '나': 0x101})
	content := "BT /F1 12 Tf 72 700 Td <0100> Tj <0024> Tj <0101> Tj ET"
	result, err := Import(context.Background(), subsetFontPDF(t, font, cmap, content, false))
	if err != nil {
		t.Fatal(err)
	}
	text := result.Document.PlainText()
	for _, r := range text {
		if r < 0x20 && r != '\n' && r != '\t' {
			t.Fatalf("제어 문자가 본문에 남았습니다: %q", text)
		}
	}
	if !strings.Contains(text, "가 나") {
		t.Errorf("본문 = %q", text)
	}
}

// When the font itself cannot be read, the producer's own correction is the
// last word: it wraps the glyphs it could not describe and says what they say.
func TestMarkedContentActualTextWins(t *testing.T) {
	font := []byte("not a font")
	content := "BT /F1 12 Tf 72 700 Td <0100> Tj /Span /MC0 BDC <0024> Tj EMC <0101> Tj ET"
	result, err := Import(context.Background(), subsetFontPDF(t, font, selfMappedSpaceCMap, content, true))
	if err != nil {
		t.Fatal(err)
	}
	text := result.Document.PlainText()
	if strings.Contains(text, "$") || !strings.Contains(text, "가 나") {
		t.Errorf("본문 = %q", text)
	}
}

// A centred heading over left-aligned names and right-aligned figures is what
// every report table looks like. Grouping columns by their left edge gives
// each alignment a column of its own — four columns of content arrived as
// seven, three of them empty.
func TestColumnsAreFoundByOverlapNotByLeftEdge(t *testing.T) {
	row := func(y int, cells ...[2]any) string {
		out := ""
		for _, cell := range cells {
			out += fmt.Sprintf("BT /F2 10 Tf %d %d Td (%s) Tj ET\n", cell[0].(int), y, cell[1].(string))
		}
		return out
	}
	content := row(700, [2]any{100, "Dept"}, [2]any{210, "Item"}, [2]any{400, "Amount"}, [2]any{520, "Rate"}) +
		row(680, [2]any{60, "Planning"}, [2]any{205, "Upgrade"}, [2]any{430, "125,000"}, [2]any{515, "82.4%"}) +
		row(660, [2]any{60, "General"}, [2]any{205, "Facility"}, [2]any{435, "64,300"}, [2]any{515, "91.0%"})
	result, err := Import(context.Background(), buildPDF(content, false))
	if err != nil {
		t.Fatal(err)
	}
	var table *richdoc.Node
	var walk func(*richdoc.Node)
	walk = func(node *richdoc.Node) {
		if node == nil {
			return
		}
		if node.Type == "table" && table == nil {
			table = node
		}
		for _, child := range node.Content {
			walk(child)
		}
	}
	walk(result.Document)
	if table == nil {
		t.Fatalf("표가 나오지 않았습니다: %q", result.Document.PlainText())
	}
	for index, tableRow := range table.Content {
		if len(tableRow.Content) != 4 {
			t.Fatalf("%d번째 행의 칸 = %d개 (4개여야 합니다)", index, len(tableRow.Content))
		}
		for _, cell := range tableRow.Content {
			if strings.TrimSpace(cell.PlainText()) == "" {
				t.Errorf("%d번째 행에 빈 칸이 생겼습니다: %q", index, tableRow.PlainText())
			}
		}
	}
}

// pagesPDF assembles a PDF of several pages sharing one font, so the tests can
// say what belongs to the paper and what belongs to the document.
func pagesPDF(contents []string) []byte {
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	out.WriteString("1 0 obj <</Type/Catalog/Pages 2 0 R>> endobj\n")
	kids := ""
	for index := range contents {
		kids += fmt.Sprintf("%d 0 R ", 4+index*2)
	}
	out.WriteString(fmt.Sprintf("2 0 obj <</Type/Pages/Kids[%s]/Count %d>> endobj\n", strings.TrimSpace(kids), len(contents)))
	out.WriteString("3 0 obj <</Type/Font/Subtype/Type1/BaseFont/Helvetica/Encoding/WinAnsiEncoding>> endobj\n")
	for index, content := range contents {
		page, stream := 4+index*2, 5+index*2
		out.WriteString(fmt.Sprintf("%d 0 obj <</Type/Page/Parent 2 0 R/MediaBox[0 0 595 842]"+
			"/Resources<</Font<</F2 3 0 R>>>>/Contents %d 0 R>> endobj\n", page, stream))
		out.WriteString(fmt.Sprintf("%d 0 obj <</Length %d>>\nstream\n%s\nendstream endobj\n", stream, len(content), content))
	}
	out.WriteString(fmt.Sprintf("trailer <</Root 1 0 R/Size %d>>\n%%%%EOF\n", 4+len(contents)*2))
	return out.Bytes()
}

func at(x, y int, text string) string {
	return fmt.Sprintf("BT /F2 11 Tf %d %d Td (%s) Tj ET\n", x, y, text)
}

// tableRow places one row of a four-column table.
func tableRowAt(y int, cells ...string) string {
	out := ""
	for index, text := range cells {
		out += at(60+index*140, y, text)
	}
	return out
}

// The running head and the page number belong to the paper. A browser prints
// the document's title across the top of every page and the address and
// "1/2" across the foot, and all of it used to arrive as text.
func TestTheRunningHeadAndPageNumberAreNotTheDocument(t *testing.T) {
	page := func(number int, body string) string {
		return at(60, 800, "Quarterly Report - Finance Division") + body +
			at(60, 30, fmt.Sprintf("- %d -", number))
	}
	pdf := pagesPDF([]string{
		page(1, at(60, 700, "The first paragraph of the document.")),
		page(2, at(60, 700, "The second page carries on the argument.")),
	})
	result, err := Import(context.Background(), pdf)
	if err != nil {
		t.Fatal(err)
	}
	text := result.Document.PlainText()
	for _, unwanted := range []string{"Quarterly Report", "- 1 -", "- 2 -"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("%q가 본문에 남았습니다:\n%s", unwanted, text)
		}
	}
	for _, wanted := range []string{"first paragraph", "second page"} {
		if !strings.Contains(text, wanted) {
			t.Errorf("%q가 사라졌습니다:\n%s", wanted, text)
		}
	}
}

func firstTable(node *richdoc.Node) *richdoc.Node {
	if node == nil {
		return nil
	}
	if node.Type == "table" {
		return node
	}
	for _, child := range node.Content {
		if found := firstTable(child); found != nil {
			return found
		}
	}
	return nil
}

func countTables(node *richdoc.Node) int {
	if node == nil {
		return 0
	}
	total := 0
	if node.Type == "table" {
		total++
	}
	for _, child := range node.Content {
		total += countTables(child)
	}
	return total
}

// A table too long for one page runs to the foot and starts again at the head
// of the next. It is one table, and the running head that used to sit between
// the halves made it look like two with a stray line between them.
func TestATableCarriedOverAPageBreakIsOneTable(t *testing.T) {
	head := at(60, 800, "Quarterly Report - Finance Division")
	first := head + at(60, 700, "Departmental spending") +
		tableRowAt(660, "Dept", "Project", "Amount", "Rate") +
		tableRowAt(640, "Planning", "Upgrade", "125,000", "82.4%") +
		tableRowAt(620, "General", "Facility", "64,300", "91.0%")
	second := head +
		tableRowAt(760, "Safety", "Response", "210,500", "55.7%") +
		tableRowAt(740, "Press", "Outreach", "33,000", "70.2%")
	result, err := Import(context.Background(), pagesPDF([]string{first, second}))
	if err != nil {
		t.Fatal(err)
	}
	if count := countTables(result.Document); count != 1 {
		t.Fatalf("표가 %d개입니다:\n%s", count, result.Document.PlainText())
	}
	table := firstTable(result.Document)
	if len(table.Content) != 5 {
		t.Fatalf("행 = %d개 (5개여야 합니다): %q", len(table.Content), table.PlainText())
	}
	for index, row := range table.Content {
		if len(row.Content) != 4 {
			t.Errorf("%d번째 행의 칸 = %d개", index, len(row.Content))
		}
	}
	if !strings.Contains(table.PlainText(), "Outreach") {
		t.Errorf("이어진 행이 표에 들어오지 않았습니다: %q", table.PlainText())
	}
}

// A heading centred over its columns overlaps neither the names on the left
// nor the figures on the right, and was left behind as a stray paragraph.
func TestACentredHeadingJoinsItsTable(t *testing.T) {
	content := at(150, 700, "Item") + at(430, 700, "Value") +
		at(60, 680, "First") + at(520, 680, "1") +
		at(60, 660, "Second") + at(520, 660, "22")
	result, err := Import(context.Background(), buildPDF(content, false))
	if err != nil {
		t.Fatal(err)
	}
	table := firstTable(result.Document)
	if table == nil {
		t.Fatalf("표가 없습니다: %q", result.Document.PlainText())
	}
	if len(table.Content) != 3 {
		t.Fatalf("행 = %d개 (머리글 포함 3개여야 합니다): %q", len(table.Content), table.PlainText())
	}
	heading := table.Content[0].PlainText()
	if !strings.Contains(heading, "Item") || !strings.Contains(heading, "Value") {
		t.Errorf("머리글 행 = %q", heading)
	}
	for index, row := range table.Content {
		if len(row.Content) != 2 {
			t.Errorf("%d번째 행의 칸 = %d개", index, len(row.Content))
		}
	}
}

// The format is exact: word spacing applies to the single-byte code 32 and to
// nothing else. Adding it to the two-byte codes a Korean font uses walks the
// pen past where the page put it, and the gaps that tell words and table
// columns apart go with it.
func TestWordSpacingAppliesOnlyToASingleByteSpace(t *testing.T) {
	simple := &fontInfo{toUnicode: map[uint32]string{}, encoding: map[byte]string{}, widths: map[uint32]float64{}}
	if !simple.takesWordSpacing(charCode{value: 32, width: 1}) {
		t.Errorf("한 바이트 빈칸에 낱말 간격이 붙지 않았습니다")
	}
	if simple.takesWordSpacing(charCode{value: 32, width: 2}) {
		t.Errorf("두 바이트 코드에 낱말 간격이 붙었습니다")
	}
	if simple.takesWordSpacing(charCode{value: 0x0020, width: 2}) {
		t.Errorf("두 바이트로 쓰인 32번에 낱말 간격이 붙었습니다")
	}
	if simple.takesWordSpacing(charCode{value: 0x24, width: 1}) {
		t.Errorf("빈칸이 아닌 코드에 낱말 간격이 붙었습니다")
	}
}

// A typesetter's ligature is one glyph holding several letters. Left as it is,
// searching a document for "file" does not find "ﬁle".
func TestLigaturesComeBackAsLetters(t *testing.T) {
	if got := readableText("ﬁle"); got != "file" {
		t.Errorf("합자 = %q", got)
	}
	if got := readableText("ofﬂoading"); got != "offloading" {
		t.Errorf("합자 = %q", got)
	}
	// A soft hyphen guides typesetting and says nothing.
	if got := readableText("문­서​"); got != "문서" {
		t.Errorf("보이지 않는 표시가 남았습니다: %q", got)
	}
	if got := readableText("보통 글자"); got != "보통 글자" {
		t.Errorf("멀쩡한 글자가 바뀌었습니다: %q", got)
	}
}
