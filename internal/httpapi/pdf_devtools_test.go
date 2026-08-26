package httpapi

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/pdfx"
)

// TestDevtoolsPDFHasPageNumbers renders a document long enough to need three
// pages and checks that the footer Chromium drew is really there.
//
// It needs a browser, so it skips where there is not one. Everything about
// this path is the conversation with Chromium; a test with the browser stubbed
// out would only be testing the stub.
func TestDevtoolsPDFHasPageNumbers(t *testing.T) {
	binary, err := chromiumBinary()
	if err != nil {
		t.Skip("no headless browser available")
	}

	// A browser installed as a snap cannot read /tmp, which is where a Go test
	// puts its scratch directory. The container ships an ordinary package and
	// has no such confinement, so this is about where the test runs rather
	// than about muni.
	tempDir, err := os.MkdirTemp(os.Getenv("HOME"), "muni-pdf-test-")
	if err != nil {
		t.Skip("no writable home directory for the browser to read from")
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })
	htmlPath := filepath.Join(tempDir, "document.html")
	var body strings.Builder
	for page := 1; page <= 3; page++ {
		body.WriteString("<h1>" + strings.Repeat("장 ", 3) + "</h1>")
		for line := 0; line < 45; line++ {
			body.WriteString("<p>본문이 이어집니다. 줄 " + strings.Repeat("내용 ", 6) + "</p>")
		}
	}
	if err := os.WriteFile(htmlPath, []byte(fullHTML("검증 문서", body.String())), 0600); err != nil {
		t.Fatal(err)
	}

	pdf, err := printToPDFWithDevtools(context.Background(), binary, tempDir, htmlPath,
		pdfFooter{Title: "검증 문서"})
	if err != nil {
		t.Fatalf("devtools render failed: %v", err)
	}
	if len(pdf) == 0 {
		t.Fatal("the render produced nothing")
	}
	if !strings.HasPrefix(string(pdf), "%PDF-") {
		t.Fatal("the result is not a PDF")
	}

	// muni's own PDF reader is used to check the result, which proves the file
	// paginated and that the two halves agree about what a PDF is.
	parsed, err := pdfx.Import(context.Background(), pdf)
	if err != nil {
		t.Fatalf("muni could not read back the PDF it produced: %v", err)
	}
	if parsed.Pages < 2 {
		t.Fatalf("expected a document long enough to paginate, got %d page(s)", parsed.Pages)
	}
	text := parsed.Document.PlainText()
	// The footer Chromium drew is text on the page, so the numbering is
	// readable back out — which is the whole point of this path.
	if !strings.Contains(text, "1 / "+strconv.Itoa(parsed.Pages)) {
		t.Fatalf("the page numbering is not in the document: %q", truncate(text, 300))
	}
	if !strings.Contains(text, "검증 문서") {
		t.Fatalf("the footer should name the document: %q", truncate(text, 300))
	}
	t.Logf("rendered %d pages, %d bytes", parsed.Pages, len(pdf))
}

func TestFooterTemplateCarriesChromiumsPlaceholders(t *testing.T) {
	// Chromium fills these two classes in as it lays out each page; without
	// them the footer renders but says nothing.
	template := pdfFooter{Title: "보고서"}.template()
	for _, needed := range []string{"pageNumber", "totalPages"} {
		if !strings.Contains(template, needed) {
			t.Fatalf("the footer must carry %s: %s", needed, template)
		}
	}
	if !strings.Contains(template, "보고서") {
		t.Fatalf("the footer should name the document: %s", template)
	}
}

func TestFooterEscapesTheTitle(t *testing.T) {
	// The title reaches a rendered document, so it is markup until escaped.
	template := pdfFooter{Title: `<script>alert(1)</script>`}.template()
	if strings.Contains(template, "<script>") {
		t.Fatalf("the title was not escaped: %s", template)
	}
}
