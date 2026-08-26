package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Templates are the starting points an organisation writes once — a report
// form, a meeting record, a proposal — so nobody rebuilds the same headings
// from a blank page every time.
//
// The table has existed from the start and there was no way to put anything in
// it: no endpoint to create one, none to list one, and the templateId a new
// document accepted was read and then ignored.

type templateItem struct {
	ID          uuid.UUID       `json:"id"`
	WorkspaceID *uuid.UUID      `json:"workspaceId,omitempty"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Content     json.RawMessage `json:"content,omitempty"`
	CreatedBy   uuid.UUID       `json:"createdBy"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

// listTemplates returns what this workspace can start from: its own templates
// and the ones shared across the whole service.
func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	if !s.workspaceMember(r.Context(), workspaceID, p.User) {
		writeError(w, 403, "WORKSPACE_PERMISSION_DENIED", "워크스페이스에 접근할 권한이 없습니다.")
		return
	}
	// Whether the body is included is asked for, because a picker needs the
	// names and only the document being created needs the content.
	withContent := r.URL.Query().Get("content") == "true"

	rows, err := s.db.Query(r.Context(), `
		SELECT id, workspace_id, name, description, content_json, created_by, created_at, updated_at
		FROM templates
		WHERE workspace_id = $1 OR workspace_id IS NULL
		ORDER BY workspace_id NULLS LAST, name`, workspaceID)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "서식을 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]templateItem, 0, 16)
	for rows.Next() {
		var item templateItem
		if rows.Scan(&item.ID, &item.WorkspaceID, &item.Name, &item.Description,
			&item.Content, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt) == nil {
			if !withContent {
				item.Content = nil
			}
			items = append(items, item)
		}
	}
	writeData(w, 200, items)
}

// createTemplate saves a starting point, either from a document or from
// content sent directly.
func (s *Server) createTemplate(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	if !s.workspaceManager(r.Context(), workspaceID, p.User) {
		writeError(w, 403, "WORKSPACE_PERMISSION_DENIED", "서식을 만들 권한이 없습니다.")
		return
	}
	var input struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Content     json.RawMessage `json:"content"`
		// FromDocument copies the current content of a document the person can
		// already read, which is how a template usually starts life.
		FromDocument *uuid.UUID `json:"fromDocumentId"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len([]rune(input.Name)) > 120 {
		writeError(w, 400, "INVALID_NAME", "서식 이름은 1~120자여야 합니다.")
		return
	}
	if len([]rune(input.Description)) > 500 {
		writeError(w, 400, "INVALID_DESCRIPTION", "설명은 500자 이하여야 합니다.")
		return
	}

	if input.FromDocument != nil {
		role, err := s.documentRole(r.Context(), p.User, *input.FromDocument, false)
		if err != nil || roleRank[role] < roleRank["VIEWER"] {
			writeError(w, 403, "DOCUMENT_PERMISSION_DENIED", "문서를 읽을 권한이 없습니다.")
			return
		}
		if err := s.db.QueryRow(r.Context(), `SELECT content_json FROM documents WHERE id=$1`, *input.FromDocument).Scan(&input.Content); err != nil {
			writeError(w, 404, "DOCUMENT_NOT_FOUND", "문서를 찾을 수 없습니다.")
			return
		}
	}
	if !validDocumentJSON(input.Content) {
		writeError(w, 400, "INVALID_CONTENT", "서식 내용 형식이 올바르지 않습니다.")
		return
	}

	id := uuid.New()
	if _, err := s.db.Exec(r.Context(),
		`INSERT INTO templates(id,workspace_id,name,description,content_json,created_by)
		 VALUES($1,$2,$3,$4,$5,$6)`,
		id, workspaceID, input.Name, strings.TrimSpace(input.Description), input.Content, p.User.ID); err != nil {
		writeError(w, 500, "DATABASE_ERROR", "서식을 저장하지 못했습니다.")
		return
	}
	s.audit(r, &p.User.ID, "CREATE_TEMPLATE", "WORKSPACE", &workspaceID, map[string]any{"template": id, "name": input.Name})
	writeData(w, 201, map[string]any{"id": id, "name": input.Name})
}

// updateTemplate renames a template or replaces what it starts from.
func (s *Server) updateTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	workspaceID, ok := s.templateWorkspace(w, r, id, p)
	if !ok {
		return
	}
	var input struct {
		Name        *string         `json:"name"`
		Description *string         `json:"description"`
		Content     json.RawMessage `json:"content"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Name != nil {
		trimmed := strings.TrimSpace(*input.Name)
		if trimmed == "" || len([]rune(trimmed)) > 120 {
			writeError(w, 400, "INVALID_NAME", "서식 이름은 1~120자여야 합니다.")
			return
		}
		input.Name = &trimmed
	}
	if len(input.Content) > 0 && !validDocumentJSON(input.Content) {
		writeError(w, 400, "INVALID_CONTENT", "서식 내용 형식이 올바르지 않습니다.")
		return
	}
	var content any
	if len(input.Content) > 0 {
		content = input.Content
	}
	result, err := s.db.Exec(r.Context(),
		`UPDATE templates SET name=COALESCE($2,name), description=COALESCE($3,description),
		 content_json=COALESCE($4,content_json), updated_at=now() WHERE id=$1`,
		id, input.Name, input.Description, content)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, 404, "TEMPLATE_NOT_FOUND", "서식을 찾을 수 없습니다.")
		return
	}
	s.audit(r, &p.User.ID, "UPDATE_TEMPLATE", "WORKSPACE", workspaceID, map[string]any{"template": id})
	writeData(w, 200, map[string]any{"id": id})
}

// deleteTemplate removes a starting point. Documents already made from it are
// untouched: a template is copied, not referenced.
func (s *Server) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	workspaceID, ok := s.templateWorkspace(w, r, id, p)
	if !ok {
		return
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM templates WHERE id=$1`, id); err != nil {
		writeError(w, 500, "DATABASE_ERROR", "서식을 삭제하지 못했습니다.")
		return
	}
	s.audit(r, &p.User.ID, "DELETE_TEMPLATE", "WORKSPACE", workspaceID, map[string]any{"template": id})
	w.WriteHeader(204)
}

// templateWorkspace finds the template and checks who may change it. A
// template shared across the whole service belongs to the administrators.
func (s *Server) templateWorkspace(w http.ResponseWriter, r *http.Request, id uuid.UUID, p principal) (*uuid.UUID, bool) {
	var workspaceID *uuid.UUID
	if err := s.db.QueryRow(r.Context(), `SELECT workspace_id FROM templates WHERE id=$1`, id).Scan(&workspaceID); err != nil {
		writeError(w, 404, "TEMPLATE_NOT_FOUND", "서식을 찾을 수 없습니다.")
		return nil, false
	}
	if workspaceID == nil {
		if p.User.Role != "ADMIN" {
			writeError(w, 403, "TEMPLATE_PERMISSION_DENIED", "공용 서식은 관리자만 바꿀 수 있습니다.")
			return nil, false
		}
		return nil, true
	}
	if !s.workspaceManager(r.Context(), *workspaceID, p.User) {
		writeError(w, 403, "WORKSPACE_PERMISSION_DENIED", "서식을 바꿀 권한이 없습니다.")
		return nil, false
	}
	return workspaceID, true
}

// workspaceMember reports whether this person can see a workspace's contents.
// An administrator can see every workspace, which is the same rule the rest of
// the service applies.
func (s *Server) workspaceMember(ctx context.Context, workspaceID uuid.UUID, user User) bool {
	if user.Role == "ADMIN" {
		return true
	}
	var member bool
	_ = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workspace_members WHERE workspace_id=$1 AND user_id=$2)`,
		workspaceID, user.ID).Scan(&member)
	return member
}

// workspaceManager reports whether this person can change how a workspace is
// set up, which is what deciding its templates amounts to.
func (s *Server) workspaceManager(ctx context.Context, workspaceID uuid.UUID, user User) bool {
	if user.Role == "ADMIN" {
		return true
	}
	var role string
	if err := s.db.QueryRow(ctx, `SELECT role FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`,
		workspaceID, user.ID).Scan(&role); err != nil {
		return false
	}
	return role == "OWNER" || role == "MANAGER"
}
