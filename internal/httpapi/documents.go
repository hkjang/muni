package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/database"
	"github.com/jackc/pgx/v5"
)

type documentItem struct {
	ID             uuid.UUID       `json:"id"`
	WorkspaceID    uuid.UUID       `json:"workspaceId"`
	FolderID       *uuid.UUID      `json:"folderId,omitempty"`
	OwnerID        uuid.UUID       `json:"ownerId"`
	OwnerName      string          `json:"ownerName"`
	Title          string          `json:"title"`
	Status         string          `json:"status"`
	Visibility     string          `json:"visibility"`
	WorkflowStatus string          `json:"workflowStatus"`
	Content        json.RawMessage `json:"content"`
	Revision       int             `json:"revision"`
	Favorite       bool            `json:"favorite"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
	DeletedAt      *time.Time      `json:"deletedAt,omitempty"`
	Permission     string          `json:"permission"`
}

func documentSelect() string {
	return `SELECT d.id,d.workspace_id,d.folder_id,d.owner_id,u.display_name,d.title,d.status,d.visibility,d.workflow_status,d.content_json,d.revision_no,
	EXISTS(SELECT 1 FROM favorites f WHERE f.document_id=d.id AND f.user_id=$2),d.created_at,d.updated_at,d.deleted_at FROM documents d JOIN users u ON u.id=d.owner_id`
}

func scanDocument(row pgx.Row, target *documentItem) error {
	return row.Scan(&target.ID, &target.WorkspaceID, &target.FolderID, &target.OwnerID, &target.OwnerName, &target.Title, &target.Status, &target.Visibility, &target.WorkflowStatus, &target.Content, &target.Revision, &target.Favorite, &target.CreatedAt, &target.UpdatedAt, &target.DeletedAt)
}

func (s *Server) createDocument(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	var input struct {
		WorkspaceID uuid.UUID       `json:"workspaceId"`
		FolderID    *uuid.UUID      `json:"folderId"`
		Title       string          `json:"title"`
		Content     json.RawMessage `json:"content"`
		TemplateID  *uuid.UUID      `json:"templateId"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.WorkspaceID == uuid.Nil {
		writeError(w, 400, "WORKSPACE_REQUIRED", "워크스페이스가 필요합니다.")
		return
	}
	var memberRole, workspaceKind string
	if s.db.QueryRow(r.Context(), `SELECT wm.role,w.kind FROM workspace_members wm JOIN workspaces w ON w.id=wm.workspace_id WHERE wm.workspace_id=$1 AND wm.user_id=$2`, input.WorkspaceID, p.User.ID).Scan(&memberRole, &workspaceKind) != nil || memberRole == "VIEWER" {
		writeError(w, 403, "WORKSPACE_PERMISSION_DENIED", "문서를 만들 권한이 없습니다.")
		return
	}
	if input.FolderID != nil {
		var validFolder bool
		_ = s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM folders WHERE id=$1 AND workspace_id=$2 AND deleted_at IS NULL)`, input.FolderID, input.WorkspaceID).Scan(&validFolder)
		if !validFolder {
			writeError(w, 400, "INVALID_FOLDER", "워크스페이스에 속한 폴더를 선택해 주세요.")
			return
		}
	}
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		input.Title = "제목 없는 문서"
	}
	if len([]rune(input.Title)) > 240 {
		writeError(w, 400, "INVALID_TITLE", "제목은 240자 이하여야 합니다.")
		return
	}
	if len(input.Content) == 0 {
		input.Content = json.RawMessage(`{"type":"doc","content":[{"type":"paragraph"}]}`)
	}
	if !validDocumentJSON(input.Content) {
		writeError(w, 400, "INVALID_CONTENT", "문서 내용 형식이 올바르지 않습니다.")
		return
	}
	text := extractDocumentText(input.Content)
	id := uuid.New()
	visibility := "RESTRICTED"
	if workspaceKind != "PERSONAL" {
		visibility = "WORKSPACE"
	}
	err := database.WithTx(r.Context(), s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(), `INSERT INTO documents(id,workspace_id,folder_id,owner_id,title,visibility,content_json,content_text,revision_no,workflow_status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,1,'NONE')`, id, input.WorkspaceID, input.FolderID, p.User.ID, input.Title, visibility, input.Content, text); err != nil {
			return err
		}
		_, err := tx.Exec(r.Context(), `INSERT INTO document_revisions(document_id,revision_no,content_json,content_text,author_id,reason) VALUES($1,1,$2,$3,$4,'create')`, id, input.Content, text, p.User.ID)
		return err
	})
	if err != nil {
		writeError(w, 400, "DOCUMENT_CREATE_FAILED", "문서를 만들지 못했습니다.")
		return
	}
	s.audit(r, &p.User.ID, "CREATE_DOCUMENT", "DOCUMENT", &id, nil)
	s.getDocumentByID(w, r, id)
}

func (s *Server) getDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	s.getDocumentByID(w, r, id)
}
func (s *Server) getDocumentByID(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	p, _ := principalFrom(r.Context())
	role, err := s.documentRole(r.Context(), p.User, id, false)
	if err != nil {
		if errors.Is(err, errForbidden) {
			writeError(w, 403, "DOCUMENT_PERMISSION_DENIED", "문서에 접근할 권한이 없습니다.")
		} else {
			writeError(w, 404, "DOCUMENT_NOT_FOUND", "문서를 찾을 수 없습니다.")
		}
		return
	}
	var d documentItem
	err = scanDocument(s.db.QueryRow(r.Context(), documentSelect()+` WHERE d.id=$1`, id, p.User.ID), &d)
	if err != nil {
		writeError(w, 404, "DOCUMENT_NOT_FOUND", "문서를 찾을 수 없습니다.")
		return
	}
	d.Permission = role
	all, _ := s.settings.GetAll(r.Context(), false)
	if all.Security.AuditReads {
		s.audit(r, &p.User.ID, "READ_DOCUMENT", "DOCUMENT", &id, nil)
	}
	writeData(w, 200, d)
}

func (s *Server) listDocuments(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	var member bool
	_ = s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM workspace_members WHERE workspace_id=$1 AND user_id=$2)`, workspaceID, p.User.ID).Scan(&member)
	if !member && p.User.Role != "ADMIN" {
		writeError(w, 403, "WORKSPACE_PERMISSION_DENIED", "워크스페이스에 접근할 권한이 없습니다.")
		return
	}
	includeTrash := r.URL.Query().Get("trash") == "true"
	folderID := r.URL.Query().Get("folderId")
	status := strings.ToUpper(r.URL.Query().Get("status"))
	all, _ := s.settings.GetAll(r.Context(), false)
	limit := parseLimit(r.URL.Query().Get("limit"), all.General.PageSize)
	rows, err := s.db.Query(r.Context(), documentSelect()+` WHERE d.workspace_id=$1 AND (($3::bool AND d.deleted_at IS NOT NULL) OR (NOT $3::bool AND d.deleted_at IS NULL)) AND ($4='' OR d.folder_id=$4::uuid) AND ($5='' OR d.status=$5)
	AND (d.owner_id=$2 OR d.visibility IN ('WORKSPACE','ORGANIZATION') OR EXISTS(SELECT 1 FROM document_permissions dp WHERE dp.document_id=d.id AND dp.subject_type='USER' AND dp.subject_id=$2 AND (dp.expires_at IS NULL OR dp.expires_at>now())) OR $6='ADMIN') ORDER BY d.updated_at DESC LIMIT $7`, workspaceID, p.User.ID, includeTrash, folderID, status, p.User.Role, limit)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "문서 목록을 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]documentItem, 0)
	for rows.Next() {
		var d documentItem
		if scanDocument(rows, &d) == nil {
			role, _ := s.documentRole(r.Context(), p.User, d.ID, true)
			d.Permission = role
			d.Content = nil
			items = append(items, d)
		}
	}
	writeData(w, 200, items)
}

func (s *Server) listUserDocuments(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	scope := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("scope")))
	if scope == "" {
		scope = "recent"
	}
	if !contains([]string{"recent", "favorites", "shared", "owned", "trash"}, scope) {
		writeError(w, 400, "INVALID_DOCUMENT_SCOPE", "문서 목록 범위를 확인해 주세요.")
		return
	}
	all, _ := s.settings.GetAll(r.Context(), false)
	limit := parseLimit(r.URL.Query().Get("limit"), all.General.PageSize)
	rows, err := s.db.Query(r.Context(), documentSelect()+` WHERE
		(( $1='trash' AND d.deleted_at IS NOT NULL AND (d.owner_id=$2 OR $3='ADMIN')) OR
		 ( $1<>'trash' AND d.deleted_at IS NULL AND
		  (d.owner_id=$2 OR d.visibility='ORGANIZATION' OR
		   EXISTS(SELECT 1 FROM workspace_members wm WHERE wm.workspace_id=d.workspace_id AND wm.user_id=$2 AND d.visibility='WORKSPACE') OR
		   EXISTS(SELECT 1 FROM document_permissions dp WHERE dp.document_id=d.id AND dp.subject_type='USER' AND dp.subject_id=$2 AND (dp.expires_at IS NULL OR dp.expires_at>now())) OR $3='ADMIN')))
		AND ($1 IN ('recent','trash') OR ($1='owned' AND d.owner_id=$2) OR
		 ($1='favorites' AND EXISTS(SELECT 1 FROM favorites f WHERE f.document_id=d.id AND f.user_id=$2)) OR
		 ($1='shared' AND d.owner_id<>$2 AND EXISTS(SELECT 1 FROM document_permissions dp WHERE dp.document_id=d.id AND dp.subject_type='USER' AND dp.subject_id=$2 AND (dp.expires_at IS NULL OR dp.expires_at>now()))))
		ORDER BY d.updated_at DESC LIMIT $4`, scope, p.User.ID, p.User.Role, limit)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "문서 목록을 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]documentItem, 0)
	for rows.Next() {
		var d documentItem
		if scanDocument(rows, &d) != nil {
			continue
		}
		role, roleErr := s.documentRole(r.Context(), p.User, d.ID, scope == "trash")
		if roleErr != nil {
			continue
		}
		d.Permission = role
		d.Content = nil
		items = append(items, d)
	}
	writeData(w, 200, items)
}

func (s *Server) updateDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	role, err := s.documentRole(r.Context(), p.User, id, false)
	if err != nil || !requireDocumentRole(w, role, "EDITOR") {
		return
	}
	var input struct {
		Title            *string         `json:"title"`
		Content          json.RawMessage `json:"content"`
		ExpectedRevision int             `json:"expectedRevision"`
		Status           *string         `json:"status"`
		Visibility       *string         `json:"visibility"`
		Reason           string          `json:"reason"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Title != nil {
		v := strings.TrimSpace(*input.Title)
		if v == "" || len([]rune(v)) > 240 {
			writeError(w, 400, "INVALID_TITLE", "제목은 1~240자여야 합니다.")
			return
		}
		input.Title = &v
	}
	if len(input.Content) > 0 && !validDocumentJSON(input.Content) {
		writeError(w, 400, "INVALID_CONTENT", "문서 내용 형식이 올바르지 않습니다.")
		return
	}
	if input.Status != nil && !contains([]string{"DRAFT", "REVIEW", "PUBLISHED", "ARCHIVED"}, *input.Status) {
		writeError(w, 400, "INVALID_STATUS", "문서 상태가 올바르지 않습니다.")
		return
	}
	if input.Visibility != nil && !contains([]string{"RESTRICTED", "WORKSPACE", "ORGANIZATION", "LINK"}, *input.Visibility) {
		writeError(w, 400, "INVALID_VISIBILITY", "공유 범위가 올바르지 않습니다.")
		return
	}
	all, _ := s.settings.GetAll(r.Context(), false)
	if input.Visibility != nil && *input.Visibility == "LINK" && !all.Security.AllowPublicLinks {
		writeError(w, 403, "PUBLIC_LINK_DISABLED", "관리자 정책에서 링크 공유가 비활성화되어 있습니다.")
		return
	}
	if input.Reason == "" {
		input.Reason = "autosave"
	}
	err = database.WithTx(r.Context(), s.db, func(tx pgx.Tx) error {
		var currentTitle, currentStatus, currentVisibility, currentWorkflow string
		var currentContent json.RawMessage
		var revision int
		if err := tx.QueryRow(r.Context(), `SELECT title,status,visibility,workflow_status,content_json,revision_no FROM documents WHERE id=$1 FOR UPDATE`, id).Scan(&currentTitle, &currentStatus, &currentVisibility, &currentWorkflow, &currentContent, &revision); err != nil {
			return err
		}
		if currentWorkflow == "PENDING" {
			return workflowConflict("승인 대기 중인 문서는 검토가 끝날 때까지 변경할 수 없습니다.")
		}
		if input.ExpectedRevision > 0 && revision != input.ExpectedRevision {
			return revisionConflict{Current: revision}
		}
		if input.Title != nil {
			currentTitle = *input.Title
		}
		if input.Status != nil {
			currentStatus = *input.Status
		}
		if input.Visibility != nil {
			currentVisibility = *input.Visibility
		}
		contentChanged := len(input.Content) > 0 && string(input.Content) != string(currentContent)
		if len(input.Content) > 0 {
			currentContent = input.Content
		}
		newRevision := revision
		if contentChanged {
			newRevision++
		}
		text := extractDocumentText(currentContent)
		if _, err := tx.Exec(r.Context(), `UPDATE documents SET title=$2,status=$3,visibility=$4,content_json=$5,content_text=$6,revision_no=$7,updated_at=now() WHERE id=$1`, id, currentTitle, currentStatus, currentVisibility, currentContent, text, newRevision); err != nil {
			return err
		}
		if contentChanged {
			_, err := tx.Exec(r.Context(), `INSERT INTO document_revisions(document_id,revision_no,content_json,content_text,author_id,reason) VALUES($1,$2,$3,$4,$5,$6)`, id, newRevision, currentContent, text, p.User.ID, truncate(input.Reason, 100))
			return err
		}
		return nil
	})
	var conflict revisionConflict
	if errors.As(err, &conflict) {
		writeJSON(w, 409, map[string]any{"error": apiError{Code: "REVISION_CONFLICT", Message: "다른 사용자가 문서를 변경했습니다.", Details: map[string]any{"currentRevision": conflict.Current}}})
		return
	}
	var workflowErr workflowConflict
	if errors.As(err, &workflowErr) {
		writeError(w, 409, "DOCUMENT_PENDING_APPROVAL", workflowErr.Error())
		return
	}
	if err != nil {
		writeError(w, 500, "DOCUMENT_SAVE_FAILED", "문서를 저장하지 못했습니다.")
		return
	}
	s.audit(r, &p.User.ID, "UPDATE_DOCUMENT", "DOCUMENT", &id, map[string]any{"reason": input.Reason})
	s.getDocumentByID(w, r, id)
}

type revisionConflict struct{ Current int }

func (e revisionConflict) Error() string { return fmt.Sprintf("revision conflict: %d", e.Current) }

func (s *Server) deleteDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	role, err := s.documentRole(r.Context(), p.User, id, false)
	if err != nil || !requireDocumentRole(w, role, "OWNER") {
		return
	}
	result, err := s.db.Exec(r.Context(), `UPDATE documents SET deleted_at=now(),updated_at=now() WHERE id=$1 AND deleted_at IS NULL AND workflow_status<>'PENDING'`, id)
	if err != nil {
		writeError(w, 500, "DELETE_FAILED", "문서를 휴지통으로 이동하지 못했습니다.")
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, 409, "DOCUMENT_PENDING_APPROVAL", "승인 대기 중인 문서는 휴지통으로 이동할 수 없습니다.")
		return
	}
	s.hub.CloseDocument(id)
	s.audit(r, &p.User.ID, "DELETE_DOCUMENT", "DOCUMENT", &id, nil)
	w.WriteHeader(204)
}
func (s *Server) restoreDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	role, err := s.documentRole(r.Context(), p.User, id, true)
	if err != nil || !requireDocumentRole(w, role, "OWNER") {
		return
	}
	_, err = s.db.Exec(r.Context(), `UPDATE documents SET deleted_at=NULL,updated_at=now() WHERE id=$1`, id)
	if err != nil {
		writeError(w, 500, "RESTORE_FAILED", "문서를 복구하지 못했습니다.")
		return
	}
	s.audit(r, &p.User.ID, "RESTORE_DOCUMENT", "DOCUMENT", &id, nil)
	s.getDocumentByID(w, r, id)
}
func (s *Server) favoriteDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	if _, err := s.documentRole(r.Context(), p.User, id, false); err != nil {
		writeError(w, 403, "DOCUMENT_PERMISSION_DENIED", "문서에 접근할 권한이 없습니다.")
		return
	}
	_, _ = s.db.Exec(r.Context(), `INSERT INTO favorites(user_id,document_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, p.User.ID, id)
	w.WriteHeader(204)
}
func (s *Server) unfavoriteDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	_, _ = s.db.Exec(r.Context(), `DELETE FROM favorites WHERE user_id=$1 AND document_id=$2`, p.User.ID, id)
	w.WriteHeader(204)
}

func (s *Server) listRevisions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	if _, err := s.documentRole(r.Context(), p.User, id, false); err != nil {
		writeError(w, 403, "DOCUMENT_PERMISSION_DENIED", "문서에 접근할 권한이 없습니다.")
		return
	}
	rows, err := s.db.Query(r.Context(), `SELECT dr.id,dr.revision_no,dr.reason,dr.name,dr.created_at,u.id,u.display_name FROM document_revisions dr JOIN users u ON u.id=dr.author_id WHERE dr.document_id=$1 ORDER BY dr.revision_no DESC LIMIT 200`, id)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "버전 기록을 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var rid, uid uuid.UUID
		var revision int
		var reason, name *string
		var created time.Time
		var author string
		if rows.Scan(&rid, &revision, &reason, &name, &created, &uid, &author) == nil {
			items = append(items, map[string]any{"id": rid, "revision": revision, "reason": reason, "name": name, "createdAt": created, "author": map[string]any{"id": uid, "displayName": author}})
		}
	}
	writeData(w, 200, items)
}
func (s *Server) restoreRevision(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	revision, err := strconv.Atoi(r.PathValue("revision"))
	if err != nil {
		writeError(w, 400, "INVALID_REVISION", "버전 번호가 올바르지 않습니다.")
		return
	}
	p, _ := principalFrom(r.Context())
	role, err := s.documentRole(r.Context(), p.User, id, false)
	if err != nil || !requireDocumentRole(w, role, "EDITOR") {
		return
	}
	err = database.WithTx(r.Context(), s.db, func(tx pgx.Tx) error {
		var content json.RawMessage
		var text string
		if err := tx.QueryRow(r.Context(), `SELECT content_json,content_text FROM document_revisions WHERE document_id=$1 AND revision_no=$2`, id, revision).Scan(&content, &text); err != nil {
			return err
		}
		var next int
		if err := tx.QueryRow(r.Context(), `UPDATE documents SET content_json=$2,content_text=$3,revision_no=revision_no+1,crdt_generation=crdt_generation+1,updated_at=now() WHERE id=$1 RETURNING revision_no`, id, content, text).Scan(&next); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `DELETE FROM collab_updates WHERE document_id=$1`, id); err != nil {
			return err
		}
		_, err := tx.Exec(r.Context(), `INSERT INTO document_revisions(document_id,revision_no,content_json,content_text,author_id,reason) VALUES($1,$2,$3,$4,$5,$6)`, id, next, content, text, p.User.ID, fmt.Sprintf("restore:%d", revision))
		return err
	})
	if err != nil {
		writeError(w, 404, "REVISION_NOT_FOUND", "복원할 버전을 찾을 수 없습니다.")
		return
	}
	s.audit(r, &p.User.ID, "RESTORE_REVISION", "DOCUMENT", &id, map[string]any{"revision": revision})
	s.getDocumentByID(w, r, id)
}

func validDocumentJSON(raw json.RawMessage) bool {
	var value struct {
		Type    string `json:"type"`
		Content []any  `json:"content"`
	}
	return len(raw) <= 10<<20 && json.Unmarshal(raw, &value) == nil && value.Type == "doc"
}
func extractDocumentText(raw json.RawMessage) string {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	parts := make([]string, 0)
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if text, ok := x["text"].(string); ok {
				parts = append(parts, text)
			}
			if c, ok := x["content"]; ok {
				walk(c)
			}
		case []any:
			for _, item := range x {
				walk(item)
			}
		}
	}
	walk(value)
	return strings.Join(parts, " ")
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func parseLimit(value string, fallback int) int {
	n, _ := strconv.Atoi(value)
	if n < 1 {
		return fallback
	}
	if n > 100 {
		return 100
	}
	return n
}

func (s *Server) searchDocuments(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeData(w, 200, []any{})
		return
	}
	all, _ := s.settings.GetAll(r.Context(), false)
	limit := parseLimit(r.URL.Query().Get("limit"), all.General.PageSize)
	rows, err := s.db.Query(r.Context(), `SELECT d.id,d.workspace_id,d.title,d.status,d.updated_at,u.display_name,ts_headline('simple',d.content_text,websearch_to_tsquery('simple',$1),'MaxWords=24,MinWords=8') FROM documents d JOIN users u ON u.id=d.owner_id WHERE d.deleted_at IS NULL
	AND (to_tsvector('simple',coalesce(d.title,'')||' '||coalesce(d.content_text,''))@@websearch_to_tsquery('simple',$1)
		OR d.title ILIKE '%'||$1||'%' OR d.content_text ILIKE '%'||$1||'%' OR u.display_name ILIKE '%'||$1||'%'
		OR EXISTS(SELECT 1 FROM comments c WHERE c.document_id=d.id AND c.deleted_at IS NULL AND c.body ILIKE '%'||$1||'%')
		OR EXISTS(SELECT 1 FROM attachments a WHERE a.document_id=d.id AND a.name ILIKE '%'||$1||'%')
		OR EXISTS(SELECT 1 FROM document_tags dt JOIN tags t ON t.id=dt.tag_id WHERE dt.document_id=d.id AND t.name ILIKE '%'||$1||'%'))
	AND (d.owner_id=$2 OR d.visibility='ORGANIZATION' OR EXISTS(SELECT 1 FROM workspace_members wm WHERE wm.workspace_id=d.workspace_id AND wm.user_id=$2 AND d.visibility='WORKSPACE') OR EXISTS(SELECT 1 FROM document_permissions dp WHERE dp.document_id=d.id AND dp.subject_type='USER' AND dp.subject_id=$2 AND (dp.expires_at IS NULL OR dp.expires_at>now())) OR $3='ADMIN') ORDER BY ts_rank(to_tsvector('simple',coalesce(d.title,'')||' '||coalesce(d.content_text,'')),websearch_to_tsquery('simple',$1)) DESC,d.updated_at DESC LIMIT $4`, q, p.User.ID, p.User.Role, limit)
	if err != nil {
		writeError(w, 500, "SEARCH_FAILED", "검색하지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, wid uuid.UUID
		var title, status, owner, snippet string
		var updated time.Time
		if rows.Scan(&id, &wid, &title, &status, &updated, &owner, &snippet) == nil {
			items = append(items, map[string]any{"id": id, "workspaceId": wid, "title": title, "status": status, "updatedAt": updated, "ownerName": owner, "snippet": snippet})
		}
	}
	s.audit(r, &p.User.ID, "SEARCH_DOCUMENT", "SEARCH", nil, map[string]any{"queryLength": len([]rune(q))})
	writeData(w, 200, items)
}
