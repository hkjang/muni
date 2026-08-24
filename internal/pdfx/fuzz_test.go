package pdfx

import (
	"context"
	"testing"
)

// FuzzImport exercises the PDF reader with malformed input. Imports run on
// user-supplied uploads, so a panic here would be a denial of service.
func FuzzImport(f *testing.F) {
	f.Add(buildPDF(sampleContent, false))
	f.Add(buildPDF(sampleContent, true))
	f.Add(buildPDF("BT /F2 11 Tf 72 700 Td (\\336\\337) Tj ET", false))
	f.Add([]byte("%PDF-1.4\ntrailer <</Root 1 0 R>>\n%%EOF"))
	f.Add([]byte("%PDF-1.7\n1 0 obj <</Type/Page/Contents 2 0 R>> endobj\n2 0 obj <</Length 99>>\nstream\nBT ET\n"))

	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 1<<20 {
			t.Skip()
		}
		result, err := Import(context.Background(), body)
		if err != nil {
			return
		}
		if result.Document == nil {
			t.Fatal("nil document with no error")
		}
		if _, err := result.Document.JSON(); err != nil {
			t.Fatalf("imported document is not serialisable: %v", err)
		}
	})
}
