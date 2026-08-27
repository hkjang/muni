package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/database"
	"github.com/hkjang/muni/internal/docx"
	"github.com/hkjang/muni/internal/hwpx"
	"github.com/hkjang/muni/internal/pdfx"
	"github.com/hkjang/muni/internal/richdoc"
	"github.com/jackc/pgx/v5"
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
	if extension == "" {
		extension = extensionFromMediaType(header.Header.Get("Content-Type"), body)
	}
	var content json.RawMessage
	var assets []richdoc.Asset
	documentID := uuid.New()
	embeddedTitle := ""
	// A .docx carries its classification on the page rather than in the text.
	var furniture docx.Meta
	switch extension {
	case ".txt":
		content, err = plainTextDocument(string(body))
	case ".md", ".markdown":
		content, assets, err = markdownDocument(string(body))
	case ".html", ".htm":
		content, assets, err = htmlDocument(body)
	case ".docx":
		content, assets, furniture, err = docxImport(body)
	case ".hwpx":
		content, assets, furniture, err = hwpxImport(body)
	case ".pdf":
		// PDF interpretation is CPU bound; bound it so one upload cannot hold
		// a worker for the whole request timeout.
		parseCtx, cancelParse := context.WithTimeout(r.Context(), 90*time.Second)
		content, assets, embeddedTitle, err = pdfImport(parseCtx, body)
		cancelParse()
	default:
		writeError(w, 400, "UNSUPPORTED_IMPORT", "지원 형식은 PDF, DOCX, HWPX, Markdown, TXT, HTML입니다.")
		return
	}
	if err != nil {
		writeError(w, 400, "IMPORT_PARSE_FAILED", "파일 내용을 읽지 못했습니다: "+err.Error())
		return
	}
	// Imported images become attachments so the editor can render them and
	// the export path can embed them again.
	attachments, content, err := prepareImportedAssets(assets, content)
	if err != nil {
		writeError(w, 400, "IMPORT_PARSE_FAILED", "가져온 문서를 변환하지 못했습니다: "+err.Error())
		return
	}
	// An imported document has never been through the editor, so stamp the
	// block identities that comments, citations and diffs anchor to.
	content, err = withBlockIDs(content)
	if err != nil {
		writeError(w, 400, "IMPORT_PARSE_FAILED", "가져온 문서를 변환하지 못했습니다: "+err.Error())
		return
	}
	if !validDocumentJSON(content) {
		writeError(w, 400, "IMPORT_TOO_LARGE", "가져온 문서가 너무 큽니다. 파일을 나눠서 가져와 주세요.")
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		title = strings.TrimSpace(embeddedTitle)
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(header.Filename), filepath.Ext(header.Filename))
	}
	if title == "" {
		title = "가져온 문서"
	}
	title = truncateRunes(title, 240)
	text := extractDocumentText(content)
	visibility := "RESTRICTED"
	if workspaceKind != "PERSONAL" {
		visibility = "WORKSPACE"
	}
	err = database.WithTx(r.Context(), s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(), `INSERT INTO documents(id,workspace_id,folder_id,owner_id,title,visibility,content_json,content_text,revision_no,page_header,page_footer,page_orientation) VALUES($1,$2,$3,$4,$5,$6,$7,$8,1,$9,$10,$11)`,
			documentID, workspaceID, folderID, p.User.ID, title, visibility, content, text,
			truncateRunes(furniture.Header, 200), truncateRunes(furniture.Footer, 200), orientationOf(furniture.Landscape)); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `INSERT INTO document_revisions(document_id,revision_no,content_json,content_text,author_id,reason) VALUES($1,1,$2,$3,$4,$5)`, documentID, content, text, p.User.ID, "import:"+strings.TrimPrefix(extension, ".")); err != nil {
			return err
		}
		for _, attachment := range attachments {
			sum := sha256.Sum256(attachment.Data)
			if _, err := tx.Exec(r.Context(), `INSERT INTO attachments(id,document_id,uploader_id,name,media_type,size_bytes,sha256,data) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
				attachment.ID, documentID, p.User.ID, truncateRunes(attachment.Name, 240), attachment.MediaType, len(attachment.Data), hex.EncodeToString(sum[:]), attachment.Data); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		writeError(w, 500, "IMPORT_FAILED", "가져온 문서를 저장하지 못했습니다.")
		return
	}
	s.audit(r, &p.User.ID, "IMPORT_DOCUMENT", "DOCUMENT", &documentID, map[string]any{"format": strings.TrimPrefix(extension, "."), "bytes": len(body), "images": len(attachments)})
	s.getDocumentByID(w, r, documentID)
}

// docxImport converts a Word file, keeping headings, lists, tables, inline
// formatting and embedded images.
func docxImport(body []byte) (json.RawMessage, []richdoc.Asset, docx.Meta, error) {
	document, assets, meta, err := docx.Parse(body)
	if err != nil {
		return nil, nil, docx.Meta{}, err
	}
	content, err := document.JSON()
	if err != nil {
		return nil, nil, docx.Meta{}, err
	}
	return content, assets, meta, nil
}

// hwpxImport converts a Hangul Office file, keeping headings, tables, inline
// formatting and embedded pictures.
//
// The furniture it reports is only the paper: HWPX keeps a header and footer
// per section in a shape muni has nowhere to put yet.
func hwpxImport(body []byte) (json.RawMessage, []richdoc.Asset, docx.Meta, error) {
	document, assets, meta, err := hwpx.Parse(body)
	if err != nil {
		return nil, nil, docx.Meta{}, err
	}
	content, err := document.JSON()
	if err != nil {
		return nil, nil, docx.Meta{}, err
	}
	return content, assets, docx.Meta{Landscape: meta.Landscape}, nil
}

// pdfImport reconstructs paragraphs, headings, lists, tables and images from
// a PDF's text layer.
func pdfImport(ctx context.Context, body []byte) (json.RawMessage, []richdoc.Asset, string, error) {
	result, err := pdfx.Import(ctx, body)
	if err != nil {
		return nil, nil, "", err
	}
	content, err := result.Document.JSON()
	if err != nil {
		return nil, nil, "", err
	}
	return content, result.Assets, result.Title, nil
}

type importedAttachment struct {
	ID        uuid.UUID
	Name      string
	MediaType string
	Data      []byte
}

// prepareImportedAssets allocates an attachment row per embedded image and
// rewrites the placeholder sources in the document JSON to point at it. Images
// in formats the editor cannot display are dropped along with their nodes.
func prepareImportedAssets(assets []richdoc.Asset, content json.RawMessage) ([]importedAttachment, json.RawMessage, error) {
	if len(assets) == 0 {
		return nil, content, nil
	}
	document, err := richdoc.Parse(content)
	if err != nil {
		return nil, nil, err
	}
	sources := map[string]string{}
	out := make([]importedAttachment, 0, len(assets))
	for index, asset := range assets {
		if len(asset.Data) == 0 || !safeInlineImageType(asset.MediaType) {
			continue
		}
		id := uuid.New()
		name := strings.TrimSpace(asset.Name)
		if name == "" {
			name = fmt.Sprintf("image-%d", index+1)
		}
		out = append(out, importedAttachment{ID: id, Name: name, MediaType: baseMediaType(asset.MediaType), Data: asset.Data})
		sources[asset.Placeholder] = "/api/v1/attachments/" + id.String()
	}
	rewriteImageSources(document, sources)
	rewritten, err := document.JSON()
	if err != nil {
		return nil, nil, err
	}
	return out, rewritten, nil
}

// rewriteImageSources points image nodes at their stored attachment and
// removes the ones whose bytes could not be kept.
func rewriteImageSources(node *richdoc.Node, sources map[string]string) {
	if node == nil {
		return
	}
	kept := node.Content[:0]
	for _, child := range node.Content {
		if child == nil {
			continue
		}
		if child.Type == "image" {
			source := child.AttrString("src")
			if strings.HasPrefix(source, richdoc.AssetPlaceholderPrefix) {
				replacement, ok := sources[source]
				if !ok {
					continue
				}
				child.SetAttr("src", replacement)
			}
		}
		rewriteImageSources(child, sources)
		kept = append(kept, child)
	}
	node.Content = kept
}

func extensionFromMediaType(mediaType string, body []byte) string {
	base, _, err := mime.ParseMediaType(mediaType)
	if err != nil {
		base = strings.ToLower(strings.TrimSpace(strings.SplitN(mediaType, ";", 2)[0]))
	}
	switch strings.ToLower(base) {
	case "application/pdf":
		return ".pdf"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return ".docx"
	case "application/hwp+zip", "application/vnd.hancom.hwpx", "application/haansofthwpx":
		return ".hwpx"
	case "text/markdown":
		return ".md"
	case "text/html":
		return ".html"
	case "text/plain":
		return ".txt"
	}
	switch {
	case bytes.HasPrefix(body, []byte("%PDF-")):
		return ".pdf"
	case bytes.HasPrefix(body, []byte("PK\x03\x04")):
		// Both formats are zips. Which one it is is written inside it.
		return zipKind(body)
	}
	return ""
}

// zipKind tells a Hangul file from a Word one by what the archive holds.
//
// An upload that arrives without a usable name or media type is common enough
// — a browser that guesses application/octet-stream, a proxy that strips the
// name — and calling every zip a .docx made a .hwpx fail with an error about
// word/document.xml being missing, which is true and useless.
func zipKind(body []byte) string {
	archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return ".docx"
	}
	for _, file := range archive.File {
		name := strings.ToLower(strings.TrimPrefix(file.Name, "/"))
		switch {
		case strings.HasPrefix(name, "contents/section"), name == "contents/content.hpf":
			return ".hwpx"
		case name == "word/document.xml":
			return ".docx"
		}
	}
	return ".docx"
}

func (s *Server) uploadAttachment(w http.ResponseWriter, r *http.Request) {
	documentID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	role, err := s.documentRole(r.Context(), p.User, documentID, false)
	if !documentAllowed(w, role, err, "EDITOR") {
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
	role, err := s.documentRole(r.Context(), p.User, documentID, false)
	if !documentAllowed(w, role, err, "VIEWER") {
		return
	}
	// in_use answers the question that makes this list worth having: which of
	// these is still in the document, and which is only taking up space. An
	// image dragged in and then deleted from the text leaves its bytes behind
	// with nothing pointing at them, and until now nothing said so — or let
	// anyone clear it. The content references an attachment by a URL that
	// carries its id, so the id appearing in the stored document is the test.
	rows, err := s.db.Query(r.Context(), `
		SELECT a.id, a.name, a.media_type, a.size_bytes, a.sha256, a.created_at,
			coalesce(u.display_name, '') ,
			d.content_json::text LIKE '%' || a.id::text || '%'
		FROM attachments a
		JOIN documents d ON d.id = a.document_id
		LEFT JOIN users u ON u.id = a.uploader_id
		WHERE a.document_id = $1 ORDER BY a.created_at`, documentID)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "첨부파일을 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id uuid.UUID
		var name, mediaType, hash, uploader string
		var size int64
		var created time.Time
		var inUse bool
		if rows.Scan(&id, &name, &mediaType, &size, &hash, &created, &uploader, &inUse) == nil {
			items = append(items, map[string]any{
				"id": id, "name": name, "mediaType": mediaType, "sizeBytes": size,
				"sha256": hash, "createdAt": created, "uploadedBy": uploader,
				"inUse": inUse, "url": "/api/v1/attachments/" + id.String(),
			})
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
	if !documentAllowed(w, role, err, "EDITOR") {
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

// withBlockIDs stamps stable block identities onto document content that did
// not come from the editor.
func withBlockIDs(content json.RawMessage) (json.RawMessage, error) {
	document, err := richdoc.Parse(content)
	if err != nil {
		return nil, err
	}
	if richdoc.AssignBlockIDs(document, time.Now().UTC()) == 0 {
		return content, nil
	}
	return document.JSON()
}

// orientationOf names the page the way the column stores it.
func orientationOf(landscape bool) string {
	if landscape {
		return "LANDSCAPE"
	}
	return "PORTRAIT"
}
