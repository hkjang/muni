package httpapi

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/database"
	"github.com/jackc/pgx/v5"
	xhtml "golang.org/x/net/html"
)

func (s *Server) importDocument(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	all, err := s.settings.GetAll(r.Context(), false)
	if err != nil {
		writeError(w, 500, "SETTINGS_ERROR", "업로드 설정을 불러오지 못했습니다.")
		return
	}
	maxBytes := int64(all.Security.MaxUploadMB) << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20))
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		writeError(w, 413, "FILE_TOO_LARGE", fmt.Sprintf("파일은 %dMB 이하여야 합니다.", all.Security.MaxUploadMB))
		return
	}
	workspaceID, err := uuid.Parse(r.FormValue("workspaceId"))
	if err != nil {
		writeError(w, 400, "WORKSPACE_REQUIRED", "워크스페이스가 필요합니다.")
		return
	}
	var memberRole, workspaceKind string
	if s.db.QueryRow(r.Context(), `SELECT wm.role,w.kind FROM workspace_members wm JOIN workspaces w ON w.id=wm.workspace_id WHERE wm.workspace_id=$1 AND wm.user_id=$2`, workspaceID, p.User.ID).Scan(&memberRole, &workspaceKind) != nil || memberRole == "VIEWER" {
		writeError(w, 403, "WORKSPACE_PERMISSION_DENIED", "이 워크스페이스로 가져올 권한이 없습니다.")
		return
	}
	var folderID *uuid.UUID
	if value := strings.TrimSpace(r.FormValue("folderId")); value != "" {
		parsed, parseErr := uuid.Parse(value)
		if parseErr != nil {
			writeError(w, 400, "INVALID_FOLDER", "폴더 식별자가 올바르지 않습니다.")
			return
		}
		var validFolder bool
		_ = s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM folders WHERE id=$1 AND workspace_id=$2 AND deleted_at IS NULL)`, parsed, workspaceID).Scan(&validFolder)
		if !validFolder {
			writeError(w, 400, "INVALID_FOLDER", "워크스페이스에 속한 폴더를 선택해 주세요.")
			return
		}
		folderID = &parsed
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, 400, "FILE_REQUIRED", "가져올 파일이 필요합니다.")
		return
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || int64(len(body)) > maxBytes {
		writeError(w, 413, "FILE_TOO_LARGE", "파일 크기 제한을 초과했습니다.")
		return
	}
	extension := strings.ToLower(filepath.Ext(header.Filename))
	var content json.RawMessage
	switch extension {
	case ".txt":
		content, err = plainTextDocument(string(body))
	case ".md", ".markdown":
		content, err = markdownDocument(string(body))
	case ".html", ".htm":
		content, err = htmlDocument(body)
	case ".docx":
		content, err = docxImport(body)
	default:
		writeError(w, 400, "UNSUPPORTED_IMPORT", "지원 형식은 DOCX, Markdown, TXT, HTML입니다.")
		return
	}
	if err != nil {
		writeError(w, 400, "IMPORT_PARSE_FAILED", "파일 내용을 읽지 못했습니다: "+err.Error())
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(header.Filename), filepath.Ext(header.Filename))
	}
	if title == "" {
		title = "가져온 문서"
	}
	title = truncateRunes(title, 240)
	documentID := uuid.New()
	text := extractDocumentText(content)
	visibility := "RESTRICTED"
	if workspaceKind != "PERSONAL" {
		visibility = "WORKSPACE"
	}
	err = database.WithTx(r.Context(), s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(), `INSERT INTO documents(id,workspace_id,folder_id,owner_id,title,visibility,content_json,content_text,revision_no) VALUES($1,$2,$3,$4,$5,$6,$7,$8,1)`, documentID, workspaceID, folderID, p.User.ID, title, visibility, content, text); err != nil {
			return err
		}
		_, err := tx.Exec(r.Context(), `INSERT INTO document_revisions(document_id,revision_no,content_json,content_text,author_id,reason) VALUES($1,1,$2,$3,$4,$5)`, documentID, content, text, p.User.ID, "import:"+strings.TrimPrefix(extension, "."))
		return err
	})
	if err != nil {
		writeError(w, 500, "IMPORT_FAILED", "가져온 문서를 저장하지 못했습니다.")
		return
	}
	s.audit(r, &p.User.ID, "IMPORT_DOCUMENT", "DOCUMENT", &documentID, map[string]any{"format": strings.TrimPrefix(extension, "."), "bytes": len(body)})
	s.getDocumentByID(w, r, documentID)
}

func plainTextDocument(value string) (json.RawMessage, error) {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	content := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		content = append(content, textNodeBlock("paragraph", line, nil))
	}
	return json.Marshal(map[string]any{"type": "doc", "content": content})
}

func markdownDocument(value string) (json.RawMessage, error) {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	content := make([]map[string]any, 0, len(lines))
	inCode := false
	code := make([]string, 0)
	flushCode := func() {
		if len(code) > 0 {
			content = append(content, textNodeBlock("codeBlock", strings.Join(code, "\n"), nil))
			code = nil
		}
	}
	for _, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		if strings.HasPrefix(line, "```") {
			if inCode {
				flushCode()
			}
			inCode = !inCode
			continue
		}
		if inCode {
			code = append(code, line)
			continue
		}
		if strings.HasPrefix(line, "#") {
			level := 0
			for level < len(line) && line[level] == '#' {
				level++
			}
			if level <= 6 && level < len(line) && line[level] == ' ' {
				content = append(content, textNodeBlock("heading", strings.TrimSpace(line[level:]), map[string]any{"level": level}))
				continue
			}
		}
		if strings.HasPrefix(line, "> ") {
			content = append(content, map[string]any{"type": "blockquote", "content": []map[string]any{textNodeBlock("paragraph", strings.TrimPrefix(line, "> "), nil)}})
			continue
		}
		if strings.HasPrefix(line, "- [ ] ") || strings.HasPrefix(line, "- [x] ") {
			checked := strings.HasPrefix(line, "- [x]")
			content = append(content, map[string]any{"type": "taskList", "content": []map[string]any{{"type": "taskItem", "attrs": map[string]any{"checked": checked}, "content": []map[string]any{textNodeBlock("paragraph", line[6:], nil)}}}})
			continue
		}
		content = append(content, textNodeBlock("paragraph", line, nil))
	}
	flushCode()
	return json.Marshal(map[string]any{"type": "doc", "content": content})
}

func htmlDocument(body []byte) (json.RawMessage, error) {
	root, err := xhtml.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	content := make([]map[string]any, 0)
	var textContent func(*xhtml.Node) string
	textContent = func(node *xhtml.Node) string {
		if node.Type == xhtml.TextNode {
			return node.Data
		}
		if node.Type == xhtml.ElementNode && (node.Data == "script" || node.Data == "style") {
			return ""
		}
		var out strings.Builder
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			out.WriteString(textContent(child))
		}
		return out.String()
	}
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode {
			name := strings.ToLower(node.Data)
			text := strings.TrimSpace(textContent(node))
			switch name {
			case "h1", "h2", "h3", "h4", "h5", "h6":
				level := int(name[1] - '0')
				content = append(content, textNodeBlock("heading", text, map[string]any{"level": level}))
				return
			case "p", "div":
				if text != "" {
					content = append(content, textNodeBlock("paragraph", text, nil))
					return
				}
			case "blockquote":
				content = append(content, map[string]any{"type": "blockquote", "content": []map[string]any{textNodeBlock("paragraph", text, nil)}})
				return
			case "pre":
				content = append(content, textNodeBlock("codeBlock", text, nil))
				return
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	if len(content) == 0 {
		content = append(content, textNodeBlock("paragraph", strings.TrimSpace(textContent(root)), nil))
	}
	return json.Marshal(map[string]any{"type": "doc", "content": content})
}

func docxImport(body []byte) (json.RawMessage, error) {
	archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, err
	}
	var document *zip.File
	for _, file := range archive.File {
		if file.Name == "word/document.xml" {
			document = file
			break
		}
	}
	if document == nil {
		return nil, errors.New("word/document.xml이 없습니다")
	}
	reader, err := document.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	decoder := xml.NewDecoder(reader)
	paragraphs := make([]string, 0)
	var current strings.Builder
	paragraphDepth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch value := token.(type) {
		case xml.StartElement:
			if value.Name.Local == "p" {
				paragraphDepth++
			}
			if value.Name.Local == "t" {
				var text string
				if decoder.DecodeElement(&text, &value) == nil {
					current.WriteString(text)
				}
			}
		case xml.EndElement:
			if value.Name.Local == "p" && paragraphDepth > 0 {
				paragraphs = append(paragraphs, current.String())
				current.Reset()
				paragraphDepth--
			}
		}
	}
	content := make([]map[string]any, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		content = append(content, textNodeBlock("paragraph", paragraph, nil))
	}
	if len(content) == 0 {
		content = append(content, textNodeBlock("paragraph", "", nil))
	}
	return json.Marshal(map[string]any{"type": "doc", "content": content})
}

func textNodeBlock(kind, value string, attrs map[string]any) map[string]any {
	block := map[string]any{"type": kind}
	if attrs != nil {
		block["attrs"] = attrs
	}
	if value != "" {
		block["content"] = []map[string]any{{"type": "text", "text": value}}
	}
	return block
}

func (s *Server) uploadAttachment(w http.ResponseWriter, r *http.Request) {
	documentID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	role, err := s.documentRole(r.Context(), p.User, documentID, false)
	if err != nil || !requireDocumentRole(w, role, "EDITOR") {
		return
	}
	all, err := s.settings.GetAll(r.Context(), false)
	if err != nil {
		writeError(w, 500, "SETTINGS_ERROR", "업로드 설정을 불러오지 못했습니다.")
		return
	}
	maxBytes := int64(all.Security.MaxUploadMB) << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20))
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, 400, "FILE_REQUIRED", "첨부할 파일이 필요합니다.")
		return
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || int64(len(body)) > maxBytes {
		writeError(w, 413, "FILE_TOO_LARGE", fmt.Sprintf("파일은 %dMB 이하여야 합니다.", all.Security.MaxUploadMB))
		return
	}
	mediaType := http.DetectContentType(body)
	if mediaType == "application/octet-stream" {
		if declared, _, parseErr := mime.ParseMediaType(header.Header.Get("Content-Type")); parseErr == nil && declared != "" {
			mediaType = declared
		}
	}
	sum := sha256.Sum256(body)
	id := uuid.New()
	_, err = s.db.Exec(r.Context(), `INSERT INTO attachments(id,document_id,uploader_id,name,media_type,size_bytes,sha256,data) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, documentID, p.User.ID, truncateRunes(filepath.Base(header.Filename), 240), mediaType, len(body), hex.EncodeToString(sum[:]), body)
	if err != nil {
		writeError(w, 500, "ATTACHMENT_SAVE_FAILED", "첨부파일을 저장하지 못했습니다.")
		return
	}
	s.audit(r, &p.User.ID, "UPLOAD_ATTACHMENT", "DOCUMENT", &documentID, map[string]any{"attachmentId": id, "bytes": len(body), "mediaType": mediaType})
	writeData(w, 201, map[string]any{"id": id, "name": header.Filename, "mediaType": mediaType, "sizeBytes": len(body), "url": "/api/v1/attachments/" + id.String()})
}

func (s *Server) listAttachments(w http.ResponseWriter, r *http.Request) {
	documentID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	if _, err := s.documentRole(r.Context(), p.User, documentID, false); err != nil {
		writeError(w, 403, "DOCUMENT_PERMISSION_DENIED", "첨부파일을 볼 권한이 없습니다.")
		return
	}
	rows, err := s.db.Query(r.Context(), `SELECT id,name,media_type,size_bytes,sha256,created_at FROM attachments WHERE document_id=$1 ORDER BY created_at`, documentID)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "첨부파일을 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id uuid.UUID
		var name, mediaType, hash string
		var size int64
		var created time.Time
		if rows.Scan(&id, &name, &mediaType, &size, &hash, &created) == nil {
			items = append(items, map[string]any{"id": id, "name": name, "mediaType": mediaType, "sizeBytes": size, "sha256": hash, "createdAt": created, "url": "/api/v1/attachments/" + id.String()})
		}
	}
	writeData(w, 200, items)
}

func (s *Server) downloadAttachment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	var documentID uuid.UUID
	var name, mediaType, hash string
	var body []byte
	if s.db.QueryRow(r.Context(), `SELECT document_id,name,media_type,sha256,data FROM attachments WHERE id=$1`, id).Scan(&documentID, &name, &mediaType, &hash, &body) != nil {
		writeError(w, 404, "ATTACHMENT_NOT_FOUND", "첨부파일을 찾을 수 없습니다.")
		return
	}
	if _, err := s.documentRole(r.Context(), p.User, documentID, false); err != nil {
		writeError(w, 403, "DOCUMENT_PERMISSION_DENIED", "첨부파일을 볼 권한이 없습니다.")
		return
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	w.Header().Set("ETag", `"`+hash+`"`)
	disposition := "attachment"
	if safeInlineImageType(mediaType) {
		disposition = "inline"
	} else {
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="attachment"; filename*=UTF-8''%s`, disposition, urlPathEscape(name)))
	w.WriteHeader(200)
	_, _ = w.Write(body)
	s.audit(r, &p.User.ID, "DOWNLOAD_ATTACHMENT", "DOCUMENT", &documentID, map[string]any{"attachmentId": id})
}

func safeInlineImageType(value string) bool {
	base, _, err := mime.ParseMediaType(value)
	if err != nil {
		base = strings.ToLower(strings.TrimSpace(strings.SplitN(value, ";", 2)[0]))
	}
	switch strings.ToLower(base) {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func baseMediaType(value string) string {
	base, _, err := mime.ParseMediaType(value)
	if err == nil && base != "" {
		return strings.ToLower(base)
	}
	return strings.ToLower(strings.TrimSpace(strings.SplitN(value, ";", 2)[0]))
}

func (s *Server) deleteAttachment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	var documentID uuid.UUID
	if s.db.QueryRow(r.Context(), `SELECT document_id FROM attachments WHERE id=$1`, id).Scan(&documentID) != nil {
		writeError(w, 404, "ATTACHMENT_NOT_FOUND", "첨부파일을 찾을 수 없습니다.")
		return
	}
	role, err := s.documentRole(r.Context(), p.User, documentID, false)
	if err != nil || !requireDocumentRole(w, role, "EDITOR") {
		return
	}
	_, err = s.db.Exec(r.Context(), `DELETE FROM attachments WHERE id=$1`, id)
	if err != nil {
		writeError(w, 500, "ATTACHMENT_DELETE_FAILED", "첨부파일을 삭제하지 못했습니다.")
		return
	}
	s.audit(r, &p.User.ID, "DELETE_ATTACHMENT", "DOCUMENT", &documentID, map[string]any{"attachmentId": id})
	w.WriteHeader(204)
}
