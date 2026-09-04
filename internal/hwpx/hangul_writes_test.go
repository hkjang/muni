package hwpx

import (
	"archive/zip"
	"bytes"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

// The package written is shaped like the one Hangul writes, part for part
// and element for element, because Hangul is the reader that matters and
// has never met one shaped any other way.

func partsOf(t *testing.T, built []byte) map[string]string {
	t.Helper()
	archive, err := zip.NewReader(bytes.NewReader(built), int64(len(built)))
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string]string{}
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(reader)
		reader.Close()
		parts[file.Name] = string(data)
	}
	return parts
}

func TestThePackageHasThePartsHangulWrites(t *testing.T) {
	built, err := Build(everyNode(t), Options{Title: "부품"})
	if err != nil {
		t.Fatal(err)
	}
	parts := partsOf(t, built)
	for _, name := range []string{"mimetype", "version.xml", "META-INF/container.xml", "META-INF/container.rdf", "META-INF/manifest.xml", "Contents/content.hpf", "Contents/header.xml", "Contents/section0.xml", "Preview/PrvText.txt", "settings.xml"} {
		if _, ok := parts[name]; !ok {
			t.Errorf("%s가 없습니다", name)
		}
	}
	if !strings.Contains(parts["version.xml"], `major="5" minor="1"`) {
		t.Errorf("판이 한글의 것이 아닙니다: %s", parts["version.xml"])
	}
	if !strings.Contains(parts["META-INF/container.xml"], "Preview/PrvText.txt") {
		t.Errorf("미리보기가 container.xml에 없습니다")
	}
	if preview := parts["Preview/PrvText.txt"]; !strings.Contains(preview, "한글") && len(preview) == 0 {
		t.Errorf("미리보기가 비었습니다")
	}
}

// The editor holds a column's width in pixels; Hangul wants it in HWPUNIT,
// summing to the width the table declares. Sharing the text column out evenly
// threw away every width a document had been carrying since it was read.
func TestATablesColumnWidthsAreWrittenInProportion(t *testing.T) {
	cell := func(text string, widths ...any) *richdoc.Node {
		node := &richdoc.Node{Type: "tableCell", Content: []*richdoc.Node{richdoc.Paragraph(richdoc.Text(text))}}
		node.SetAttr("colspan", len(widths))
		node.SetAttr("rowspan", 1)
		node.SetAttr("colwidth", widths)
		return node
	}
	document := &richdoc.Node{Type: "doc", Content: []*richdoc.Node{{Type: "table", Content: []*richdoc.Node{
		{Type: "tableRow", Content: []*richdoc.Node{cell("머리글", 50, 150)}},
		{Type: "tableRow", Content: []*richdoc.Node{cell("좁은칸", 50), cell("넓은칸", 150)}},
	}}}}
	built, err := Build(document, Options{Title: "너비"})
	if err != nil {
		t.Fatal(err)
	}
	section := partsOf(t, built)["Contents/section0.xml"]
	found := regexp.MustCompile(`<hp:cellSz width="(\d+)"`).FindAllStringSubmatch(section, -1)
	if len(found) != 3 {
		t.Fatalf("칸 = %d개", len(found))
	}
	// A quarter and three quarters of the six-inch text column, and the
	// merged cell across the whole of it.
	for index, want := range []string{"43200", "10800", "32400"} {
		if found[index][1] != want {
			t.Errorf("%d번째 칸의 너비 = %s, 원하는 것 = %s", index, found[index][1], want)
		}
	}
}

// A cell's shade is written where HWPX keeps one: a borderFill of its own in
// the header, which the cell names. Writing nothing lost every shade a table
// carried — the colour a reviewer painted a row of a report in came back as
// a plain white table the moment it was exported.
func TestACellsShadeIsWrittenAsABorderFillOfItsOwn(t *testing.T) {
	cell := func(text, shade string) *richdoc.Node {
		node := &richdoc.Node{Type: "tableCell", Content: []*richdoc.Node{richdoc.Paragraph(richdoc.Text(text))}}
		node.SetAttr("colspan", 1)
		node.SetAttr("rowspan", 1)
		if shade != "" {
			node.SetAttr("backgroundColor", shade)
		}
		return node
	}
	document := &richdoc.Node{Type: "doc", Content: []*richdoc.Node{{Type: "table", Content: []*richdoc.Node{
		{Type: "tableRow", Content: []*richdoc.Node{cell("음영칸", "#d9e2f3"), cell("같은음영칸", "#d9e2f3")}},
		{Type: "tableRow", Content: []*richdoc.Node{cell("맨칸", ""), cell("흰칸", "#ffffff")}},
	}}}}
	built, err := Build(document, Options{Title: "칸음영"})
	if err != nil {
		t.Fatal(err)
	}
	parts := partsOf(t, built)
	header, body := parts["Contents/header.xml"], parts["Contents/section0.xml"]
	// The three every file carries, and one more for the one colour used.
	if !strings.Contains(header, `<hh:borderFills itemCnt="4">`) ||
		!strings.Contains(header, `<hh:borderFill id="4"`) ||
		!strings.Contains(header, `<hc:winBrush faceColor="#D9E2F3"`) {
		t.Errorf("음영이 borderFill 로 쓰이지 않았습니다: %s", header)
	}
	// Two cells the same colour share one fill; the unshaded ones keep the
	// table's own.
	if got := strings.Count(body, `borderFillIDRef="4"`); got != 2 {
		t.Errorf("4번 채우기를 쓰는 칸 = %d개", got)
	}
	if got := strings.Count(body, `<hp:tc name="" header="0" hasMargin="0" protect="0" editable="0" dirty="0" borderFillIDRef="3">`); got != 2 {
		t.Errorf("음영 없는 칸이 표의 채우기를 쓰지 않았습니다: %d개", got)
	}

	back, _, _, err := Parse(built)
	if err != nil {
		t.Fatal(err)
	}
	table := back.Content[0]
	if table.Type != "table" {
		t.Fatalf("표가 돌아오지 않았습니다: %s", table.Type)
	}
	for _, want := range []struct {
		row, column int
		shade       string
	}{{0, 0, "#d9e2f3"}, {0, 1, "#d9e2f3"}, {1, 0, ""}, {1, 1, ""}} {
		got := table.Content[want.row].Content[want.column].AttrString("backgroundColor")
		if got != want.shade {
			t.Errorf("%d행 %d칸의 왕복한 음영 = %q, 원하는 것 = %q", want.row, want.column, got, want.shade)
		}
	}
}

// onePixel is the smallest PNG there is, for a picture to be written.
var onePixel = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
	0x89, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9C, 0x63, 0xF8, 0xCF, 0xC0, 0xF0,
	0x1F, 0x00, 0x05, 0x00, 0x01, 0xFF, 0x89, 0x8D, 0x0B, 0xE2, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45,
	0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
}

func TestTheElementsAreShapedLikeHanguls(t *testing.T) {
	built, err := Build(everyNode(t), Options{Title: "모양", ResolveImage: func(string) (Image, bool) {
		return Image{Data: onePixel, MediaType: "image/png"}, true
	}})
	if err != nil {
		t.Fatal(err)
	}
	parts := partsOf(t, built)
	header, body := parts["Contents/header.xml"], parts["Contents/section0.xml"]

	// A run names its font by number, through a table that lists it.
	if !regexp.MustCompile(`<hh:fontRef hangul="\d+"`).MatchString(header) {
		t.Errorf("fontRef가 번호가 아닙니다")
	}
	if !strings.Contains(header, `<hh:fontface lang="HANGUL"`) || !strings.Contains(header, `<hh:typeInfo `) {
		t.Errorf("글꼴 표가 한글의 모양이 아닙니다")
	}
	// Every id the body refers to is defined: tab 0, borders 1-3.
	for _, want := range []string{`<hh:tabPr id="0"`, `<hh:borderFill id="1"`, `<hh:borderFill id="2"`, `<hh:borderFill id="3"`, `<hh:numbering id="1"`, `<hh:bullet id="1"`} {
		if !strings.Contains(header, want) {
			t.Errorf("%s가 없습니다", want)
		}
	}
	// The margins sit inside the switch, and the list order of refList is
	// Hangul's.
	if !strings.Contains(header, `<hp:switch><hp:case`) {
		t.Errorf("여백이 switch 안에 있지 않습니다")
	}
	order := []string{"<hh:fontfaces", "<hh:borderFills", "<hh:charProperties", "<hh:tabProperties", "<hh:numberings", "<hh:bullets", "<hh:paraProperties", "<hh:styles"}
	last := -1
	for _, name := range order {
		at := strings.Index(header, name)
		if at < last {
			t.Errorf("%s의 순서가 어긋났습니다", name)
		}
		last = at
	}

	// A paragraph is numbered and carries Hangul's flags.
	if !regexp.MustCompile(`<hp:p id="\d+" paraPrIDRef="\d+" styleIDRef="\d+" pageBreak="[01]" columnBreak="0" merged="0">`).MatchString(body) {
		t.Errorf("문단 여는 태그가 한글의 모양이 아닙니다")
	}
	// The cell's paragraphs come first and its address after, and the
	// vertical alignment is on the list.
	cell := regexp.MustCompile(`<hp:tc [^>]*><hp:subList [^>]*vertAlign="[A-Z]+"[^>]*>.*?</hp:subList><hp:cellAddr `)
	if !cell.MatchString(body) {
		t.Errorf("칸의 차례가 한글의 것이 아닙니다")
	}
	if strings.Contains(body, `header="true"`) || strings.Contains(body, `repeatHeader="true"`) {
		t.Errorf("참/거짓이 1/0이 아닙니다")
	}
	// A picture carries every element Hangul writes for one, in order.
	for _, want := range []string{"<hp:offset ", "<hp:orgSz ", "<hp:curSz ", "<hp:flip ", "<hp:rotationInfo ", "<hp:renderingInfo>", "<hp:imgClip ", "<hp:imgDim ", "<hp:effects/>", "<hp:outMargin "} {
		if !strings.Contains(body, want) {
			t.Errorf("그림에 %s가 없습니다", want)
		}
	}
	if !regexp.MustCompile(`<hp:pic id="\d+" zOrder="\d+" numberingType="PICTURE"`).MatchString(body) {
		t.Errorf("그림 id가 번호가 아닙니다")
	}
	// The section carries note settings and page borders after the paper.
	for _, want := range []string{"<hp:footNotePr>", "<hp:endNotePr>", `<hp:pageBorderFill type="BOTH"`, "<hp:colPr "} {
		if !strings.Contains(body, want) {
			t.Errorf("구역 정의에 %s가 없습니다", want)
		}
	}
}

func TestListsAreWrittenAsShapesAndComeBackAsLists(t *testing.T) {
	document := everyNode(t)
	back := roundTrip(t, document)
	for _, kind := range []string{"bulletList", "orderedList", "listItem"} {
		if before, after := countKind(document, kind), countKind(back, kind); before == 0 || before != after {
			t.Errorf("%s: %d → %d", kind, before, after)
		}
	}
	built, _ := Build(document, Options{Title: "목록"})
	body := partsOf(t, built)["Contents/section0.xml"]
	if strings.Contains(body, "<hp:t>• ") || strings.Contains(body, "<hp:t>1. ") {
		t.Errorf("목록 표시가 글자로 쓰였습니다")
	}
}

// A highlight is the charPr's shadeColor, and the runs nobody marked keep
// the "none" Hangul writes for them. Writing the colour on every charPr would
// shade the whole document; writing it on none of them threw the mark away,
// which is what every .hwpx muni exported used to do.
func TestAHighlightIsWrittenAsTheShadeBehindTheWords(t *testing.T) {
	marked := richdoc.Text("형광펜친글")
	marked.Marks = []richdoc.Mark{{Type: "highlight", Attrs: map[string]any{"color": "#fff3a3"}}}
	document := richdoc.Doc(richdoc.Paragraph(marked, richdoc.Text("맨글")))
	built, err := Build(document, Options{Title: "음영"})
	if err != nil {
		t.Fatal(err)
	}
	header := partsOf(t, built)["Contents/header.xml"]
	if !strings.Contains(header, `shadeColor="#FFF3A3"`) {
		t.Errorf("음영 색이 charPr 에 쓰이지 않았습니다: %s", header)
	}
	if !strings.Contains(header, `shadeColor="none"`) {
		t.Errorf("음영 없는 글자 모양이 none 이 아닙니다")
	}
	back, _, _, err := Parse(built)
	if err != nil {
		t.Fatal(err)
	}
	if color := markAttr(t, back, "형광펜친글", "highlight", "color"); color != "#FFF3A3" {
		t.Errorf("왕복한 음영 색 = %q", color)
	}
	if hasMarkOn(back, "맨글", "highlight") {
		t.Errorf("음영 없던 글에 음영이 붙어 돌아왔습니다")
	}
}

func TestTheHeaderAndFooterAreWrittenAndReadBack(t *testing.T) {
	built, err := Build(everyNode(t), Options{Title: "머리말", Header: "회의록 — 대외비", Footer: "무니"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, meta, err := Parse(built)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Header != "회의록 — 대외비" || meta.Footer != "무니" {
		t.Errorf("머리말/꼬리말 = %q / %q", meta.Header, meta.Footer)
	}
}

func countKind(node *richdoc.Node, kind string) int {
	if node == nil {
		return 0
	}
	total := 0
	if node.Type == kind {
		total++
	}
	for _, child := range node.Content {
		total += countKind(child, kind)
	}
	return total
}
