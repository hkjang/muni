package docx

import "testing"

// FuzzParse feeds malformed packages to the DOCX reader, which runs directly
// on uploaded files.
func FuzzParse(f *testing.F) {
	if data, err := Build(sample(), Options{Title: "seed"}); err == nil {
		f.Add(data)
	}
	f.Add([]byte("PK\x03\x04 not really a zip"))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 1<<20 {
			t.Skip()
		}
		document, _, err := Parse(body)
		if err != nil {
			return
		}
		if document == nil {
			t.Fatal("nil document with no error")
		}
		if _, err := document.JSON(); err != nil {
			t.Fatalf("imported document is not serialisable: %v", err)
		}
	})
}
