package docx

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

func sample() *richdoc.Node {
	raw := `{"type":"doc","content":[
	 {"type":"heading","attrs":{"level":1,"textAlign":"center"},"content":[{"type":"text","text":"보고서 제목"}]},
	 {"type":"paragraph","content":[
	   {"type":"text","marks":[{"type":"bold"}],"text":"굵게"},
	   {"type":"text","text":" 그리고 "},
	   {"type":"text","marks":[{"type":"italic"},{"type":"underline"}],"text":"기울임밑줄"},
	   {"type":"text","text":" 및 "},
	   {"type":"text","marks":[{"type":"link","attrs":{"href":"https://example.com"}}],"text":"링크"}]},
	 {"type":"bulletList","content":[
	   {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"첫 항목"}]},
	     {"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"중첩 항목"}]}]}]}]},
	   {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"둘째 항목"}]}]}]},
	 {"type":"orderedList","attrs":{"start":1},"content":[
	   {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"순번 하나"}]}]},
	   {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"순번 둘"}]}]}]},
	 {"type":"taskList","content":[
	   {"type":"taskItem","attrs":{"checked":true},"content":[{"type":"paragraph","content":[{"type":"text","text":"완료된 일"}]}]},
	   {"type":"taskItem","attrs":{"checked":false},"content":[{"type":"paragraph","content":[{"type":"text","text":"남은 일"}]}]}]},
	 {"type":"blockquote","content":[{"type":"paragraph","content":[{"type":"text","text":"인용문"}]}]},
	 {"type":"codeBlock","content":[{"type":"text","text":"fmt.Println(1)\nfmt.Println(2)"}]},
	 {"type":"horizontalRule"},
	 {"type":"table","content":[
	   {"type":"tableRow","content":[
	     {"type":"tableHeader","attrs":{"colspan":2,"rowspan":1,"colwidth":[200,300]},"content":[{"type":"paragraph","content":[{"type":"text","text":"머리글"}]}]}]},
	   {"type":"tableRow","content":[
	     {"type":"tableCell","attrs":{"colspan":1,"rowspan":2,"colwidth":[200]},"content":[{"type":"paragraph","content":[{"type":"text","text":"세로병합"}]}]},
	     {"type":"tableCell","attrs":{"colspan":1,"rowspan":1,"colwidth":[300]},"content":[{"type":"paragraph","content":[{"type":"text","text":"우상"}]}]}]},
	   {"type":"tableRow","content":[
	     {"type":"tableCell","attrs":{"colspan":1,"rowspan":1,"colwidth":[300]},"content":[{"type":"paragraph","content":[{"type":"text","text":"우하"}]}]}]}]}
	]}`
	var node richdoc.Node
	if err := json.Unmarshal([]byte(raw), &node); err != nil {
		panic(err)
	}
	return &node
}

func build(t *testing.T, doc *richdoc.Node, opts Options) []byte {
	t.Helper()
	data, err := Build(doc, opts)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return data
}

func partOf(t *testing.T, data []byte, name string) string {
	t.Helper()
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip: %v", err)
	}
	for _, file := range archive.File {
		if file.Name != name {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		defer reader.Close()
		var buffer bytes.Buffer
		if _, err := buffer.ReadFrom(reader); err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return buffer.String()
	}
	t.Fatalf("part %s missing", name)
	return ""
}

func TestExportProducesLayoutMarkup(t *testing.T) {
	data := build(t, sample(), Options{Title: "테스트 문서", Author: "muni"})
	document := partOf(t, data, "word/document.xml")

	for _, needle := range []string{
		`<w:pStyle w:val="Title"/>`,
		`<w:pStyle w:val="Heading1"/>`,
		`<w:jc w:val="center"/>`,
		`<w:b/>`, `<w:i/>`, `<w:u w:val="single"/>`,
		`<w:hyperlink`,
		`<w:numPr>`,
		`<w:pStyle w:val="Quote"/>`,
		`<w:pStyle w:val="CodeBlock"/>`,
		`<w:tbl>`, `<w:gridSpan w:val="2"/>`, `<w:vMerge w:val="restart"/>`, `<w:vMerge/>`,
		`<w:tblHeader/>`,
		`<w:pBdr>`,
	} {
		if !strings.Contains(document, needle) {
			t.Errorf("document.xml missing %s", needle)
		}
	}
	for _, part := range []string{"word/styles.xml", "word/numbering.xml", "word/settings.xml", "word/fontTable.xml", "word/theme/theme1.xml", "docProps/core.xml", "docProps/app.xml", "word/_rels/document.xml.rels", "[Content_Types].xml"} {
		if partOf(t, data, part) == "" {
			t.Errorf("missing part %s", part)
		}
	}
}

func TestOrderedListsGetSeparateNumbering(t *testing.T) {
	doc := richdoc.Doc()
	for i := 0; i < 2; i++ {
		list := &richdoc.Node{Type: "orderedList"}
		list.SetAttr("start", 1)
		list.Append(&richdoc.Node{Type: "listItem", Content: []*richdoc.Node{richdoc.Paragraph(richdoc.Text("항목"))}})
		doc.Append(list)
	}
	data := build(t, doc, Options{Title: "목록"})
	numbering := partOf(t, data, "word/numbering.xml")
	if strings.Count(numbering, "<w:num ") != 2 {
		t.Fatalf("expected two numbering instances so the second list restarts:\n%s", numbering)
	}
}

func TestRoundTripPreservesStructure(t *testing.T) {
	data := build(t, sample(), Options{Title: "테스트 문서"})
	imported, assets, _, err := Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(assets) != 0 {
		t.Fatalf("unexpected assets: %d", len(assets))
	}
	types := map[string]int{}
	var walk func(*richdoc.Node)
	walk = func(node *richdoc.Node) {
		types[node.Type]++
		for _, child := range node.Content {
			walk(child)
		}
	}
	walk(imported)
	for _, expected := range []string{"heading", "bulletList", "orderedList", "taskList", "blockquote", "codeBlock", "table", "tableHeader", "tableCell"} {
		if types[expected] == 0 {
			encoded, _ := json.Marshal(imported)
			t.Fatalf("round trip lost %s\n%s", expected, encoded)
		}
	}
	text := imported.PlainText()
	for _, expected := range []string{"보고서 제목", "굵게", "중첩 항목", "순번 둘", "완료된 일", "인용문", "fmt.Println(1)", "세로병합"} {
		if !strings.Contains(text, expected) {
			t.Errorf("round trip lost text %q", expected)
		}
	}
	if types["taskItem"] != 2 {
		t.Errorf("expected 2 task items, got %d", types["taskItem"])
	}
}

func TestRoundTripPreservesMarks(t *testing.T) {
	data := build(t, sample(), Options{Title: "T"})
	imported, _, _, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	var walk func(*richdoc.Node)
	walk = func(node *richdoc.Node) {
		for _, mark := range node.Marks {
			found[mark.Type] = true
		}
		for _, child := range node.Content {
			walk(child)
		}
	}
	walk(imported)
	for _, expected := range []string{"bold", "italic", "underline", "link"} {
		if !found[expected] {
			t.Errorf("round trip lost mark %s", expected)
		}
	}
}

func TestRoundTripPreservesImages(t *testing.T) {
	pixel := []byte{
		0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0a, 'I', 'D', 'A', 'T', 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 'I', 'E', 'N', 'D', 0xae,
		0x42, 0x60, 0x82,
	}
	image := &richdoc.Node{Type: "image"}
	image.SetAttr("src", "/api/v1/attachments/abc")
	image.SetAttr("alt", "픽셀")
	doc := richdoc.Doc(image)
	data := build(t, doc, Options{Title: "그림", ResolveImage: func(src string) (Image, bool) {
		if src != "/api/v1/attachments/abc" {
			return Image{}, false
		}
		return Image{Data: pixel, MediaType: "image/png"}, true
	}})
	if !strings.Contains(partOf(t, data, "word/document.xml"), "<w:drawing>") {
		t.Fatal("image not embedded")
	}
	imported, assets, _, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || !bytes.Equal(assets[0].Data, pixel) {
		t.Fatalf("image asset not recovered: %+v", assets)
	}
	encoded, _ := json.Marshal(imported)
	if !strings.Contains(string(encoded), assets[0].Placeholder) {
		t.Fatalf("image node missing placeholder: %s", encoded)
	}
}

func TestAllPartsAreWellFormedXML(t *testing.T) {
	data := build(t, sample(), Options{Title: "테스트 문서"})
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range archive.File {
		if !strings.HasSuffix(file.Name, ".xml") && !strings.HasSuffix(file.Name, ".rels") {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		decoder := xml.NewDecoder(reader)
		for {
			_, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("%s is not well formed: %v", file.Name, err)
			}
		}
		reader.Close()
	}
}

// TestPackageIsInternallyConsistent checks the references Word validates when
// opening a file: relationship ids, content types, numbering ids and styles.
func TestPackageIsInternallyConsistent(t *testing.T) {
	pixel := []byte{
		0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0a, 'I', 'D', 'A', 'T', 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 'I', 'E', 'N', 'D', 0xae,
		0x42, 0x60, 0x82,
	}
	document := sample()
	image := &richdoc.Node{Type: "image"}
	image.SetAttr("src", "/api/v1/attachments/abc")
	document.Append(image)

	data := build(t, document, Options{Title: "일관성", ResolveImage: func(string) (Image, bool) {
		return Image{Data: pixel, MediaType: "image/png"}, true
	}})

	main := partOf(t, data, "word/document.xml")
	rels := partOf(t, data, "word/_rels/document.xml.rels")
	numbering := partOf(t, data, "word/numbering.xml")
	styles := partOf(t, data, "word/styles.xml")
	types := partOf(t, data, "[Content_Types].xml")

	for _, match := range regexp.MustCompile(`r:(?:id|embed)="([^"]+)"`).FindAllStringSubmatch(main, -1) {
		if !strings.Contains(rels, `Id="`+match[1]+`"`) {
			t.Errorf("relationship %s used but not declared", match[1])
		}
	}
	for _, match := range regexp.MustCompile(`<w:numId w:val="(\d+)"/>`).FindAllStringSubmatch(main, -1) {
		if !strings.Contains(numbering, `<w:num w:numId="`+match[1]+`">`) {
			t.Errorf("numId %s used but not defined", match[1])
		}
	}
	for _, match := range regexp.MustCompile(`<w:(?:pStyle|rStyle|tblStyle) w:val="([^"]+)"/>`).FindAllStringSubmatch(main, -1) {
		if !strings.Contains(styles, `w:styleId="`+match[1]+`"`) {
			t.Errorf("style %s used but not defined", match[1])
		}
	}

	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range archive.File {
		if !strings.HasPrefix(file.Name, "word/media/") {
			continue
		}
		extension := file.Name[strings.LastIndex(file.Name, ".")+1:]
		if !strings.Contains(types, `Extension="`+extension+`"`) {
			t.Errorf("media extension %s has no content type", extension)
		}
		if !strings.Contains(rels, "media/"+file.Name[len("word/media/"):]) {
			t.Errorf("media part %s is not referenced by a relationship", file.Name)
		}
	}

	// Every relationship target must resolve to a part or be external.
	for _, match := range regexp.MustCompile(`Target="([^"]+)"(?:\s+TargetMode="External")?`).FindAllStringSubmatch(rels, -1) {
		target := match[1]
		if strings.HasPrefix(target, "http") || strings.HasPrefix(target, "mailto:") {
			continue
		}
		found := false
		for _, file := range archive.File {
			if file.Name == "word/"+target {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("relationship target %s missing from the package", target)
		}
	}
}

// TestImportedImageIsItsOwnBlock pins the shape the editor can open. Word
// writes a picture inside the run that holds it; the editor's image is a block
// of its own, and a paragraph with one inside is a document it refuses.
func TestImportedImageIsItsOwnBlock(t *testing.T) {
	pixel := []byte{
		0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0a, 'I', 'D', 'A', 'T', 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 'I', 'E', 'N', 'D', 0xae,
		0x42, 0x60, 0x82,
	}
	image := &richdoc.Node{Type: "image"}
	image.SetAttr("src", "/api/v1/attachments/abc")
	data := build(t, richdoc.Doc(richdoc.Paragraph(richdoc.Text("사진 앞"), image)), Options{
		Title: "그림",
		ResolveImage: func(string) (Image, bool) {
			return Image{Data: pixel, MediaType: "image/png"}, true
		},
	})

	imported, _, _, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, block := range imported.Content {
		for _, child := range block.Content {
			if child != nil && child.Type == "image" {
				encoded, _ := json.Marshal(imported)
				t.Fatalf("이미지가 %s 안에 갇혀 있습니다: %s", block.Type, encoded)
			}
		}
	}
	found := false
	for _, block := range imported.Content {
		if block.Type == "image" {
			found = true
		}
	}
	if !found {
		encoded, _ := json.Marshal(imported)
		t.Fatalf("이미지 블록이 없습니다: %s", encoded)
	}
}
