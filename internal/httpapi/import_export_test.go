package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/docx"
	"github.com/hkjang/muni/internal/richdoc"
)

func TestMarkdownImport(t *testing.T) {
	document, _, err := markdownDocument("# 제목\n\n본문\n- [x] 완료")
	if err != nil {
		t.Fatal(err)
	}
	if !validDocumentJSON(document) {
		t.Fatalf("invalid document: %s", document)
	}
	text := extractDocumentText(document)
	if !strings.Contains(text, "제목") || !strings.Contains(text, "완료") {
		t.Fatalf("missing imported text: %q", text)
	}
}

func TestHTMLImportDropsScript(t *testing.T) {
	document, _, err := htmlDocument([]byte(`<html><body><h1>안전한 제목</h1><p>본문</p><script>alert('x')</script></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	text := extractDocumentText(document)
	if strings.Contains(text, "alert") || !strings.Contains(text, "안전한 제목") {
		t.Fatalf("unexpected imported text: %q", text)
	}
}

func TestDOCXExportImportRoundTrip(t *testing.T) {
	content := json.RawMessage(`{"type":"doc","content":[
		{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"소제목"}]},
		{"type":"paragraph","content":[{"type":"text","marks":[{"type":"bold"}],"text":"한글 문서 본문"}]},
		{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"목록 항목"}]}]}]}
	]}`)
	document, err := richdoc.Parse(content)
	if err != nil {
		t.Fatal(err)
	}
	file, err := docx.Build(document, docx.Options{Title: "테스트 제목"})
	if err != nil {
		t.Fatal(err)
	}
	imported, assets, _, err := docxImport(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 0 {
		t.Fatalf("unexpected assets: %d", len(assets))
	}
	if !validDocumentJSON(imported) {
		t.Fatalf("invalid imported document: %s", imported)
	}
	text := extractDocumentText(imported)
	for _, expected := range []string{"테스트 제목", "소제목", "한글 문서 본문", "목록 항목"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("DOCX round trip lost %q: %q", expected, text)
		}
	}
	if !strings.Contains(string(imported), `"bulletList"`) || !strings.Contains(string(imported), `"heading"`) {
		t.Fatalf("DOCX round trip lost structure: %s", imported)
	}
}

func TestHTMLRendererEscapesContent(t *testing.T) {
	content := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"<script>alert(1)</script>"}]}]}`)
	rendered := renderHTML(content)
	if strings.Contains(rendered, "<script>") || !strings.Contains(rendered, "&lt;script&gt;") {
		t.Fatalf("unsafe HTML: %s", rendered)
	}
}

func TestHTMLRendererKeepsLayout(t *testing.T) {
	content := json.RawMessage(`{"type":"doc","content":[
		{"type":"heading","attrs":{"level":2,"textAlign":"center"},"content":[{"type":"text","text":"제목"}]},
		{"type":"taskList","content":[{"type":"taskItem","attrs":{"checked":true},"content":[{"type":"paragraph","content":[{"type":"text","text":"완료"}]}]}]},
		{"type":"table","content":[{"type":"tableRow","content":[
			{"type":"tableHeader","attrs":{"colspan":2},"content":[{"type":"paragraph","content":[{"type":"text","text":"머리"}]}]}]}]}
	]}`)
	rendered := renderHTML(content)
	for _, expected := range []string{`<h2 style="text-align:center">`, `data-type="taskList"`, `<th colspan="2"`} {
		if !strings.Contains(rendered, expected) {
			t.Errorf("rendered HTML missing %s:\n%s", expected, rendered)
		}
	}
}

func TestMarkdownRendererKeepsStructure(t *testing.T) {
	content := json.RawMessage(`{"type":"doc","content":[
		{"type":"paragraph","content":[{"type":"text","marks":[{"type":"bold"}],"text":"굵게"}]},
		{"type":"bulletList","content":[
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"상위"}]},
				{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"하위"}]}]}]}]}]},
		{"type":"table","content":[
			{"type":"tableRow","content":[
				{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"A"}]}]},
				{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"B"}]}]}]},
			{"type":"tableRow","content":[
				{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"1"}]}]},
				{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"2"}]}]}]}]}
	]}`)
	rendered := renderMarkdown("보고서", content)
	for _, expected := range []string{"# 보고서", "**굵게**", "- 상위", "  - 하위", "| A | B |", "| --- | --- |"} {
		if !strings.Contains(rendered, expected) {
			t.Errorf("markdown missing %q:\n%s", expected, rendered)
		}
	}
}

func TestPlainTextRendererKeepsListMarkers(t *testing.T) {
	content := json.RawMessage(`{"type":"doc","content":[
		{"type":"orderedList","attrs":{"start":1},"content":[
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"첫째"}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"둘째"}]}]}]}
	]}`)
	rendered := renderPlainText("문서", content)
	if !strings.Contains(rendered, "1. 첫째") || !strings.Contains(rendered, "2. 둘째") {
		t.Fatalf("plain text lost numbering:\n%s", rendered)
	}
}

func TestPrepareImportedAssetsRewritesAndDrops(t *testing.T) {
	content := json.RawMessage(`{"type":"doc","content":[
		{"type":"image","attrs":{"src":"muni-import-image:1"}},
		{"type":"image","attrs":{"src":"muni-import-image:2"}},
		{"type":"paragraph","content":[{"type":"text","text":"본문"}]}
	]}`)
	attachments, rewritten, err := prepareImportedAssets([]richdoc.Asset{
		{Placeholder: "muni-import-image:1", Name: "a.png", MediaType: "image/png", Data: []byte{1, 2, 3}},
		{Placeholder: "muni-import-image:2", Name: "b.emf", MediaType: "image/x-emf", Data: []byte{4, 5}},
	}, content)
	if err != nil {
		t.Fatal(err)
	}
	if len(attachments) != 1 || attachments[0].Name != "a.png" {
		t.Fatalf("unexpected attachments: %+v", attachments)
	}
	encoded := string(rewritten)
	if strings.Contains(encoded, "muni-import-image:") {
		t.Errorf("placeholder survived: %s", encoded)
	}
	if !strings.Contains(encoded, "/api/v1/attachments/"+attachments[0].ID.String()) {
		t.Errorf("image not pointed at its attachment: %s", encoded)
	}
	if strings.Count(encoded, `"image"`) != 1 {
		t.Errorf("unstorable image should have been dropped: %s", encoded)
	}
	if !strings.Contains(encoded, "본문") {
		t.Errorf("text lost: %s", encoded)
	}
}

func TestExtensionDetectionFallsBackToContent(t *testing.T) {
	if got := extensionFromMediaType("", []byte("%PDF-1.7\n")); got != ".pdf" {
		t.Errorf("sniffed extension = %q", got)
	}
	if got := extensionFromMediaType("application/pdf; charset=binary", nil); got != ".pdf" {
		t.Errorf("declared extension = %q", got)
	}
}

func TestHTMLTableRepeatsHeaderRowsWhenPrinted(t *testing.T) {
	content := json.RawMessage(`{"type":"doc","content":[{"type":"table","content":[
		{"type":"tableRow","content":[
			{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"A"}]}]},
			{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"B"}]}]}]},
		{"type":"tableRow","content":[
			{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"1"}]}]},
			{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"2"}]}]}]}]}]}`)
	rendered := renderHTML(content)
	if !strings.Contains(rendered, "<thead><tr><th") || !strings.Contains(rendered, "</thead><tbody><tr><td") {
		t.Fatalf("header rows were not grouped into a thead:\n%s", rendered)
	}
	if !strings.Contains(exportStylesheet, "thead{display:table-header-group}") {
		t.Error("print stylesheet does not repeat table headers across pages")
	}
	if strings.Contains(exportStylesheet, "table{border-collapse:collapse;width:100%;margin:10pt 0;page-break-inside:avoid}") {
		t.Error("a long table must be allowed to split across pages")
	}
}

func TestTableWithoutHeaderRowSkipsThead(t *testing.T) {
	content := json.RawMessage(`{"type":"doc","content":[{"type":"table","content":[
		{"type":"tableRow","content":[
			{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"1"}]}]}]}]}]}`)
	rendered := renderHTML(content)
	if strings.Contains(rendered, "<thead>") {
		t.Fatalf("unexpected thead:\n%s", rendered)
	}
	if !strings.Contains(rendered, "<tbody>") {
		t.Fatalf("body rows missing:\n%s", rendered)
	}
}

func TestPDFExportConcurrencyIsBounded(t *testing.T) {
	if cap(pdfSlots) < 1 || cap(pdfSlots) > 32 {
		t.Fatalf("unexpected PDF concurrency limit: %d", cap(pdfSlots))
	}
	t.Setenv("MUNI_PDF_CONCURRENCY", "5")
	if got := pdfConcurrency(); got != 5 {
		t.Errorf("MUNI_PDF_CONCURRENCY not honoured: %d", got)
	}
	t.Setenv("MUNI_PDF_CONCURRENCY", "nonsense")
	if got := pdfConcurrency(); got != 2 {
		t.Errorf("invalid value should fall back to the default, got %d", got)
	}
}

func TestPDFExportGivesUpWhenSlotsAreBusy(t *testing.T) {
	// Fill every slot, then confirm a cancelled request fails fast with a
	// message instead of blocking on the queue.
	for index := 0; index < cap(pdfSlots); index++ {
		pdfSlots <- struct{}{}
	}
	defer func() {
		for index := 0; index < cap(pdfSlots); index++ {
			<-pdfSlots
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := makePDF(ctx, "제목", "", "", false, "<p>본문</p>")
	if err == nil {
		t.Fatal("expected the export to be rejected while all slots are busy")
	}
	if !strings.Contains(err.Error(), "다시 시도") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestChromiumCommandGivesTheBrowserAWritableHome pins the fix for PDF export
// failing in the container: muni runs as a user whose home directory does not
// exist, and Chromium's crash handler aborts the browser when it cannot create
// its database under HOME.
func TestChromiumCommandGivesTheBrowserAWritableHome(t *testing.T) {
	tempDir := t.TempDir()
	command := chromiumCommand(context.Background(), "/usr/bin/chromium", tempDir, tempDir+"/in.html", tempDir+"/out.pdf")

	home := ""
	for _, entry := range command.Env {
		if value, ok := strings.CutPrefix(entry, "HOME="); ok {
			home = value
		}
	}
	if home != tempDir {
		t.Fatalf("HOME = %q, want the per-render temp dir %q", home, tempDir)
	}
	for _, expected := range []string{"XDG_CONFIG_HOME=", "XDG_CACHE_HOME="} {
		found := false
		for _, entry := range command.Env {
			if strings.HasPrefix(entry, expected) && strings.Contains(entry, tempDir) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s is not pointed inside the temp dir", expected)
		}
	}
	joined := strings.Join(command.Args, " ")
	for _, flag := range []string{"--headless=new", "--no-sandbox", "--print-to-pdf=" + tempDir + "/out.pdf", "file://" + tempDir + "/in.html"} {
		if !strings.Contains(joined, flag) {
			t.Errorf("missing flag %s in %s", flag, joined)
		}
	}
}

func TestImportedDocumentsCarryBlockIDs(t *testing.T) {
	source := json.RawMessage(`{"type":"doc","content":[
		{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"제목"}]},
		{"type":"paragraph","content":[{"type":"text","text":"본문"}]}]}`)
	stamped, err := withBlockIDs(source)
	if err != nil {
		t.Fatal(err)
	}
	document, err := richdoc.Parse(stamped)
	if err != nil {
		t.Fatal(err)
	}
	for _, block := range document.Content {
		if value, _ := block.Attr(richdoc.BlockIDAttr).(string); value == "" {
			t.Fatalf("%s has no block id: %s", block.Type, stamped)
		}
	}
	// Running it again must not churn the ids the first pass handed out.
	again, err := withBlockIDs(stamped)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(stamped) {
		t.Fatalf("re-stamping changed the document:\n%s\n%s", stamped, again)
	}
}

func TestExportersIgnoreBlockIDs(t *testing.T) {
	source := json.RawMessage(`{"type":"doc","content":[
		{"type":"paragraph","attrs":{"blockId":"blk_abc"},"content":[{"type":"text","text":"본문"}]}]}`)
	if rendered := renderHTML(source); !strings.Contains(rendered, "본문") {
		t.Fatalf("HTML export broke on a block id: %s", rendered)
	}
	if markdown := renderMarkdown("", source); !strings.Contains(markdown, "본문") {
		t.Fatalf("Markdown export broke on a block id: %s", markdown)
	}
	document, err := richdoc.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	file, err := docx.Build(document, docx.Options{Title: "T"})
	if err != nil {
		t.Fatalf("DOCX export broke on a block id: %v", err)
	}
	imported, _, _, err := docxImport(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(extractDocumentText(imported), "본문") {
		t.Fatalf("DOCX round trip lost the text: %s", imported)
	}
}

func TestBlockIDsSurviveTheHTMLRoundTrip(t *testing.T) {
	source := json.RawMessage(`{"type":"doc","content":[
		{"type":"heading","attrs":{"level":2,"blockId":"blk_head"},"content":[{"type":"text","text":"제목"}]},
		{"type":"paragraph","attrs":{"blockId":"blk_body"},"content":[{"type":"text","text":"본문"}]},
		{"type":"bulletList","content":[
			{"type":"listItem","attrs":{"blockId":"blk_item"},"content":[
				{"type":"paragraph","attrs":{"blockId":"blk_inner"},"content":[{"type":"text","text":"항목"}]}]}]}
	]}`)
	rendered := renderHTML(source)
	for _, expected := range []string{`data-block-id="blk_head"`, `data-block-id="blk_body"`, `data-block-id="blk_item"`} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("export dropped %s:\n%s", expected, rendered)
		}
	}
	content, _, err := htmlDocument([]byte(fullHTML("", false, rendered)))
	if err != nil {
		t.Fatal(err)
	}
	document, err := richdoc.Parse(content)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	var walk func(*richdoc.Node)
	walk = func(node *richdoc.Node) {
		if value := node.AttrString(richdoc.BlockIDAttr); value != "" {
			found[value] = true
		}
		for _, child := range node.Content {
			walk(child)
		}
	}
	walk(document)
	for _, expected := range []string{"blk_head", "blk_body", "blk_item", "blk_inner"} {
		if !found[expected] {
			t.Errorf("round trip lost %s: %s", expected, content)
		}
	}
}
