package hwpx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

// The writer is the reader's test. muni does not write .hwpx any other way,
// so before this every reader test fed it a file muni built by hand for that
// test — which proves the reader agrees with the hand, not with a document.
// A round trip through the writer is a document the reader did not see the
// making of.

func everyNode(t *testing.T) *richdoc.Node {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "frontend", "testdata", "every-node.json"))
	if err != nil {
		t.Fatal(err)
	}
	document, err := richdoc.Parse(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func roundTrip(t *testing.T, document *richdoc.Node) *richdoc.Node {
	t.Helper()
	built, err := Build(document, Options{Title: "왕복"})
	if err != nil {
		t.Fatal(err)
	}
	back, _, _, err := Parse(built)
	if err != nil {
		t.Fatalf("muni 가 쓴 파일을 muni 가 읽지 못했습니다: %v", err)
	}
	return back
}

// Every phrase the export coverage tests follow through the other formats.
var carriedPhrases = []string{
	"제목입니다", "평문과", "굵은글씨", "기울임", "밑줄", "취소선", "코드조각",
	"링크글자", "위첨자표시", "아래첨자표시", "형광펜표시", "글자서식표시",
	"각주내용입니다", "줄바꿈뒤문장", "인용문입니다", "글머리항목", "하위항목",
	"셋째항목", "번호항목", "할일항목", "코드블록내용", "graph TD",
	"표머리글", "표셀내용", "마지막문단",
}

func TestEverythingComesBackWhenTheFileIsReadAgain(t *testing.T) {
	back := roundTrip(t, everyNode(t))
	text := back.PlainText()
	for _, phrase := range carriedPhrases {
		if !strings.Contains(text, phrase) {
			t.Errorf("%q 가 왕복에서 사라졌습니다", phrase)
		}
	}
}

func hasMarkOn(node *richdoc.Node, phrase, mark string) bool {
	if node == nil {
		return false
	}
	if node.Type == "text" && strings.Contains(node.Text, phrase) {
		for _, m := range node.Marks {
			if m.Type == mark {
				return true
			}
		}
	}
	for _, child := range node.Content {
		if hasMarkOn(child, phrase, mark) {
			return true
		}
	}
	return false
}

// Carrying the words is the floor. The marks have to come back on the words
// they were on.
func TestTheMarksComeBackWithTheWordsTheyWereOn(t *testing.T) {
	back := roundTrip(t, everyNode(t))
	for _, want := range []struct{ phrase, mark string }{
		{"굵은글씨", "bold"},
		{"기울임", "italic"},
		{"밑줄", "underline"},
		{"취소선", "strike"},
		{"위첨자표시", "superscript"},
		{"아래첨자표시", "subscript"},
		{"글자서식표시", "textStyle"},
	} {
		if !hasMarkOn(back, want.phrase, want.mark) {
			t.Errorf("%q 가 %s 없이 돌아왔습니다", want.phrase, want.mark)
		}
	}
}

func blockTypesOf(document *richdoc.Node) []string {
	out := []string{}
	for _, block := range document.Content {
		out = append(out, block.Type)
	}
	return out
}

func TestHeadingsAndTablesKeepTheirShape(t *testing.T) {
	back := roundTrip(t, everyNode(t))
	types := blockTypesOf(back)
	headings, tables := 0, 0
	for _, kind := range types {
		switch kind {
		case "heading":
			headings++
		case "table":
			tables++
		}
	}
	if headings == 0 {
		t.Errorf("제목이 돌아오지 않았습니다: %v", types)
	}
	if tables != 1 {
		t.Errorf("표 = %d개: %v", tables, types)
	}
	for _, block := range back.Content {
		if block.Type != "table" {
			continue
		}
		head := block.Content[0].Content[0]
		if head.Type != "tableHeader" {
			t.Errorf("표머리글 행이 머리글로 돌아오지 않았습니다: %s", head.Type)
		}
	}
}

func TestParagraphLayoutSurvives(t *testing.T) {
	source := `{"type":"doc","content":[{"type":"paragraph","attrs":{"textAlign":"center","indent":2,"firstLine":true,"lineHeight":"1.6"},"content":[{"type":"text","text":"가운데 들여쓴 문단"}]}]}`
	node, err := richdoc.Parse(json.RawMessage(source))
	if err != nil {
		t.Fatal(err)
	}
	back := roundTrip(t, node)
	paragraph := back.Content[0]
	if got := paragraph.AttrString("textAlign"); got != "center" {
		t.Errorf("정렬 = %q", got)
	}
	if paragraph.AttrInt("indent", 0) != 2 {
		t.Errorf("들여쓰기 = %d", paragraph.AttrInt("indent", 0))
	}
	if first, _ := paragraph.Attr("firstLine").(bool); !first {
		t.Errorf("첫 줄 들여쓰기가 사라졌습니다: %v", paragraph.Attrs)
	}
	if got := paragraph.AttrString("lineHeight"); got != "1.6" {
		t.Errorf("줄간격 = %q", got)
	}
}

func TestAPictureSurvivesTheRoundTrip(t *testing.T) {
	pixel := []byte{
		0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0a, 'I', 'D', 'A', 'T', 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 'I', 'E', 'N', 'D', 0xae,
		0x42, 0x60, 0x82,
	}
	image := &richdoc.Node{Type: "image"}
	image.SetAttr("src", "/api/v1/attachments/abc")
	built, err := Build(richdoc.Doc(image), Options{Title: "그림", ResolveImage: func(string) (Image, bool) {
		return Image{Data: pixel, MediaType: "image/png"}, true
	}})
	if err != nil {
		t.Fatal(err)
	}
	back, assets, _, err := Parse(built)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || string(assets[0].Data) != string(pixel) {
		t.Fatalf("그림의 바이트가 돌아오지 않았습니다: %+v", assets)
	}
	if types := blockTypesOf(back); len(types) != 1 || types[0] != "image" {
		t.Errorf("블록 = %v", types)
	}
}

// The package opens with its type, stored rather than deflated, so a reader
// can tell what it has from the first bytes.
func TestTheMimetypeComesFirstAndUncompressed(t *testing.T) {
	built, err := Build(richdoc.Doc(richdoc.Paragraph(richdoc.Text("본문"))), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(built[:64]), "mimetype") {
		t.Error("mimetype 이 맨 앞이 아닙니다")
	}
	if !strings.Contains(string(built[:96]), "application/hwp+zip") {
		t.Error("mimetype 이 압축되어 있어 처음 바이트에서 읽히지 않습니다")
	}
}
