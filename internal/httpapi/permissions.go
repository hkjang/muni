package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var roleRank = map[string]int{"VIEWER": 1, "COMMENTER": 2, "EDITOR": 3, "OWNER": 4}

func (s *Server) documentRole(ctx context.Context, user User, documentID uuid.UUID, includeDeleted bool) (string, error) {
	var ownerID, workspaceID uuid.UUID
	var visibility string
	var deletedAt *time.Time
	err := s.db.QueryRow(ctx, `SELECT owner_id,workspace_id,visibility,deleted_at FROM documents WHERE id=$1`, documentID).Scan(&ownerID, &workspaceID, &visibility, &deletedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", databaseNotFound()
		}
		return "", err
	}
	if deletedAt != nil && !includeDeleted {
		return "", databaseNotFound()
	}
	if user.Role == "ADMIN" || ownerID == user.ID {
		return "OWNER", nil
	}
	var explicit string
	err = s.db.QueryRow(ctx, `SELECT role FROM document_permissions WHERE document_id=$1 AND subject_type='USER' AND subject_id=$2
		AND (expires_at IS NULL OR expires_at>now())`, documentID, user.ID).Scan(&explicit)
	if err == nil {
		return explicit, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	var memberRole string
	err = s.db.QueryRow(ctx, `SELECT role FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`, workspaceID, user.ID).Scan(&memberRole)
	if err == nil {
		if visibility == "WORKSPACE" || visibility == "ORGANIZATION" {
			if memberRole == "OWNER" || memberRole == "MANAGER" {
				return "EDITOR", nil
			}
			return "VIEWER", nil
		}
	}
	if visibility == "ORGANIZATION" {
		return "VIEWER", nil
	}
	return "", errForbidden
}

func databaseNotFound() error { return errors.New("not found") }

func requireDocumentRole(w http.ResponseWriter, role string, minimum string) bool {
	if roleRank[role] < roleRank[minimum] {
		writeError(w, http.StatusForbidden, "DOCUMENT_PERMISSION_DENIED", "이 작업을 수행할 문서 권한이 없습니다.")
		return false
	}
	return true
}

func (s *Server) canReview(ctx context.Context, user User, documentID uuid.UUID) bool {
	if user.Role == "ADMIN" {
		return true
	}
	var allowed bool
	_ = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM documents d JOIN workspace_members wm ON wm.workspace_id=d.workspace_id
		WHERE d.id=$1 AND wm.user_id=$2 AND wm.role IN ('OWNER','MANAGER'))`, documentID, user.ID).Scan(&allowed)
	return allowed
}

func (s *Server) listDocumentPermissions(w http.ResponseWriter, r *http.Request) {
	documentID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	role, err := s.documentRole(r.Context(), p.User, documentID, false)
	if err != nil || !requireDocumentRole(w, role, "OWNER") {
		return
	}
	rows, err := s.db.Query(r.Context(), `SELECT p.id,p.subject_type,p.subject_id,p.role,p.expires_at,p.created_at,
		CASE WHEN p.subject_type='USER' THEN u.display_name ELSE p.subject_type END
		FROM document_permissions p LEFT JOIN users u ON p.subject_type='USER' AND u.id=p.subject_id WHERE p.document_id=$1 ORDER BY p.created_at`, documentID)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "공유 권한을 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id uuid.UUID
		var subjectType, permissionRole, label string
		var subjectID *uuid.UUID
		var expires *time.Time
		var created time.Time
		if err := rows.Scan(&id, &subjectType, &subjectID, &permissionRole, &expires, &created, &label); err != nil {
			continue
		}
		items = append(items, map[string]any{"id": id, "subjectType": subjectType, "subjectId": subjectID, "role": permissionRole, "expiresAt": expires, "createdAt": created, "label": label})
	}
	writeData(w, 200, items)
}

func (s *Server) upsertDocumentPermission(w http.ResponseWriter, r *http.Request) {
	documentID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	role, err := s.documentRole(r.Context(), p.User, documentID, false)
	if err != nil || !requireDocumentRole(w, role, "OWNER") {
		return
	}
	var input struct {
		SubjectType string     `json:"subjectType"`
		SubjectID   *uuid.UUID `json:"subjectId"`
		Role        string     `json:"role"`
		ExpiresAt   *time.Time `json:"expiresAt"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.SubjectType != "USER" || input.SubjectID == nil || roleRank[input.Role] < 1 || input.Role == "OWNER" {
		writeError(w, 400, "INVALID_PERMISSION", "현재 API는 사용자 VIEWER/COMMENTER/EDITOR 공유를 지원합니다.")
		return
	}
	var id uuid.UUID
	err = s.db.QueryRow(r.Context(), `INSERT INTO document_permissions(document_id,subject_type,subject_id,role,expires_at,created_by)
		VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(document_id,subject_type,subject_id) DO UPDATE SET role=excluded.role,expires_at=excluded.expires_at RETURNING id`, documentID, input.SubjectType, input.SubjectID, input.Role, input.ExpiresAt, p.User.ID).Scan(&id)
	if err != nil {
		writeError(w, 400, "PERMISSION_SAVE_FAILED", "공유 권한을 저장하지 못했습니다.")
		return
	}
	s.audit(r, &p.User.ID, "SHARE_DOCUMENT", "DOCUMENT", &documentID, map[string]any{"permissionId": id, "role": input.Role})
	writeData(w, 200, map[string]any{"id": id})
}

func (s *Server) deleteDocumentPermission(w http.ResponseWriter, r *http.Request) {
	documentID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	permissionID, ok := pathUUID(w, r, "permissionId")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	role, err := s.documentRole(r.Context(), p.User, documentID, false)
	if err != nil || !requireDocumentRole(w, role, "OWNER") {
		return
	}
	result, err := s.db.Exec(r.Context(), `DELETE FROM document_permissions WHERE id=$1 AND document_id=$2`, permissionID, documentID)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, 404, "PERMISSION_NOT_FOUND", "공유 권한을 찾을 수 없습니다.")
		return
	}
	s.audit(r, &p.User.ID, "REMOVE_DOCUMENT_PERMISSION", "DOCUMENT", &documentID, map[string]any{"permissionId": permissionID})
	w.WriteHeader(204)
}
