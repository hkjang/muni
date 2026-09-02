package hwpx

import (
	"archive/zip"
	"bytes"
	"testing"
)

// minimalHWPX is one paragraph in a zip, as a seed the fuzzer can mutate into
// something interesting rather than starting from noise.
func minimalHWPX() []byte {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for _, part := range []struct{ name, content string }{
		{"mimetype", "application/hwp+zip"},
		{"Contents/header.xml", headerXML},
		{"Contents/section0.xml", `<?xml version="1.0"?><hs:sec xmlns:hs="s" xmlns:hp="p">` +
			`<hp:p paraPrIDRef="0" styleIDRef="0"><hp:run charPrIDRef="0">` +
			`<hp:t>씨앗 문단</hp:t></hp:run></hp:p></hs:sec>`},
	} {
		writer, err := archive.Create(part.name)
		if err != nil {
			return nil
		}
		if _, err := writer.Write([]byte(part.content)); err != nil {
			return nil
		}
	}
	if err := archive.Close(); err != nil {
		return nil
	}
	return buffer.Bytes()
}

// FuzzParse feeds malformed packages to the HWPX reader, which runs directly
// on uploaded files.
//
// The reader walks a zip's parts and an XML tree built from whatever they
// hold, and every one of those steps is a chance to read past the end of
// something or to follow a file into a loop.
func FuzzParse(f *testing.F) {
	f.Add(minimalHWPX())
	f.Add([]byte("PK\x03\x04 not really a zip"))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 1<<20 {
			t.Skip()
		}
		document, assets, _, err := Parse(body)
		if err != nil {
			return
		}
		if document == nil {
			t.Fatal("nil document with no error")
		}
		if _, err := document.JSON(); err != nil {
			t.Fatalf("imported document is not serialisable: %v", err)
		}
		for _, asset := range assets {
			if asset.Placeholder == "" {
				t.Fatal("an asset with no placeholder cannot be referred to")
			}
		}
	})
}
