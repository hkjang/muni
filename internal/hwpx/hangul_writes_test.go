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
