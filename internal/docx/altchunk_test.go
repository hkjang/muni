package docx

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// simpleDocx builds a package around one body, optionally naming an embedded
// document the way Word does when a file is inserted rather than merged.
func simpleDocx(t *testing.T, body string, chunk []byte) []byte {
	t.Helper()
	const header = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`
	const namespaces = ` xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"` +
		` xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	write := func(name, data string) {
		part, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	write("word/document.xml", header+`<w:document`+namespaces+`><w:body>`+body+`</w:body></w:document>`)
	rels := `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`
	if chunk != nil {
		rels += `<Relationship Id="rIdChunk" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/aFChunk" Target="afchunk.docx"/>`
	}
	rels += `</Relationships>`
	write("word/_rels/document.xml.rels", header+rels)
	if chunk != nil {
		part, err := archive.Create("word/afchunk.docx")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// Inserting a file into a Word document does not merge it: Word writes the
// file whole and leaves one <w:altChunk> pointing at it. Everything the
// inserted file said used to arrive as nothing at all.
func TestAnEmbeddedDocumentIsRead(t *testing.T) {
	inner := simpleDocx(t, `<w:p><w:r><w:t>끼워 넣은 문서의 글</w:t></w:r></w:p>`, nil)
	outer := simpleDocx(t,
		`<w:p><w:r><w:t>앞 문단</w:t></w:r></w:p>`+
			`<w:altChunk r:id="rIdChunk"/>`+
			`<w:p><w:r><w:t>뒤 문단</w:t></w:r></w:p>`, inner)

	document, _, _, err := Parse(outer)
	if err != nil {
		t.Fatal(err)
	}
	text := document.PlainText()
	for _, want := range []string{"앞 문단", "끼워 넣은 문서의 글", "뒤 문단"} {
		if !strings.Contains(text, want) {
			t.Errorf("%q가 없습니다: %q", want, text)
		}
	}
	// And in the order the document puts them.
	if strings.Index(text, "앞 문단") > strings.Index(text, "끼워 넣은 문서의 글") ||
		strings.Index(text, "끼워 넣은 문서의 글") > strings.Index(text, "뒤 문단") {
		t.Errorf("차례가 어긋났습니다: %q", text)
	}
}

// A package that embeds itself would otherwise be read for ever.
func TestAnEmbeddedDocumentCannotNestForEver(t *testing.T) {
	body := `<w:p><w:r><w:t>속</w:t></w:r></w:p><w:altChunk r:id="rIdChunk"/>`
	level := simpleDocx(t, `<w:p><w:r><w:t>바닥</w:t></w:r></w:p>`, nil)
	for index := 0; index < 12; index++ {
		level = simpleDocx(t, body, level)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, _, _, err := Parse(level); err != nil {
			t.Errorf("읽지 못했습니다: %v", err)
		}
	}()
	<-done
}
