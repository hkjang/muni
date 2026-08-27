package docx

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// Every other import test in this package feeds Parse something Build wrote,
// which can only find disagreements muni has with itself. wordPackage builds a
// .docx around a body muni would never write, so a test can describe what Word
// puts in a file and check what muni makes of it.
func wordPackage(t *testing.T, body string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	parts := []struct{ name, content string }{
		{"[Content_Types].xml", xmlHeader +
			`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
			`</Types>`},
		{"_rels/.rels", xmlHeader +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
			`</Relationships>`},
		{"word/document.xml", xmlHeader + `<w:document` + documentNamespaces + `><w:body>` + body + `</w:body></w:document>`},
		{"word/_rels/document.xml.rels", xmlHeader +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"/>`},
	}
	for _, part := range parts {
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
	return buffer.Bytes()
}

// wordParagraph wraps runs in a paragraph.
func wordParagraph(runs ...string) string {
	out := "<w:p>"
	for _, run := range runs {
		out += run
	}
	return out + "</w:p>"
}

func wordText(value string) string {
	return `<w:r><w:t xml:space="preserve">` + escapeXML(value) + `</w:t></w:r>`
}

func wordFieldChar(kind string) string {
	return `<w:r><w:fldChar w:fldCharType="` + kind + `"/></w:r>`
}

func wordInstruction(value string) string {
	return `<w:r><w:instrText xml:space="preserve">` + escapeXML(value) + `</w:instrText></w:r>`
}

// wordPackageWithStyles is wordPackage with a word/styles.xml, for the styles
// a document refers to instead of spelling out.
func wordPackageWithStyles(t *testing.T, body, styles string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	parts := []struct{ name, content string }{
		{"[Content_Types].xml", xmlHeader +
			`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
			`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
			`</Types>`},
		{"_rels/.rels", xmlHeader +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
			`</Relationships>`},
		{"word/document.xml", xmlHeader + `<w:document` + documentNamespaces + `><w:body>` + body + `</w:body></w:document>`},
		{"word/styles.xml", xmlHeader + `<w:styles` + documentNamespaces + `>` + styles + `</w:styles>`},
		{"word/_rels/document.xml.rels", xmlHeader +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rIdS" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
			`</Relationships>`},
	}
	for _, part := range parts {
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
	return buffer.Bytes()
}

// wordPackageWithNotes is wordPackage with the two note parts, for the notes a
// document refers to by number.
func wordPackageWithNotes(t *testing.T, body, footnotes, endnotes string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	parts := []struct{ name, content string }{
		{"[Content_Types].xml", xmlHeader +
			`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
			`</Types>`},
		{"_rels/.rels", xmlHeader +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
			`</Relationships>`},
		{"word/document.xml", xmlHeader + `<w:document` + documentNamespaces + `><w:body>` + body + `</w:body></w:document>`},
		{"word/footnotes.xml", xmlHeader + `<w:footnotes` + documentNamespaces + `>` + footnotes + `</w:footnotes>`},
		{"word/endnotes.xml", xmlHeader + `<w:endnotes` + documentNamespaces + `>` + endnotes + `</w:endnotes>`},
		{"word/_rels/document.xml.rels", xmlHeader +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"/>`},
	}
	for _, part := range parts {
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
	return buffer.Bytes()
}

// mustParseXML reads a fragment as a part root, for testing readers that take
// one.
func mustParseXML(t *testing.T, fragment string) *xnode {
	t.Helper()
	root, err := parseXML(strings.NewReader(xmlHeader + `<w:hdr` + documentNamespaces + `>` + fragment + `</w:hdr>`))
	if err != nil {
		t.Fatal(err)
	}
	return root
}
