package docx

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// A part nested past anything Word writes is not a document. The tree is
// built without recursion, but every reader of it recurses, and running out
// of stack kills the process rather than failing the request.
func TestADeeplyNestedPartIsRefusedRatherThanCrashing(t *testing.T) {
	var body strings.Builder
	body.WriteString(`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	const depth = 20000
	for index := 0; index < depth; index++ {
		body.WriteString("<w:tbl><w:tr><w:tc>")
	}
	body.WriteString("<w:p><w:r><w:t>깊은 글</w:t></w:r></w:p>")
	for index := 0; index < depth; index++ {
		body.WriteString("</w:tc></w:tr></w:tbl>")
	}
	body.WriteString(`</w:body></w:document>`)

	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	part, err := archive.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(body.String())); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}

	if _, _, _, err := Parse(buffer.Bytes()); err == nil {
		t.Errorf("너무 깊게 중첩된 파일을 받아들였습니다")
	}
}

// The bytes of a part are bounded, but a tree costs many times what its text
// does: a part of nothing but empty tags turns a small upload into a large
// heap. So the pieces are counted too.
func TestAPartOfTooManyPiecesIsRefused(t *testing.T) {
	var body strings.Builder
	body.WriteString(`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	for index := 0; index <= maxElements; index++ {
		body.WriteString("<w:p/>")
	}
	body.WriteString(`</w:body></w:document>`)

	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	part, err := archive.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(body.String())); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := Parse(buffer.Bytes()); err == nil {
		t.Errorf("조각이 %d개가 넘는 파일을 받아들였습니다", maxElements)
	}
}
