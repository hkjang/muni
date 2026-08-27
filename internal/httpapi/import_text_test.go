package httpapi

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/docx"
	"github.com/hkjang/muni/internal/richdoc"
)

func parseImported(t *testing.T, content json.RawMessage) *richdoc.Node {
	t.Helper()
	document, err := richdoc.Parse(content)
	if err != nil {
		t.Fatalf("parse imported document: %v", err)
	}
	return document
}

func countTypes(node *richdoc.Node) map[string]int {
	counts := map[string]int{}
	var walk func(*richdoc.Node)
	walk = func(current *richdoc.Node) {
		counts[current.Type]++
		for _, mark := range current.Marks {
			counts["mark:"+mark.Type]++
		}
		for _, child := range current.Content {
			walk(child)
		}
	}
	walk(node)
	return counts
}

const richMarkdown = "# 분기 보고서\n" +
	"\n" +
	"본문에는 **굵게**, *기울임*, `코드`, ~~취소선~~, ==형광==,\n" +
	"그리고 [링크](https://example.com)가 들어갑니다.\n" +
	"\n" +
	"## 목록\n" +
	"\n" +
	"- 상위 항목\n" +
	"  - 하위 항목\n" +
	"    1. 더 깊은 번호\n" +
	"- 둘째 항목\n" +
	"\n" +
	"1. 하나\n" +
	"2. 둘\n" +
	"\n" +
	"- [x] 완료된 일\n" +
	"- [ ] 남은 일\n" +
	"\n" +
	"> 인용된 문장입니다.\n" +
	"> 두 번째 줄입니다.\n" +
	"\n" +
	"```go\n" +
	"func main() {\n" +
	"\tprintln(\"hi\")\n" +
	"}\n" +
	"```\n" +
	"\n" +
	"---\n" +
	"\n" +
	"| 항목 | 값 |\n" +
	"| --- | ---: |\n" +
	"| 속도 | 12.5 |\n" +
	"| 정확도 | 98% |\n"

func TestMarkdownImportKeepsStructure(t *testing.T) {
	content, assets, err := markdownDocument(richMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 0 {
		t.Fatalf("unexpected assets: %d", len(assets))
	}
	if !validDocumentJSON(content) {
		t.Fatalf("invalid document: %s", content)
	}
	document := parseImported(t, content)
	counts := countTypes(document)
	for _, expected := range []string{
		"heading", "bulletList", "orderedList", "taskList", "blockquote",
		"codeBlock", "horizontalRule", "table", "tableHeader", "tableCell",
		"mark:bold", "mark:italic", "mark:code", "mark:strike", "mark:highlight", "mark:link",
	} {
		if counts[expected] == 0 {
			encoded, _ := json.Marshal(document)
			t.Errorf("markdown import lost %s\n%s", expected, encoded)
		}
	}
	if counts["taskItem"] != 2 {
		t.Errorf("expected 2 task items, got %d", counts["taskItem"])
	}
	text := document.PlainText()
	for _, expected := range []string{"분기 보고서", "더 깊은 번호", "인용된 문장입니다. 두 번째 줄입니다.", `println("hi")`, "98%"} {
		if !strings.Contains(text, expected) {
			t.Errorf("markdown import lost %q:\n%s", expected, text)
		}
	}
	// The wrapped paragraph must be rejoined without losing the comma.
	if !strings.Contains(text, "형광, 그리고") {
		t.Errorf("soft line break not joined:\n%s", text)
	}
}

func TestMarkdownNestingDepth(t *testing.T) {
	content, _, err := markdownDocument("- 상위\n  - 하위\n    1. 번호\n")
	if err != nil {
		t.Fatal(err)
	}
	document := parseImported(t, content)
	// bulletList > listItem > (paragraph, bulletList > listItem > (paragraph, orderedList))
	outer := document.Content[0]
	if outer.Type != "bulletList" || len(outer.Content) != 1 {
		t.Fatalf("unexpected outer list: %+v", outer)
	}
	inner := outer.Content[0].Content
	if len(inner) != 2 || inner[1].Type != "bulletList" {
		t.Fatalf("nested list missing: %+v", inner)
	}
	deepest := inner[1].Content[0].Content
	if len(deepest) != 2 || deepest[1].Type != "orderedList" {
		t.Fatalf("third level missing: %+v", deepest)
	}
}

func TestMarkdownExportImportRoundTrip(t *testing.T) {
	original := json.RawMessage(`{"type":"doc","content":[
		{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"소제목"}]},
		{"type":"paragraph","content":[
			{"type":"text","marks":[{"type":"bold"}],"text":"굵게"},
			{"type":"text","text":" 그리고 "},
			{"type":"text","marks":[{"type":"italic"}],"text":"기울임"}]},
		{"type":"bulletList","content":[
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"상위"}]},
				{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"하위"}]}]}]}]}]},
		{"type":"taskList","content":[
			{"type":"taskItem","attrs":{"checked":true},"content":[{"type":"paragraph","content":[{"type":"text","text":"완료"}]}]}]},
		{"type":"codeBlock","attrs":{"language":"go"},"content":[{"type":"text","text":"a := 1"}]},
		{"type":"table","content":[
			{"type":"tableRow","content":[
				{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"A"}]}]},
				{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"B"}]}]}]},
			{"type":"tableRow","content":[
				{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"1"}]}]},
				{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"2"}]}]}]}]}
	]}`)
	exported := renderMarkdown("", original)
	content, _, err := markdownDocument(exported)
	if err != nil {
		t.Fatalf("re-import: %v\n%s", err, exported)
	}
	counts := countTypes(parseImported(t, content))
	for _, expected := range []string{"heading", "bulletList", "taskList", "codeBlock", "table", "mark:bold", "mark:italic"} {
		if counts[expected] == 0 {
			t.Errorf("round trip lost %s\nexported markdown:\n%s\nimported: %s", expected, exported, content)
		}
	}
	if counts["bulletList"] != 2 {
		t.Errorf("nested list lost: %v\n%s", counts, exported)
	}
}

const richHTML = `<!doctype html><html><head><title>무시</title><style>p{color:red}</style></head><body>
<h2 style="text-align:center">가운데 제목</h2>
<p>본문 <strong>굵게</strong> <em>기울임</em> <u>밑줄</u> <s>취소</s> <code>코드</code>
<mark style="background-color:#ffe08a">형광</mark> <a href="https://example.com">링크</a>
<span style="color:#336699;font-size:14pt">스타일</span><br>줄바꿈 뒤</p>
<ul><li>상위<ul><li>하위</li></ul></li><li>둘째</li></ul>
<ol start="3"><li>셋</li><li>넷</li></ol>
<ul data-type="taskList"><li data-checked="true"><div><p>완료</p></div></li><li data-checked="false"><div><p>남음</p></div></li></ul>
<blockquote><p>인용</p></blockquote>
<pre><code class="language-go">func main() {}</code></pre>
<hr>
<table><thead><tr><th colspan="2">머리글</th></tr></thead>
<tbody><tr><td rowspan="2">병합</td><td>우상</td></tr><tr><td>우하</td></tr></tbody></table>
<script>alert('x')</script>
</body></html>`

func TestHTMLImportKeepsStructure(t *testing.T) {
	content, assets, err := htmlDocument([]byte(richHTML))
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 0 {
		t.Fatalf("unexpected assets: %d", len(assets))
	}
	document := parseImported(t, content)
	counts := countTypes(document)
	for _, expected := range []string{
		"heading", "bulletList", "orderedList", "taskList", "blockquote", "codeBlock",
		"horizontalRule", "table", "tableHeader", "tableCell", "hardBreak",
		"mark:bold", "mark:italic", "mark:underline", "mark:strike", "mark:code",
		"mark:highlight", "mark:link", "mark:textStyle",
	} {
		if counts[expected] == 0 {
			encoded, _ := json.Marshal(document)
			t.Errorf("HTML import lost %s\n%s", expected, encoded)
		}
	}
	text := document.PlainText()
	if strings.Contains(text, "alert") || strings.Contains(text, "color:red") || strings.Contains(text, "무시") {
		t.Errorf("script, style or head content leaked:\n%s", text)
	}
	for _, expected := range []string{"가운데 제목", "하위", "완료", "인용", "func main() {}", "병합", "우하"} {
		if !strings.Contains(text, expected) {
			t.Errorf("HTML import lost %q:\n%s", expected, text)
		}
	}
}

func TestHTMLImportKeepsTableSpansAndAlignment(t *testing.T) {
	content, _, err := htmlDocument([]byte(richHTML))
	if err != nil {
		t.Fatal(err)
	}
	document := parseImported(t, content)
	var heading, header, merged *richdoc.Node
	var walk func(*richdoc.Node)
	walk = func(node *richdoc.Node) {
		switch {
		case node.Type == "heading" && heading == nil:
			heading = node
		case node.Type == "tableHeader" && header == nil:
			header = node
		case node.Type == "tableCell" && node.AttrInt("rowspan", 1) > 1 && merged == nil:
			merged = node
		}
		for _, child := range node.Content {
			walk(child)
		}
	}
	walk(document)
	if heading == nil || heading.AttrString("textAlign") != "center" {
		t.Errorf("heading alignment lost: %+v", heading)
	}
	if header == nil || header.AttrInt("colspan", 1) != 2 {
		t.Errorf("colspan lost: %+v", header)
	}
	if merged == nil || merged.AttrInt("rowspan", 1) != 2 {
		t.Errorf("rowspan lost: %+v", merged)
	}
}

func TestHTMLExportImportRoundTrip(t *testing.T) {
	original := json.RawMessage(`{"type":"doc","content":[
		{"type":"heading","attrs":{"level":3,"textAlign":"right"},"content":[{"type":"text","text":"제목"}]},
		{"type":"paragraph","content":[
			{"type":"text","marks":[{"type":"bold"},{"type":"underline"}],"text":"강조"},
			{"type":"text","text":" 일반 "},
			{"type":"text","marks":[{"type":"link","attrs":{"href":"https://example.com"}}],"text":"링크"}]},
		{"type":"taskList","content":[
			{"type":"taskItem","attrs":{"checked":true},"content":[{"type":"paragraph","content":[{"type":"text","text":"완료"}]}]}]},
		{"type":"table","content":[
			{"type":"tableRow","content":[
				{"type":"tableHeader","attrs":{"colspan":2},"content":[{"type":"paragraph","content":[{"type":"text","text":"머리"}]}]}]},
			{"type":"tableRow","content":[
				{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"좌"}]}]},
				{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"우"}]}]}]}]}
	]}`)
	exported := fullHTML("문서 제목", false, renderHTML(original))
	content, _, err := htmlDocument([]byte(exported))
	if err != nil {
		t.Fatal(err)
	}
	document := parseImported(t, content)
	counts := countTypes(document)
	for _, expected := range []string{"heading", "taskList", "table", "mark:bold", "mark:underline", "mark:link"} {
		if counts[expected] == 0 {
			t.Errorf("round trip lost %s\n%s", expected, exported)
		}
	}
	if counts["taskItem"] != 1 {
		t.Errorf("task item lost: %v", counts)
	}
	text := document.PlainText()
	for _, expected := range []string{"문서 제목", "제목", "강조", "머리", "좌", "우"} {
		if !strings.Contains(text, expected) {
			t.Errorf("round trip lost %q:\n%s", expected, text)
		}
	}
	// Whitespace between neighbouring inline elements must survive.
	if !strings.Contains(text, "강조 일반 링크") {
		t.Errorf("spacing between inline elements lost:\n%s", text)
	}
	// The rendered check-box is decoration; only the attribute carries state.
	if strings.ContainsAny(text, "\u2610\u2612") {
		t.Errorf("check-box glyph leaked into the task item text:\n%s", text)
	}
	var task *richdoc.Node
	var find func(*richdoc.Node)
	find = func(node *richdoc.Node) {
		if node.Type == "taskItem" && task == nil {
			task = node
		}
		for _, child := range node.Content {
			find(child)
		}
	}
	find(document)
	if task == nil || !task.AttrBool("checked") || len(task.Content) != 1 {
		t.Errorf("task item not restored cleanly: %+v", task)
	}
}

func TestHTMLImportStoresInlineImages(t *testing.T) {
	pixel := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	content, assets, err := htmlDocument([]byte(`<body><p><img src="` + pixel + `" alt="점"></p><p><img src="https://example.com/x.png" alt="외부 그림"></p></body>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || assets[0].MediaType != "image/png" || len(assets[0].Data) == 0 {
		t.Fatalf("inline image not captured: %+v", assets)
	}
	encoded := string(content)
	if !strings.Contains(encoded, assets[0].Placeholder) {
		t.Errorf("image node missing placeholder: %s", encoded)
	}
	// A remote image cannot be fetched, so its description survives as text.
	if !strings.Contains(encoded, "외부 그림") {
		t.Errorf("remote image description dropped: %s", encoded)
	}
	if strings.Contains(encoded, "https://example.com/x.png") {
		t.Errorf("remote image source should not be kept: %s", encoded)
	}
}

func TestMarkdownImportStoresInlineImages(t *testing.T) {
	pixel := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	content, assets, err := markdownDocument("![점](" + pixel + ")\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 {
		t.Fatalf("inline image not captured: %+v", assets)
	}
	if !strings.Contains(string(content), assets[0].Placeholder) {
		t.Errorf("image node missing placeholder: %s", content)
	}
}

func TestPlainTextImportKeepsParagraphs(t *testing.T) {
	content, err := plainTextDocument("첫 문단 첫 줄\n첫 문단 둘째 줄\n\n둘째 문단\n")
	if err != nil {
		t.Fatal(err)
	}
	document := parseImported(t, content)
	if len(document.Content) != 2 {
		t.Fatalf("expected two paragraphs, got %d", len(document.Content))
	}
	if countTypes(document)["hardBreak"] != 1 {
		t.Errorf("line break inside the paragraph lost: %s", content)
	}
}

func TestMarkdownDoesNotEmphasiseSnakeCase(t *testing.T) {
	content, _, err := markdownDocument("변수 이름은 user_name_field 입니다.\n")
	if err != nil {
		t.Fatal(err)
	}
	document := parseImported(t, content)
	if countTypes(document)["mark:italic"] != 0 {
		t.Errorf("snake_case became emphasis: %s", content)
	}
	if !strings.Contains(document.PlainText(), "user_name_field") {
		t.Errorf("underscores lost: %s", document.PlainText())
	}
}

// TestCrossFormatRoundTrip walks a document through every export and import
// pair to make sure the formats agree on the same document model.
func TestCrossFormatRoundTrip(t *testing.T) {
	original := json.RawMessage(`{"type":"doc","content":[
		{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"보고서"}]},
		{"type":"paragraph","content":[
			{"type":"text","marks":[{"type":"bold"}],"text":"핵심"},
			{"type":"text","text":" 내용입니다."}]},
		{"type":"bulletList","content":[
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"항목 하나"}]},
				{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"하위 항목"}]}]}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"항목 둘"}]}]}]},
		{"type":"table","content":[
			{"type":"tableRow","content":[
				{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"이름"}]}]},
				{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"값"}]}]}]},
			{"type":"tableRow","content":[
				{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"속도"}]}]},
				{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"12.5"}]}]}]}]}
	]}`)

	document, err := richdoc.Parse(original)
	if err != nil {
		t.Fatal(err)
	}
	word, err := docx.Build(document, docx.Options{Title: "보고서"})
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		reload func() (json.RawMessage, error)
	}{
		{"docx", func() (json.RawMessage, error) {
			content, _, _, err := docxImport(word)
			return content, err
		}},
		{"markdown", func() (json.RawMessage, error) {
			content, _, err := markdownDocument(renderMarkdown("", original))
			return content, err
		}},
		{"html", func() (json.RawMessage, error) {
			content, _, err := htmlDocument([]byte(fullHTML("", false, renderHTML(original))))
			return content, err
		}},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			content, err := item.reload()
			if err != nil {
				t.Fatalf("reload: %v", err)
			}
			if !validDocumentJSON(content) {
				t.Fatalf("invalid document: %s", content)
			}
			counts := countTypes(parseImported(t, content))
			for _, expected := range []string{"heading", "bulletList", "table", "tableCell", "mark:bold"} {
				if counts[expected] == 0 {
					t.Errorf("%s round trip lost %s: %s", item.name, expected, content)
				}
			}
			if counts["bulletList"] != 2 {
				t.Errorf("%s round trip lost the nested list: %v", item.name, counts)
			}
			text := parseImported(t, content).PlainText()
			for _, expected := range []string{"보고서", "핵심 내용입니다.", "하위 항목", "속도", "12.5"} {
				if !strings.Contains(text, expected) {
					t.Errorf("%s round trip lost %q:\n%s", item.name, expected, text)
				}
			}
		})
	}
}

// A Markdown or HTML picture is written inside the line of prose that holds
// it. The editor draws an image as a block, so the importers have to lift it
// out — otherwise the document arrives in a shape the schema refuses.
func TestImportedInlineImagesBecomeBlocks(t *testing.T) {
	// A 1x1 transparent GIF, small enough to write out in full.
	const pixel = "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7"
	sources := map[string]func() (json.RawMessage, []richdoc.Asset, error){
		"markdown": func() (json.RawMessage, []richdoc.Asset, error) {
			return markdownDocument("사진 앞 ![그림](" + pixel + ") 사진 뒤")
		},
		"html": func() (json.RawMessage, []richdoc.Asset, error) {
			return htmlDocument([]byte(`<p>사진 앞 <img src="` + pixel + `" alt="그림"> 사진 뒤</p>`))
		},
	}
	for name, importer := range sources {
		t.Run(name, func(t *testing.T) {
			content, assets, err := importer()
			if err != nil {
				t.Fatal(err)
			}
			if len(assets) != 1 {
				t.Fatalf("이미지 자산 1개를 기대했지만 %d개입니다", len(assets))
			}
			document := parseImported(t, content)
			images := 0
			for _, block := range document.Content {
				if block.Type == "image" {
					images++
				}
				for _, child := range block.Content {
					if child != nil && child.Type == "image" {
						t.Fatalf("이미지가 %s 안에 갇혀 있습니다: %s", block.Type, content)
					}
				}
			}
			if images != 1 {
				t.Fatalf("이미지 블록 1개를 기대했지만 %d개입니다: %s", images, content)
			}
			if text := document.PlainText(); !strings.Contains(text, "사진 앞") || !strings.Contains(text, "사진 뒤") {
				t.Fatalf("주변 문장이 사라졌습니다: %q", text)
			}
		})
	}
}

// Documents imported before the importers learned to lift a picture out of the
// paragraph holding it are still in the database, and the editor cannot open
// them. They are repaired as they are read.
func TestStoredImagesAreLiftedOnRead(t *testing.T) {
	broken := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"사진 앞"},{"type":"image","attrs":{"src":"/api/v1/attachments/abc"}}]}]}`)
	repaired := parseImported(t, liftStoredImages(broken))
	if len(repaired.Content) != 2 || repaired.Content[1].Type != "image" {
		t.Fatalf("저장된 이미지가 올라오지 않았습니다: %s", liftStoredImages(broken))
	}

	// Prose pays nothing: the bytes come back as they were.
	prose := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"사진 없음"}]}]}`)
	if got := liftStoredImages(prose); string(got) != string(prose) {
		t.Fatalf("이미지 없는 문서가 바뀌었습니다: %s", got)
	}

	// Invalid JSON is handed back untouched rather than emptied.
	junk := json.RawMessage(`{"type":"doc","content":[{"type":"image"`)
	if got := liftStoredImages(junk); string(got) != string(junk) {
		t.Fatalf("깨진 문서를 덮어썼습니다: %s", got)
	}
}

// A .hwpx and a .docx are both zips. An upload that arrives without a usable
// name or media type — a browser guessing application/octet-stream, a proxy
// stripping the name — used to be called a .docx whatever it held, and a
// Hangul file then failed with an error about word/document.xml being missing,
// which is true and useless.
func TestAHangulFileIsNotMistakenForAWordFile(t *testing.T) {
	var hangul bytes.Buffer
	archive := zip.NewWriter(&hangul)
	for _, part := range []struct{ name, content string }{
		{"mimetype", "application/hwp+zip"},
		{"Contents/section0.xml", `<hs:sec xmlns:hs="http://www.hancom.co.kr/hwpml/2011/section"/>`},
	} {
		writer, err := archive.Create(part.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(part.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if got := extensionFromMediaType("application/octet-stream", hangul.Bytes()); got != ".hwpx" {
		t.Errorf("한글 파일을 %q 로 봤습니다", got)
	}

	var word bytes.Buffer
	wordArchive := zip.NewWriter(&word)
	writer, err := wordArchive.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte(`<w:document/>`)); err != nil {
		t.Fatal(err)
	}
	if err := wordArchive.Close(); err != nil {
		t.Fatal(err)
	}
	if got := extensionFromMediaType("application/octet-stream", word.Bytes()); got != ".docx" {
		t.Errorf("워드 파일을 %q 로 봤습니다", got)
	}
}
