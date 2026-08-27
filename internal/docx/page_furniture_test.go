package docx

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

func plainDoc(t *testing.T) *richdoc.Node {
	t.Helper()
	node, err := richdoc.Parse(json.RawMessage(
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"본문"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	return node
}

// A Korean office document carries its classification on the page — 대외비 in
// the header, the department beside it — and importing one used to drop those
// parts of the file entirely and say nothing. Losing "대외비" is not a loss of
// formatting.
func TestThePageHeaderSurvivesTheRoundTrip(t *testing.T) {
	built, err := Build(plainDoc(t), Options{
		Title:  "상반기 보고",
		Header: "기획조정실 · 대외비",
		Footer: "내부 열람용",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, meta, err := Parse(built)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Header != "기획조정실 · 대외비" {
		t.Errorf("머리글 = %q", meta.Header)
	}
	if meta.Footer != "내부 열람용" {
		t.Errorf("바닥글 = %q", meta.Footer)
	}
}

func TestNoHeaderMeansNoHeaderPart(t *testing.T) {
	// A part in the package with no reference to it, or a reference with no
	// part, is a file Word calls corrupt. With nothing to say, neither is
	// written.
	built, err := Build(plainDoc(t), Options{Title: "머리글 없는 문서"})
	if err != nil {
		t.Fatal(err)
	}
	xml := documentXMLOf(t, built)
	if contains(xml, "headerReference") {
		t.Error("빈 머리글에 참조가 남았습니다")
	}
	if packageHas(t, built, "word/header1.xml") {
		t.Error("word/header1.xml 가 쓰였습니다")
	}
	// And it still opens, with an empty header rather than a missing one.
	if _, _, meta, err := Parse(built); err != nil || meta.Header != "" {
		t.Errorf("다시 읽기 실패: %v %q", err, meta.Header)
	}
}

// The footer is the exception: it is written even when it is empty, because it
// carries the page number.
func TestNoFooterTextStillNumbersThePages(t *testing.T) {
	built, err := Build(plainDoc(t), Options{Title: "바닥글 없는 문서"})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(documentXMLOf(t, built), "footerReference") {
		t.Fatal("바닥글 참조가 없습니다")
	}
	footer := partOf(t, built, "word/footer1.xml")
	if !strings.Contains(footer, "PAGE") || !strings.Contains(footer, "NUMPAGES") {
		t.Errorf("쪽 번호가 없습니다:\n%s", footer)
	}
	// Reading it back must not invent footer text out of the counter.
	if _, _, meta, err := Parse(built); err != nil || meta.Footer != "" {
		t.Errorf("바닥글 = %q (%v)", meta.Footer, err)
	}
}

func TestAHeaderLaidOutInColumnsComesBackAsOneLine(t *testing.T) {
	// Word builds "기획조정실 <tab> 대외비" out of separate runs so it prints as
	// two columns. muni holds one line, and joining the runs without a space
	// would give "기획조정실대외비".
	built, err := Build(plainDoc(t), Options{Header: "기획조정실   ·   대외비"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, meta, err := Parse(built)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Header != "기획조정실 · 대외비" {
		t.Errorf("머리글 = %q", meta.Header)
	}
}

func documentXMLOf(t *testing.T, built []byte) string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(built), int64(len(built)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range reader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		opened, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer opened.Close()
		raw, err := io.ReadAll(opened)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	t.Fatal("no document.xml")
	return ""
}

func packageHas(t *testing.T, built []byte, name string) bool {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(built), int64(len(built)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range reader.File {
		if file.Name == name {
			return true
		}
	}
	return false
}

func contains(haystack, needle string) bool { return strings.Contains(haystack, needle) }

// A Korean office document is full of wide tables, and printing one sideways
// is routine in Word. muni had A4 portrait written into the code, so it was
// not possible at all.
func TestALandscapeDocumentTurnsThePageAndTheTextColumn(t *testing.T) {
	built, err := Build(plainDoc(t), Options{Title: "가로 문서", Landscape: true})
	if err != nil {
		t.Fatal(err)
	}
	xml := documentXMLOf(t, built)
	// Word reads w:orient, not the fact that the numbers were swapped.
	if !contains(xml, `w:orient="landscape"`) {
		t.Error("w:orient가 없습니다")
	}
	if !contains(xml, `w:w="16838"`) || !contains(xml, `w:h="11906"`) {
		t.Errorf("용지 크기가 돌지 않았습니다: %.200s", xml)
	}
	_, _, meta, err := Parse(built)
	if err != nil {
		t.Fatal(err)
	}
	if !meta.Landscape {
		t.Error("가로 방향이 되읽히지 않았습니다")
	}
}

func TestAPortraitDocumentSaysNothingAboutOrientation(t *testing.T) {
	built, err := Build(plainDoc(t), Options{Title: "세로 문서"})
	if err != nil {
		t.Fatal(err)
	}
	if contains(documentXMLOf(t, built), "w:orient") {
		t.Error("세로 문서에 w:orient가 붙었습니다")
	}
	if _, _, meta, err := Parse(built); err != nil || meta.Landscape {
		t.Errorf("세로로 읽혀야 합니다: %v %v", err, meta.Landscape)
	}
}

func TestATableFillsTheWiderPage(t *testing.T) {
	// The point of turning the page is the extra width. A table sized against
	// the portrait column would leave it unused.
	source := `{"type":"doc","content":[{"type":"table","content":[
		{"type":"tableRow","content":[
			{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"가"}]}]},
			{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"나"}]}]}]}]}]}`
	node, err := richdoc.Parse(json.RawMessage(source))
	if err != nil {
		t.Fatal(err)
	}
	portrait, err := Build(node, Options{})
	if err != nil {
		t.Fatal(err)
	}
	landscape, err := Build(node, Options{Landscape: true})
	if err != nil {
		t.Fatal(err)
	}
	if widthOfGrid(t, documentXMLOf(t, landscape)) <= widthOfGrid(t, documentXMLOf(t, portrait)) {
		t.Error("가로 문서의 표가 넓어지지 않았습니다")
	}
}

// widthOfGrid adds up the column widths the table declares.
func widthOfGrid(t *testing.T, xml string) int {
	t.Helper()
	total := 0
	for _, match := range regexp.MustCompile(`<w:gridCol w:w="(\d+)"`).FindAllStringSubmatch(xml, -1) {
		value, err := strconv.Atoi(match[1])
		if err != nil {
			t.Fatal(err)
		}
		total += value
	}
	if total == 0 {
		t.Fatalf("표의 열 너비를 찾지 못했습니다: %.300s", xml)
	}
	return total
}

// muni's PDF export has always numbered the pages in the footer. The .docx
// export did not, so printing a document and exporting it to Word disagreed
// about whether it was paginated at all.
func TestTheFooterNumbersThePages(t *testing.T) {
	built, err := Build(plainDoc(t), Options{Title: "상반기 보고", Footer: "내부 열람용"})
	if err != nil {
		t.Fatal(err)
	}
	footer := partOf(t, built, "word/footer1.xml")
	for _, want := range []string{"내부 열람용", "PAGE", "NUMPAGES", `w:fldCharType="begin"`, `w:fldCharType="end"`} {
		if !strings.Contains(footer, want) {
			t.Errorf("%q 가 바닥글에 없습니다:\n%s", want, footer)
		}
	}
	// Word counts these itself; an unbalanced pair is a file it calls corrupt.
	if begins, ends := strings.Count(footer, `w:fldCharType="begin"`), strings.Count(footer, `w:fldCharType="end"`); begins != ends {
		t.Errorf("필드 시작 %d개, 끝 %d개", begins, ends)
	}
}

// A Word header is often a field and then something else — "[STYLEREF] · 대외비".
// Stopping at the field took the classification away with the counter, and
// losing "대외비" is not a loss of formatting.
func TestAHeaderKeepsTheWordsAroundItsFields(t *testing.T) {
	header := `<w:p>` +
		`<w:r><w:fldChar w:fldCharType="begin"/></w:r>` +
		`<w:r><w:instrText xml:space="preserve"> STYLEREF 1 </w:instrText></w:r>` +
		`<w:r><w:fldChar w:fldCharType="separate"/></w:r>` +
		`<w:r><w:t>지난 계산값</w:t></w:r>` +
		`<w:r><w:fldChar w:fldCharType="end"/></w:r>` +
		`<w:r><w:t> · 대외비</w:t></w:r></w:p>`
	got := collapseSpaces(furnitureText(mustParseXML(t, header)))
	if !strings.Contains(got, "대외비") {
		t.Errorf("머리글 = %q, 대외비 를 기대했습니다", got)
	}
	// What the field last calculated is not the header's text.
	if strings.Contains(got, "지난 계산값") {
		t.Errorf("머리글에 필드 계산값이 섞였습니다: %q", got)
	}
	// And the dot that only separated the field from the words goes with it.
	if strings.HasPrefix(strings.TrimSpace(got), "·") {
		t.Errorf("필드를 붙이던 구분점이 남았습니다: %q", got)
	}
}

// A separator the author typed, with words on both sides, is theirs.
func TestAHeaderKeepsPunctuationBetweenItsOwnWords(t *testing.T) {
	header := `<w:p><w:r><w:t>기획조정실 · 대외비</w:t></w:r></w:p>`
	if got := collapseSpaces(furnitureText(mustParseXML(t, header))); got != "기획조정실 · 대외비" {
		t.Errorf("머리글 = %q", got)
	}
}
