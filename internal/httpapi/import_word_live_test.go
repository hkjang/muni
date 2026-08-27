package httpapi

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Everything else about the Word importer is tested a layer down, against
// docx.Parse. This is the whole way through: a file uploaded to the endpoint
// a person uses, stored, and read back the way the editor reads it.
//
// The shapes here are the ones muni used to get wrong. The picture is the one
// that mattered most — a paragraph holding one is a document the editor
// refuses outright, so a .docx with a photo in it did not open at all.

// onePixelPNG is the smallest picture that is really a picture.
var onePixelPNG = []byte{
	0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0a, 'I', 'D', 'A', 'T', 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 'I', 'E', 'N', 'D', 0xae,
	0x42, 0x60, 0x82,
}

const wordNamespaces = ` xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"` +
	` xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"` +
	` xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"` +
	` xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"` +
	` xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"` +
	` xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"` +
	` xmlns:v="urn:schemas-microsoft-com:vml"`

// wordFileAWordUserWouldSend builds a .docx carrying the shapes muni used to
// lose: a picture inside a sentence, a text box, a styled paragraph, a table
// whose header is drawn by its style, and a table of contents field.
func wordFileAWordUserWouldSend(t *testing.T) []byte {
	t.Helper()
	const header = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`

	drawing := `<w:drawing><wp:inline><wp:extent cx="9525" cy="9525"/><wp:docPr id="1" name="사진" descr="현장 사진"/>` +
		`<a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture">` +
		`<pic:pic><pic:blipFill><a:blip r:embed="rIdImg"/></pic:blipFill></pic:pic>` +
		`</a:graphicData></a:graphic></wp:inline></w:drawing>`

	body := `<w:p><w:pPr><w:pStyle w:val="Body"/></w:pPr><w:r><w:t>들여쓴 본문입니다</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>사진 앞 </w:t></w:r><w:r>` + drawing + `</w:r><w:r><w:t> 사진 뒤</w:t></w:r></w:p>` +
		`<w:p><w:r><mc:AlternateContent><mc:Fallback><w:pict><v:shape><v:textbox><w:txbxContent>` +
		`<w:p><w:r><w:t>붙임 제1호</w:t></w:r></w:p></w:txbxContent></v:textbox></v:shape></w:pict>` +
		`</mc:Fallback></mc:AlternateContent></w:r></w:p>` +
		`<w:p><w:r><w:fldChar w:fldCharType="begin"/></w:r>` +
		`<w:r><w:instrText xml:space="preserve"> TOC \o "1-3" \h </w:instrText></w:r>` +
		`<w:r><w:fldChar w:fldCharType="separate"/></w:r></w:p>` +
		`<w:p><w:r><w:t>굳어버린 목차 항목	1</w:t></w:r></w:p>` +
		`<w:p><w:r><w:fldChar w:fldCharType="end"/></w:r></w:p>` +
		`<w:p><w:r><w:rPr><w:rStyle w:val="Strong"/></w:rPr><w:t>문자스타일로 굵게</w:t></w:r></w:p>` +
		`<w:tbl><w:tblPr><w:tblStyle w:val="Shaded"/><w:tblLook w:val="04A0" w:firstRow="1"/></w:tblPr>` +
		`<w:tr><w:tc><w:tcPr/><w:p><w:r><w:t>표머리글</w:t></w:r></w:p></w:tc></w:tr>` +
		`<w:tr><w:tc><w:tcPr/><w:p><w:r><w:t>표값</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`

	styles := `<w:style w:type="paragraph" w:styleId="Body"><w:name w:val="본문"/>` +
		`<w:pPr><w:ind w:left="800"/><w:spacing w:line="360" w:lineRule="auto"/></w:pPr></w:style>` +
		`<w:style w:type="character" w:styleId="Strong"><w:name w:val="Strong"/><w:rPr><w:b/></w:rPr></w:style>` +
		`<w:style w:type="table" w:styleId="Shaded"><w:name w:val="Light Shading"/>` +
		`<w:tblStylePr w:type="firstRow"><w:rPr><w:b/></w:rPr></w:tblStylePr></w:style>`

	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	write := func(name string, data []byte) {
		writer, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	write("[Content_Types].xml", []byte(header+
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`+
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`+
		`<Default Extension="xml" ContentType="application/xml"/>`+
		`<Default Extension="png" ContentType="image/png"/>`+
		`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>`+
		`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>`+
		`</Types>`))
	write("_rels/.rels", []byte(header+
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>`+
		`</Relationships>`))
	write("word/document.xml", []byte(header+`<w:document`+wordNamespaces+`><w:body>`+body+`</w:body></w:document>`))
	write("word/styles.xml", []byte(header+`<w:styles`+wordNamespaces+`>`+styles+`</w:styles>`))
	write("word/_rels/document.xml.rels", []byte(header+
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
		`<Relationship Id="rIdImg" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/image1.png"/>`+
		`<Relationship Id="rIdSty" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>`+
		`</Relationships>`))
	write("word/media/image1.png", onePixelPNG)
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func importWordFile(t *testing.T, srv *serverUnderTest, workspaceID uuid.UUID, file []byte) map[string]any {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("workspaceId", workspaceID.String()); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", "보고서.docx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(file); err != nil {
		t.Fatal(err)
	}
	writer.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := srv.admin.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		t.Fatalf("import = %d %s", resp.StatusCode, raw)
	}
	var out struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out.Data
}

func adminWorkspace(t *testing.T, srv *serverUnderTest) uuid.UUID {
	t.Helper()
	var adminID, workspaceID uuid.UUID
	if err := srv.db.QueryRow(t.Context(), `SELECT id FROM users WHERE email='admin@muni.local'`).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	if err := srv.db.QueryRow(t.Context(), `SELECT id FROM workspaces WHERE owner_id=$1 LIMIT 1`, adminID).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	return workspaceID
}

func TestAWordFileArrivesWholeThroughTheEndpoint(t *testing.T) {
	srv := newServerUnderTest(t)
	document := importWordFile(t, srv, adminWorkspace(t, srv), wordFileAWordUserWouldSend(t))

	content, err := json.Marshal(document["content"])
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)

	// The words that used to go missing.
	for _, phrase := range []string{"들여쓴 본문입니다", "사진 앞", "사진 뒤", "붙임 제1호", "문자스타일로 굵게", "표머리글", "표값"} {
		if !strings.Contains(text, phrase) {
			t.Errorf("%q 가 사라졌습니다", phrase)
		}
	}
	// The stale contents cache is not one of them.
	if strings.Contains(text, "굳어버린 목차 항목") {
		t.Error("굳어버린 목차가 본문에 남았습니다")
	}
	for _, want := range []string{
		`"tableOfContents"`, // the field became a living contents node
		`"tableHeader"`,     // the style drew the header row
		`"bold"`,            // the character style brought its weight
		`"indent"`,          // the paragraph style brought its layout
	} {
		if !strings.Contains(text, want) {
			t.Errorf("%s 가 없습니다: %s", want, text)
		}
	}

	// And the picture is a block of its own, which is the only shape the
	// editor will open.
	var parsed struct {
		Content []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
			} `json:"content"`
		} `json:"content"`
	}
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatal(err)
	}
	images := 0
	for _, block := range parsed.Content {
		if block.Type == "image" {
			images++
		}
		for _, child := range block.Content {
			if child.Type == "image" {
				t.Fatalf("이미지가 %s 안에 갇혔습니다: %s", block.Type, text)
			}
		}
	}
	if images != 1 {
		t.Errorf("이미지 블록 = %d개", images)
	}
	// The picture's bytes became an attachment the editor can ask for.
	if !strings.Contains(text, "/api/v1/attachments/") {
		t.Errorf("이미지가 첨부로 저장되지 않았습니다: %s", text)
	}
}
