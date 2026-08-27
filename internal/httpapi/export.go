package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/docx"
	"github.com/hkjang/muni/internal/richdoc"
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
	var title, numbering, pageHeader, pageFooter, orientation string
	var content json.RawMessage
	if err := s.db.QueryRow(r.Context(), `SELECT title,content_json,heading_numbering,page_header,page_footer,page_orientation FROM documents WHERE id=$1`, id).Scan(&title, &content, &numbering, &pageHeader, &pageFooter, &orientation); err != nil {
		writeError(w, 404, "DOCUMENT_NOT_FOUND", "문서를 찾을 수 없습니다.")
		return
	}
	// Numbering is applied once, here, so every format sees the same headings
	// and the contents list picks the numbers up with them.
	content = numberedContent(content, numbering)
	landscape := orientation == "LANDSCAPE"
	filename := safeFilename(title)
	var body []byte
	var contentType string
	var err error
	switch format {
	case "txt":
		body = []byte(renderPlainText(title, content))
		contentType = "text/plain; charset=utf-8"
	case "md":
		body = []byte(renderMarkdown(title, content))
		contentType = "text/markdown; charset=utf-8"
	case "html":
		rendered := s.renderHTMLWithAttachments(r.Context(), id, content)
		body = []byte(fullHTMLWithDrawing(title, landscape, rendered, htmlHasDiagram(rendered)))
		contentType = "text/html; charset=utf-8"
	case "docx":
		body, err = s.makeDOCX(r.Context(), id, title, content, p.User.DisplayName, pageHeader, pageFooter, landscape)
		contentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "pdf":
		body, err = makePDF(r.Context(), title, pageHeader, pageFooter, landscape, s.renderHTMLWithAttachments(r.Context(), id, content))
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
	s.audit(r, &p.User.ID, "EXPORT_DOCUMENT", "DOCUMENT", &id, map[string]any{"format": format, "bytes": len(body)})
}

// makeDOCX renders a Word file that keeps the document's headings, lists,
// tables, inline formatting and images.
func (s *Server) makeDOCX(ctx context.Context, documentID uuid.UUID, title string, content json.RawMessage, author, pageHeader, pageFooter string, landscape bool) ([]byte, error) {
	document, err := richdoc.Parse(content)
	if err != nil {
		return nil, fmt.Errorf("문서 구조를 읽지 못했습니다: %w", err)
	}
	document = richdoc.WithTableOfContents(document)
	images := s.attachmentImages(ctx, documentID)
	return docx.Build(document, docx.Options{
		Title:     title,
		Author:    author,
		Generator: "muni " + s.info.Version,
		Created:   time.Now().UTC(),
		Header:    pageHeader,
		Footer:    pageFooter,
		Landscape: landscape,
		ResolveImage: func(src string) (docx.Image, bool) {
			if picture, ok := images[src]; ok {
				return picture, true
			}
			return docx.DecodeDataURI(src)
		},
	})
}

func (s *Server) attachmentImages(ctx context.Context, documentID uuid.UUID) map[string]docx.Image {
	images := map[string]docx.Image{}
	rows, err := s.db.Query(ctx, `SELECT id,name,media_type,data FROM attachments WHERE document_id=$1`, documentID)
	if err != nil {
		return images
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var name, mediaType string
		var data []byte
		if rows.Scan(&id, &name, &mediaType, &data) != nil || !safeInlineImageType(mediaType) {
			continue
		}
		images["/api/v1/attachments/"+id.String()] = docx.Image{Data: data, MediaType: baseMediaType(mediaType), Name: name}
	}
	return images
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

// pdfSlots bounds how many headless browsers run at once. Each Chromium
// instance costs a few hundred megabytes, so an unbounded queue of exports
// would exhaust a container long before the CPU is busy.
var pdfSlots = make(chan struct{}, pdfConcurrency())

func pdfConcurrency() int {
	if value, err := strconv.Atoi(strings.TrimSpace(os.Getenv("MUNI_PDF_CONCURRENCY"))); err == nil && value >= 1 && value <= 32 {
		return value
	}
	return 2
}

func makePDF(parent context.Context, title, pageHeader, pageFooter string, landscape bool, renderedHTML string) ([]byte, error) {
	// Wait for a rendering slot, but never longer than the caller allows.
	waitCtx, cancelWait := context.WithTimeout(parent, 60*time.Second)
	defer cancelWait()
	select {
	case pdfSlots <- struct{}{}:
		defer func() { <-pdfSlots }()
	case <-waitCtx.Done():
		return nil, errors.New("PDF 변환 요청이 많아 처리하지 못했습니다. 잠시 후 다시 시도해 주세요")
	}

	tempDir, err := os.MkdirTemp("", "muni-pdf-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)
	htmlPath := filepath.Join(tempDir, "document.html")
	pdfPath := filepath.Join(tempDir, "document.pdf")
	draws := htmlHasDiagram(renderedHTML)
	if err = os.WriteFile(htmlPath, []byte(fullHTMLWithDrawing(title, landscape, renderedHTML, draws)), 0600); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(parent, 120*time.Second)
	defer cancel()
	binary, err := chromiumBinary()
	if err != nil {
		return nil, err
	}
	// The protocol path is what puts page numbers at the bottom; the command
	// line cannot. It has more ways to fail than running a program does, so a
	// failure falls back rather than losing the export.
	if body, devErr := printToPDFWithDevtools(ctx, binary, tempDir, htmlPath, pdfFurniture{Title: title, Header: pageHeader, Footer: pageFooter, Landscape: landscape, Draws: draws}); devErr == nil {
		return body, nil
	} else {
		pdfLogger.Warn("devtools pdf failed, falling back to the command line", "error", devErr)
	}

	command := chromiumCommand(ctx, binary, tempDir, htmlPath, pdfPath)
	if output, runErr := command.CombinedOutput(); runErr != nil {
		if _, statErr := os.Stat(pdfPath); statErr != nil {
			return nil, fmt.Errorf("Chromium PDF: %v (%s)", runErr, truncate(string(output), 300))
		}
	}
	body, err := os.ReadFile(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("Chromium이 PDF를 생성하지 못했습니다: %w", err)
	}
	if len(body) == 0 {
		return nil, errors.New("Chromium이 빈 PDF를 생성했습니다")
	}
	return body, nil
}

// chromiumCommand builds the headless render invocation. muni runs as a user
// whose home directory does not exist, and Chromium's crash handler aborts the
// browser when it cannot create its database under HOME — so the child gets a
// writable HOME inside the per-render temporary directory.
func chromiumCommand(ctx context.Context, binary, tempDir, htmlPath, pdfPath string) *exec.Cmd {
	command := exec.CommandContext(ctx, binary,
		"--headless=new", "--no-sandbox", "--disable-gpu", "--disable-dev-shm-usage",
		"--no-pdf-header-footer", "--print-to-pdf-no-header",
		"--user-data-dir="+filepath.Join(tempDir, "profile"),
		"--print-to-pdf="+pdfPath, "file://"+htmlPath)
	command.Env = append(os.Environ(),
		"HOME="+tempDir,
		"XDG_CONFIG_HOME="+filepath.Join(tempDir, "config"),
		"XDG_CACHE_HOME="+filepath.Join(tempDir, "cache"),
	)
	return command
}

// chromiumBinary locates a headless browser, allowing operators to override
// the choice through MUNI_CHROMIUM_PATH.
func chromiumBinary() (string, error) {
	candidates := []string{os.Getenv("MUNI_CHROMIUM_PATH"), "chromium", "chromium-browser", "google-chrome", "google-chrome-stable", "chrome"}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", errors.New("PDF 변환에 필요한 Chromium을 찾을 수 없습니다. 서버에 chromium을 설치하거나 MUNI_CHROMIUM_PATH를 설정하세요")
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

func fullHTML(title string, landscape bool, body string) string {
	return fullHTMLWithDrawing(title, landscape, body, false)
}

// fullHTMLWithDrawing is fullHTML with the drawing library carried inside it.
//
// Only when the document has something to draw: the library is three and a
// half megabytes, and an exported report with no diagram in it should not be
// three and a half megabytes larger than the report.
func fullHTMLWithDrawing(title string, landscape bool, body string, draw bool) string {
	// A turned page needs both the sheet and the text column to change; the
	// stylesheet caps the column at the portrait width, which would leave a
	// landscape print with the same narrow text and wide empty margins.
	page := `@page{size:A4 portrait;margin:20mm}`
	column := ""
	if landscape {
		page = `@page{size:A4 landscape;margin:20mm}`
		column = `body{max-width:257mm}`
	}
	drawing := ""
	if draw {
		if library := drawingLibrary(); len(library) > 0 {
			// Inlined rather than linked: an exported file is one file, and it
			// has to draw the same on a machine with no network.
			drawing = `<script>` + string(library) + `</script><script>` + diagramBootScript + `</script>`
		}
	}
	return `<!doctype html><html lang="ko"><head><meta charset="utf-8"><title>` + html.EscapeString(title) +
		`</title><style>` + page + exportStylesheet + diagramStyle + column + `</style></head><body><h1 class="doc-title">` +
		html.EscapeString(title) + `</h1>` + body + drawing + `</body></html>`
}

const exportStylesheet = `
*{box-sizing:border-box}
body{font-family:"Noto Sans CJK KR","Noto Sans KR","Malgun Gothic",sans-serif;font-size:11pt;line-height:1.65;color:#202124;max-width:190mm;margin:auto}
.muni-footnote-ref{font-size:0.75em;line-height:0}
.muni-footnote-ref a{text-decoration:none;color:#1a56c4}
.muni-footnote-rule{margin:24pt 0 8pt;border:none;border-top:1px solid #c8ccd8;width:33%}
.muni-footnotes{font-size:9pt;color:#3c4043;line-height:1.5;padding-left:18pt}
.muni-footnotes li{margin:0 0 4pt}
.muni-footnote-back{text-decoration:none;color:#9aa0a6}
h1{font-size:24pt;margin:0 0 12pt}h2{font-size:19pt;margin:18pt 0 8pt}h3{font-size:16pt;margin:16pt 0 6pt}
h4{font-size:13pt;margin:14pt 0 6pt}h5,h6{font-size:11.5pt;margin:12pt 0 6pt}
h1,h2,h3,h4,h5,h6{page-break-after:avoid;color:#14142b}
p{margin:0 0 8pt}
table{border-collapse:collapse;width:100%;margin:10pt 0}
thead{display:table-header-group}
tr{page-break-inside:avoid}
td,th{border:1px solid #c7c9d1;padding:6px 8px;vertical-align:top}
th{background:#f3f4fa;text-align:left}
img{max-width:100%;height:auto;page-break-inside:avoid}
blockquote{border-left:3px solid #6b6bd6;margin:10pt 0;padding-left:14px;color:#555;font-style:italic}
pre{background:#f5f6fa;border:1px solid #d8dae5;border-radius:4px;padding:10px 12px;white-space:pre-wrap;word-break:break-word;font-family:Consolas,"D2Coding",monospace;font-size:9.5pt}
code{font-family:Consolas,"D2Coding",monospace;font-size:9.5pt;background:#f2f2f7;border-radius:3px;padding:0 3px}
pre code{background:none;padding:0}
hr{border:none;border-top:1px solid #c7c9d1;margin:14pt 0}
ul,ol{margin:0 0 8pt;padding-left:22pt}
li{margin:0 0 3pt}
ul[data-type="taskList"]{list-style:none;padding-left:4pt}
ul[data-type="taskList"] li{display:flex;gap:6px;align-items:flex-start}
mark{padding:0 2px;border-radius:2px}
a{color:#1155cc}`

// numberedContent writes the heading numbers into a copy of the document.
//
// The numbers are never stored: one written into the document would be wrong
// the moment a section moved. A document that cannot be parsed is returned as
// it is — an export that loses the numbering is better than one that fails.
func numberedContent(content json.RawMessage, scheme string) json.RawMessage {
	if richdoc.ValidNumbering(scheme) == richdoc.NumberingNone {
		return content
	}
	document, err := richdoc.Parse(content)
	if err != nil {
		return content
	}
	encoded, err := richdoc.WithHeadingNumbers(document, scheme).JSON()
	if err != nil {
		return content
	}
	return encoded
}

// pdfLogger records why the protocol path was not used, which is the only
// sign an operator would otherwise get that their PDFs lost their page
// numbers.
var pdfLogger = slog.Default().With("component", "pdf")
