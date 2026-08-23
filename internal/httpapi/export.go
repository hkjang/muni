package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *Server) exportDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	if _, err := s.documentRole(r.Context(), p.User, id, false); err != nil {
		writeError(w, 403, "DOCUMENT_PERMISSION_DENIED", "문서를 내보낼 권한이 없습니다.")
		return
	}
	format := strings.ToLower(r.PathValue("format"))
	if !contains([]string{"txt", "md", "html", "docx", "pdf"}, format) {
		writeError(w, 400, "UNSUPPORTED_EXPORT", "지원 형식은 txt, md, html, docx, pdf입니다.")
		return
	}
	all, _ := s.settings.GetAll(r.Context(), false)
	if format == "pdf" && !all.Export.EnablePDF {
		writeError(w, 403, "PDF_EXPORT_DISABLED", "관리자 정책에서 PDF 내보내기가 비활성화되어 있습니다.")
		return
	}
	if format == "docx" && !all.Export.EnableDOCX {
		writeError(w, 403, "DOCX_EXPORT_DISABLED", "관리자 정책에서 DOCX 내보내기가 비활성화되어 있습니다.")
		return
	}
	var title, text string
	var content json.RawMessage
	if err := s.db.QueryRow(r.Context(), `SELECT title,content_text,content_json FROM documents WHERE id=$1`, id).Scan(&title, &text, &content); err != nil {
		writeError(w, 404, "DOCUMENT_NOT_FOUND", "문서를 찾을 수 없습니다.")
		return
	}
	filename := safeFilename(title)
	var body []byte
	var contentType string
	var err error
	switch format {
	case "txt":
		body = []byte(text)
		contentType = "text/plain; charset=utf-8"
	case "md":
		body = []byte(renderMarkdown(content))
		contentType = "text/markdown; charset=utf-8"
	case "html":
		body = []byte(fullHTML(title, s.renderHTMLWithAttachments(r.Context(), id, content)))
		contentType = "text/html; charset=utf-8"
	case "docx":
		body, err = makeDOCX(title, content)
		contentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "pdf":
		body, err = makePDF(r.Context(), title, s.renderHTMLWithAttachments(r.Context(), id, content))
		contentType = "application/pdf"
	}
	if err != nil {
		writeError(w, 500, "EXPORT_FAILED", "문서를 내보내지 못했습니다: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="document.%s"; filename*=UTF-8''%s.%s`, format, urlPathEscape(filename), format))
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	w.WriteHeader(200)
	_, _ = w.Write(body)
	s.audit(r, &p.User.ID, "EXPORT_DOCUMENT", "DOCUMENT", &id, map[string]any{"format": format})
}

func (s *Server) renderHTMLWithAttachments(ctx context.Context, documentID uuid.UUID, content json.RawMessage) string {
	value := renderHTML(content)
	rows, err := s.db.Query(ctx, `SELECT id,media_type,data FROM attachments WHERE document_id=$1`, documentID)
	if err != nil {
		return value
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var mediaType string
		var data []byte
		if rows.Scan(&id, &mediaType, &data) != nil || !safeInlineImageType(mediaType) {
			continue
		}
		source := `/api/v1/attachments/` + id.String()
		encoded := `data:` + baseMediaType(mediaType) + `;base64,` + base64.StdEncoding.EncodeToString(data)
		value = strings.ReplaceAll(value, `src="`+source+`"`, `src="`+encoded+`"`)
	}
	return value
}

func renderHTML(raw json.RawMessage) string {
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		return ""
	}
	return renderHTMLNode(root)
}
func renderHTMLNode(node map[string]any) string {
	kind, _ := node["type"].(string)
	attrs, _ := node["attrs"].(map[string]any)
	children := ""
	if items, ok := node["content"].([]any); ok {
		for _, item := range items {
			if child, ok := item.(map[string]any); ok {
				children += renderHTMLNode(child)
			}
		}
	}
	if kind == "text" {
		value := html.EscapeString(fmt.Sprint(node["text"]))
		if marks, ok := node["marks"].([]any); ok {
			for _, mark := range marks {
				m, _ := mark.(map[string]any)
				switch m["type"] {
				case "bold":
					value = "<strong>" + value + "</strong>"
				case "italic":
					value = "<em>" + value + "</em>"
				case "underline":
					value = "<u>" + value + "</u>"
				case "strike":
					value = "<s>" + value + "</s>"
				case "code":
					value = "<code>" + value + "</code>"
				case "link":
					a, _ := m["attrs"].(map[string]any)
					href := html.EscapeString(fmt.Sprint(a["href"]))
					if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
						value = `<a href="` + href + `">` + value + `</a>`
					}
				}
			}
		}
		return value
	}
	switch kind {
	case "doc":
		return children
	case "paragraph":
		return "<p>" + children + "</p>"
	case "heading":
		level := intFrom(attrs["level"], 1)
		if level < 1 || level > 6 {
			level = 1
		}
		return fmt.Sprintf("<h%d>%s</h%d>", level, children, level)
	case "bulletList":
		return "<ul>" + children + "</ul>"
	case "orderedList":
		return "<ol>" + children + "</ol>"
	case "listItem":
		return "<li>" + children + "</li>"
	case "blockquote":
		return "<blockquote>" + children + "</blockquote>"
	case "codeBlock":
		return "<pre><code>" + children + "</code></pre>"
	case "hardBreak":
		return "<br>"
	case "horizontalRule":
		return "<hr>"
	case "table":
		return "<table>" + children + "</table>"
	case "tableRow":
		return "<tr>" + children + "</tr>"
	case "tableCell":
		return "<td>" + children + "</td>"
	case "tableHeader":
		return "<th>" + children + "</th>"
	case "image":
		src := html.EscapeString(fmt.Sprint(attrs["src"]))
		alt := html.EscapeString(fmt.Sprint(attrs["alt"]))
		if strings.HasPrefix(src, "data:image/") || strings.HasPrefix(src, "/api/v1/attachments/") {
			return `<img src="` + src + `" alt="` + alt + `">`
		}
		return ""
	default:
		return children
	}
}
func renderMarkdown(raw json.RawMessage) string {
	htmlValue := renderHTML(raw)
	replacer := strings.NewReplacer("<p>", "", "</p>", "\n\n", "<strong>", "**", "</strong>", "**", "<em>", "*", "</em>", "*", "<u>", "", "</u>", "", "<s>", "~~", "</s>", "~~", "<code>", "`", "</code>", "`", "<br>", "\n", "<hr>", "\n---\n", "<blockquote>", "> ", "</blockquote>", "\n", "<ul>", "", "</ul>", "\n", "<ol>", "", "</ol>", "\n", "<li>", "- ", "</li>", "\n")
	value := replacer.Replace(htmlValue)
	for i := 1; i <= 6; i++ {
		value = strings.ReplaceAll(value, fmt.Sprintf("<h%d>", i), strings.Repeat("#", i)+" ")
		value = strings.ReplaceAll(value, fmt.Sprintf("</h%d>", i), "\n\n")
	}
	return strings.TrimSpace(stripTags(value)) + "\n"
}
func stripTags(value string) string {
	var out strings.Builder
	inside := false
	for _, r := range value {
		if r == '<' {
			inside = true
			continue
		}
		if r == '>' {
			inside = false
			continue
		}
		if !inside {
			out.WriteRune(r)
		}
	}
	return html.UnescapeString(out.String())
}
func fullHTML(title, body string) string {
	return `<!doctype html><html lang="ko"><head><meta charset="utf-8"><title>` + html.EscapeString(title) + `</title><style>@page{margin:20mm}body{font-family:"Noto Sans CJK KR","Noto Sans KR",sans-serif;font-size:11pt;line-height:1.65;color:#202124;max-width:190mm;margin:auto}h1{font-size:24pt}h2{font-size:19pt}h3{font-size:16pt}table{border-collapse:collapse;width:100%}td,th{border:1px solid #c7c9d1;padding:6px 8px}img{max-width:100%}blockquote{border-left:3px solid #6b6bd6;margin-left:0;padding-left:14px;color:#555}</style></head><body><h1>` + html.EscapeString(title) + `</h1>` + body + `</body></html>`
}

func makeDOCX(title string, content json.RawMessage) ([]byte, error) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	files := map[string]string{"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`, "_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`, "word/document.xml": docxDocument(title, content)}
	for name, value := range files {
		writer, err := archive.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err = io.WriteString(writer, value); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
func docxDocument(title string, raw json.RawMessage) string {
	text := extractDocumentText(raw)
	paragraphs := strings.Split(text, "\n")
	var body strings.Builder
	body.WriteString(`<w:p><w:pPr><w:pStyle w:val="Title"/></w:pPr><w:r><w:t>` + xmlEscape(title) + `</w:t></w:r></w:p>`)
	for _, p := range paragraphs {
		body.WriteString(`<w:p><w:r><w:t xml:space="preserve">` + xmlEscape(p) + `</w:t></w:r></w:p>`)
	}
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` + body.String() + `<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1134" w:right="1134" w:bottom="1134" w:left="1134"/></w:sectPr></w:body></w:document>`
}
func xmlEscape(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&apos;").Replace(value)
}
func makePDF(parent context.Context, title, renderedHTML string) ([]byte, error) {
	tempDir, err := os.MkdirTemp("", "muni-pdf-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)
	htmlPath := filepath.Join(tempDir, "document.html")
	pdfPath := filepath.Join(tempDir, "document.pdf")
	if err = os.WriteFile(htmlPath, []byte(fullHTML(title, renderedHTML)), 0600); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(parent, 60*time.Second)
	defer cancel()
	binary := "chromium"
	if _, err = exec.LookPath(binary); err != nil {
		binary = "chromium-browser"
	}
	command := exec.CommandContext(ctx, binary, "--headless=new", "--no-sandbox", "--disable-gpu", "--disable-dev-shm-usage", "--no-pdf-header-footer", "--print-to-pdf="+pdfPath, "file://"+htmlPath)
	if output, runErr := command.CombinedOutput(); runErr != nil {
		return nil, fmt.Errorf("Chromium PDF: %v (%s)", runErr, truncate(string(output), 300))
	}
	return os.ReadFile(pdfPath)
}
func safeFilename(value string) string {
	value = strings.TrimSpace(value)
	value = strings.NewReplacer("/", "-", "\\", "-", "\r", " ", "\n", " ", "\x00", "").Replace(value)
	if value == "" {
		return "muni-document"
	}
	return truncateRunes(value, 100)
}
func urlPathEscape(value string) string {
	replacer := strings.NewReplacer("%", "%25", " ", "%20", "#", "%23", "?", "%3F", "\"", "%22")
	return replacer.Replace(value)
}
func intFrom(value any, fallback int) int {
	switch n := value.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return fallback
	}
}
