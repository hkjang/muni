package pdfx

import (
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

// buildPDF assembles a small but structurally valid PDF around one content
// stream so the tests do not depend on an external producer.
func buildPDF(content string, compress bool) []byte {
	stream := []byte(content)
	filter := ""
	if compress {
		var buffer bytes.Buffer
		writer := zlib.NewWriter(&buffer)
		_, _ = writer.Write(stream)
		_ = writer.Close()
		stream = buffer.Bytes()
		filter = "/Filter/FlateDecode"
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	out.WriteString("1 0 obj <</Type/Catalog/Pages 2 0 R>> endobj\n")
	out.WriteString("2 0 obj <</Type/Pages/Kids[3 0 R]/Count 1>> endobj\n")
	out.WriteString("3 0 obj <</Type/Page/Parent 2 0 R/MediaBox[0 0 595 842]" +
		"/Resources<</Font<</F1 4 0 R/F2 5 0 R>>>>/Contents 6 0 R>> endobj\n")
	out.WriteString("4 0 obj <</Type/Font/Subtype/Type1/BaseFont/Helvetica-Bold/Encoding/WinAnsiEncoding>> endobj\n")
	out.WriteString("5 0 obj <</Type/Font/Subtype/Type1/BaseFont/Helvetica/Encoding/WinAnsiEncoding>> endobj\n")
	out.WriteString(fmt.Sprintf("6 0 obj <</Length %d%s>>\nstream\n", len(stream), filter))
	out.Write(stream)
	out.WriteString("\nendstream endobj\n")
	out.WriteString("7 0 obj <</Title (\\376\\377\\000T\\000i\\000t\\000l\\000e)>> endobj\n")
	out.WriteString("trailer <</Root 1 0 R/Info 7 0 R/Size 8>>\n%%EOF\n")
	return out.Bytes()
}

const sampleContent = `
BT /F1 24 Tf 72 760 Td (Quarterly Report) Tj ET
BT /F2 11 Tf 72 720 Td (This paragraph explains the numbers in detail and it continues) Tj ET
BT /F2 11 Tf 72 706 Td (onto a second line that wraps naturally.) Tj ET
BT /F1 16 Tf 72 670 Td (Highlights) Tj ET
BT /F2 11 Tf 90 640 Td (\225 First bullet) Tj ET
BT /F2 11 Tf 90 624 Td (\225 Second bullet) Tj ET
BT /F2 11 Tf 72 596 Td (1. Numbered one) Tj ET
BT /F2 11 Tf 72 580 Td (2. Numbered two) Tj ET
`

func nodeTypes(node *richdoc.Node) map[string]int {
	counts := map[string]int{}
	var walk func(*richdoc.Node)
	walk = func(current *richdoc.Node) {
		counts[current.Type]++
		for _, child := range current.Content {
			walk(child)
		}
	}
	walk(node)
	return counts
}

func TestImportExtractsStructure(t *testing.T) {
	result, err := Import(context.Background(), buildPDF(sampleContent, false))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if result.Pages != 1 {
		t.Fatalf("expected 1 page, got %d", result.Pages)
	}
	if result.Title != "Title" {
		t.Errorf("title = %q", result.Title)
	}
	counts := nodeTypes(result.Document)
	if counts["heading"] < 2 {
		t.Errorf("expected two headings, got %d", counts["heading"])
	}
	if counts["bulletList"] != 1 || counts["orderedList"] != 1 {
		t.Errorf("lists not recovered: %v", counts)
	}
	text := result.Document.PlainText()
	for _, expected := range []string{"Quarterly Report", "Highlights", "First bullet", "Numbered two"} {
		if !strings.Contains(text, expected) {
			t.Errorf("missing %q in:\n%s", expected, text)
		}
	}
	if !strings.Contains(text, "it continues onto a second line") {
		t.Errorf("wrapped line was not joined:\n%s", text)
	}
	if strings.Contains(text, "• First") {
		t.Errorf("bullet glyph should be stripped from list text:\n%s", text)
	}
}

func TestImportHandlesFlateStreams(t *testing.T) {
	result, err := Import(context.Background(), buildPDF(sampleContent, true))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if !strings.Contains(result.Document.PlainText(), "Quarterly Report") {
		t.Fatal("compressed content stream was not decoded")
	}
}

func TestImportRejectsNonPDF(t *testing.T) {
	if _, err := Import(context.Background(), []byte("this is not a pdf")); err == nil {
		t.Fatal("expected an error for non-PDF input")
	}
}

func TestImportReportsMissingTextLayer(t *testing.T) {
	_, err := Import(context.Background(), buildPDF("q 1 0 0 1 0 0 cm Q\n", false))
	if err == nil {
		t.Fatal("expected an error for a PDF with no text")
	}
}

func TestTableReconstruction(t *testing.T) {
	content := `
BT /F2 11 Tf 72 700 Td (Item) Tj ET
BT /F2 11 Tf 260 700 Td (Value) Tj ET
BT /F2 11 Tf 72 684 Td (Speed) Tj ET
BT /F2 11 Tf 260 684 Td (12.5) Tj ET
BT /F2 11 Tf 72 668 Td (Accuracy) Tj ET
BT /F2 11 Tf 260 668 Td (98 percent) Tj ET
`
	result, err := Import(context.Background(), buildPDF(content, false))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	counts := nodeTypes(result.Document)
	if counts["table"] != 1 || counts["tableRow"] != 3 {
		t.Fatalf("table not reconstructed: %v", counts)
	}
}

func TestProseIsNotMistakenForATable(t *testing.T) {
	content := `
BT /F2 11 Tf 72 700 Td (The quick brown fox jumps over the lazy dog again) Tj ET
BT /F2 11 Tf 72 684 Td (and then it rests for a while before running more.) Tj ET
BT /F2 11 Tf 72 668 Td (Another ordinary sentence follows this one closely.) Tj ET
`
	result, err := Import(context.Background(), buildPDF(content, false))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if counts := nodeTypes(result.Document); counts["table"] != 0 {
		t.Fatalf("prose became a table: %v", counts)
	}
}

func TestFilterRoundTrips(t *testing.T) {
	original := []byte("Hello \x00 world, this is a filter test with repetition repetition repetition.")
	hexed := make([]byte, 0, len(original)*2+1)
	for _, item := range original {
		hexed = append(hexed, "0123456789abcdef"[item>>4], "0123456789abcdef"[item&0xf])
	}
	hexed = append(hexed, '>')
	decoded, err := asciiHexDecode(hexed)
	if err != nil || !bytes.Equal(decoded, original) {
		t.Fatalf("ASCIIHexDecode round trip failed: %v %q", err, decoded)
	}

	runLength := []byte{2, 'a', 'b', 'c', 254, 'z', 128}
	decoded, err = runLengthDecode(runLength)
	if err != nil || string(decoded) != "abczzz" {
		t.Fatalf("RunLengthDecode = %q (%v)", decoded, err)
	}
}

func TestMonospaceLinesBecomeCodeBlocks(t *testing.T) {
	content := `
BT /F1 24 Tf 72 760 Td (Snippet) Tj ET
BT /F2 10 Tf 72 700 Td (func main\(\) {) Tj ET
BT /F2 10 Tf 72 688 Td (  println\("hi"\)) Tj ET
BT /F2 10 Tf 72 676 Td (}) Tj ET
`
	source := strings.Replace(string(buildPDF(content, false)),
		"BaseFont/Helvetica/Encoding", "BaseFont/Courier/Encoding", 1)
	result, err := Import(context.Background(), []byte(source))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	counts := nodeTypes(result.Document)
	if counts["codeBlock"] != 1 {
		t.Fatalf("expected one code block: %v", counts)
	}
	if !strings.Contains(result.Document.PlainText(), `println("hi")`) {
		t.Fatalf("code text lost: %q", result.Document.PlainText())
	}
}

func TestCheckboxGlyphsBecomeTaskItems(t *testing.T) {
	// \x{2612} and \x{2610} are written through the font's Differences table.
	content := `
BT /F2 11 Tf 72 700 Td ([x] Ship the release) Tj ET
BT /F2 11 Tf 72 684 Td ([ ] Write the notes) Tj ET
`
	result, err := Import(context.Background(), buildPDF(content, false))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	counts := nodeTypes(result.Document)
	if counts["taskList"] != 1 || counts["taskItem"] != 2 {
		t.Fatalf("task list not recovered: %v", counts)
	}
	text := result.Document.PlainText()
	if strings.Contains(text, "[x]") || !strings.Contains(text, "Ship the release") {
		t.Fatalf("checkbox prefix not stripped: %q", text)
	}
}

func TestIndentedRunsBecomeNestedLists(t *testing.T) {
	content := `
BT /F2 11 Tf 72 700 Td (Body text that sets the page margin for the document.) Tj ET
BT /F2 11 Tf 100 680 Td (First) Tj ET
BT /F2 11 Tf 118 664 Td (Nested) Tj ET
BT /F2 11 Tf 100 648 Td (Second) Tj ET
BT /F2 11 Tf 72 620 Td (Closing body text at the original left margin here.) Tj ET
`
	result, err := Import(context.Background(), buildPDF(content, false))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	counts := nodeTypes(result.Document)
	if counts["bulletList"] < 2 {
		t.Fatalf("nested list not recovered: %v", counts)
	}
}

// buildImagePDF produces a document where every page draws the same image
// XObject, which is what a letterhead logo or watermark looks like.
func buildImagePDF(pages int) []byte {
	var pixels bytes.Buffer
	writer := zlib.NewWriter(&pixels)
	_, _ = writer.Write([]byte{
		0x10, 0x20, 0x30, 0x40, 0x50, 0x60,
		0x70, 0x80, 0x90, 0xa0, 0xb0, 0xc0,
	})
	_ = writer.Close()

	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	out.WriteString("1 0 obj <</Type/Catalog/Pages 2 0 R>> endobj\n")
	kids := make([]string, 0, pages)
	for page := 0; page < pages; page++ {
		kids = append(kids, fmt.Sprintf("%d 0 R", 10+page))
	}
	out.WriteString(fmt.Sprintf("2 0 obj <</Type/Pages/Kids[%s]/Count %d>> endobj\n", strings.Join(kids, " "), pages))
	out.WriteString("5 0 obj <</Type/Font/Subtype/Type1/BaseFont/Helvetica/Encoding/WinAnsiEncoding>> endobj\n")
	out.WriteString(fmt.Sprintf("9 0 obj <</Type/XObject/Subtype/Image/Width 2/Height 2/ColorSpace/DeviceRGB"+
		"/BitsPerComponent 8/Filter/FlateDecode/Length %d>>\nstream\n", pixels.Len()))
	out.Write(pixels.Bytes())
	out.WriteString("\nendstream endobj\n")

	for page := 0; page < pages; page++ {
		content := fmt.Sprintf("q 200 0 0 100 60 600 cm /Im0 Do Q\nBT /F2 11 Tf 72 500 Td (Page %d body text) Tj ET\n", page+1)
		out.WriteString(fmt.Sprintf("%d 0 obj <</Type/Page/Parent 2 0 R/MediaBox[0 0 595 842]"+
			"/Resources<</XObject<</Im0 9 0 R>>/Font<</F2 5 0 R>>>>/Contents %d 0 R>> endobj\n", 10+page, 100+page))
		out.WriteString(fmt.Sprintf("%d 0 obj <</Length %d>>\nstream\n%s\nendstream endobj\n", 100+page, len(content), content))
	}
	out.WriteString("trailer <</Root 1 0 R/Size 200>>\n%%EOF\n")
	return out.Bytes()
}

func TestRepeatedImageIsStoredOnce(t *testing.T) {
	result, err := Import(context.Background(), buildImagePDF(2))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(result.Assets) != 1 {
		t.Fatalf("the same picture should be stored once, got %d assets", len(result.Assets))
	}
	if counts := nodeTypes(result.Document); counts["image"] != 2 {
		t.Fatalf("both occurrences should still be referenced: %v", counts)
	}
}

func TestImageOnEveryPageIsTreatedAsFurniture(t *testing.T) {
	result, err := Import(context.Background(), buildImagePDF(4))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(result.Assets) != 0 {
		t.Fatalf("a logo repeated on every page should be dropped, got %d assets", len(result.Assets))
	}
	if counts := nodeTypes(result.Document); counts["image"] != 0 {
		t.Fatalf("furniture image nodes survived: %v", counts)
	}
	if !strings.Contains(result.Document.PlainText(), "Page 4 body text") {
		t.Fatalf("page text lost: %q", result.Document.PlainText())
	}
}

func TestImportStopsAtDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Import(ctx, buildImagePDF(4)); err == nil {
		t.Fatal("expected an error once the deadline has passed")
	}
}
