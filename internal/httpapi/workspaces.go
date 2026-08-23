package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,46}[a-z0-9]$`)

type workspaceItem struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	Kind        string    `json:"kind"`
	Role        string    `json:"role"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (s *Server) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	rows, err := s.db.Query(r.Context(), `SELECT w.id,w.name,w.slug,w.description,w.kind,wm.role,w.updated_at FROM workspaces w JOIN workspace_members wm ON wm.workspace_id=w.id WHERE wm.user_id=$1 AND w.deleted_at IS NULL ORDER BY CASE w.kind WHEN 'PERSONAL' THEN 0 ELSE 1 END,w.name`, p.User.ID)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "워크스페이스를 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]workspaceItem, 0)
	for rows.Next() {
		var x workspaceItem
		if rows.Scan(&x.ID, &x.Name, &x.Slug, &x.Description, &x.Kind, &x.Role, &x.UpdatedAt) == nil {
			items = append(items, x)
		}
	}
	writeData(w, 200, items)
}

func (s *Server) createWorkspace(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	var input struct {
		Name        string `json:"name"`
		Slug        string `json:"slug"`
		Description string `json:"description"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Slug = strings.ToLower(strings.TrimSpace(input.Slug))
	if input.Name == "" || len([]rune(input.Name)) > 80 || !slugPattern.MatchString(input.Slug) {
		writeError(w, 400, "INVALID_WORKSPACE", "이름과 영문 소문자 slug(3~48자)를 확인해 주세요.")
		return
	}
	id := uuid.New()
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "워크스페이스를 만들지 못했습니다.")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `INSERT INTO workspaces(id,name,slug,description,kind,owner_id) VALUES($1,$2,$3,$4,'TEAM',$5)`, id, input.Name, input.Slug, input.Description, p.User.ID); err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO workspace_members(workspace_id,user_id,role) VALUES($1,$2,'OWNER')`, id, p.User.ID)
	}
	if err != nil {
		writeError(w, 409, "WORKSPACE_CONFLICT", "같은 slug의 워크스페이스가 있거나 입력값이 올바르지 않습니다.")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "DATABASE_ERROR", "워크스페이스를 만들지 못했습니다.")
		return
	}
	s.audit(r, &p.User.ID, "CREATE_WORKSPACE", "WORKSPACE", &id, nil)
	writeData(w, 201, map[string]any{"id": id, "name": input.Name, "slug": input.Slug, "description": input.Description, "kind": "TEAM", "role": "OWNER"})
}

func (s *Server) getWorkspace(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	var x workspaceItem
	err := s.db.QueryRow(r.Context(), `SELECT w.id,w.name,w.slug,w.description,w.kind,wm.role,w.updated_at FROM workspaces w JOIN workspace_members wm ON wm.workspace_id=w.id WHERE w.id=$1 AND wm.user_id=$2 AND w.deleted_at IS NULL`, id, p.User.ID).Scan(&x.ID, &x.Name, &x.Slug, &x.Description, &x.Kind, &x.Role, &x.UpdatedAt)
	if err != nil {
		writeError(w, 404, "WORKSPACE_NOT_FOUND", "워크스페이스를 찾을 수 없습니다.")
		return
	}
	writeData(w, 200, x)
}

func (s *Server) createFolder(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	var input struct {
		Name     string     `json:"name"`
		ParentID *uuid.UUID `json:"parentId"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len([]rune(input.Name)) > 120 {
		writeError(w, 400, "INVALID_FOLDER", "폴더 이름을 확인해 주세요.")
		return
	}
	var role string
	if s.db.QueryRow(r.Context(), `SELECT role FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`, workspaceID, p.User.ID).Scan(&role) != nil || role == "VIEWER" {
		writeError(w, 403, "WORKSPACE_PERMISSION_DENIED", "폴더를 만들 권한이 없습니다.")
		return
	}
	id := uuid.New()
	result, err := s.db.Exec(r.Context(), `INSERT INTO folders(id,workspace_id,parent_id,name,owner_id) SELECT $1,$2,$3,$4,$5 WHERE $3::uuid IS NULL OR EXISTS(SELECT 1 FROM folders WHERE id=$3 AND workspace_id=$2 AND deleted_at IS NULL)`, id, workspaceID, input.ParentID, input.Name, p.User.ID)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, 400, "FOLDER_CREATE_FAILED", "폴더를 만들지 못했습니다.")
		return
	}
	s.audit(r, &p.User.ID, "CREATE_FOLDER", "FOLDER", &id, nil)
	writeData(w, 201, map[string]any{"id": id, "workspaceId": workspaceID, "parentId": input.ParentID, "name": input.Name})
}

func (s *Server) listFolders(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	var member bool
	_ = s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM workspace_members WHERE workspace_id=$1 AND user_id=$2)`, workspaceID, p.User.ID).Scan(&member)
	if !member && p.User.Role != "ADMIN" {
		writeError(w, 403, "WORKSPACE_PERMISSION_DENIED", "폴더를 볼 권한이 없습니다.")
		return
	}
	rows, err := s.db.Query(r.Context(), `SELECT id,parent_id,name,created_at,updated_at FROM folders WHERE workspace_id=$1 AND deleted_at IS NULL ORDER BY parent_id NULLS FIRST,name`, workspaceID)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "폴더를 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id uuid.UUID
		var parentID *uuid.UUID
		var name string
		var created, updated time.Time
		if rows.Scan(&id, &parentID, &name, &created, &updated) == nil {
			items = append(items, map[string]any{"id": id, "workspaceId": workspaceID, "parentId": parentID, "name": name, "createdAt": created, "updatedAt": updated})
		}
	}
	writeData(w, 200, items)
}

func (s *Server) listWorkspaceMembers(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	var member bool
	_ = s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM workspace_members WHERE workspace_id=$1 AND user_id=$2)`, workspaceID, p.User.ID).Scan(&member)
	if !member && p.User.Role != "ADMIN" {
		writeError(w, 403, "WORKSPACE_PERMISSION_DENIED", "워크스페이스 구성원을 볼 권한이 없습니다.")
		return
	}
	rows, err := s.db.Query(r.Context(), `SELECT u.id,u.username,u.email,u.display_name,u.avatar_url,wm.role,wm.created_at FROM workspace_members wm JOIN users u ON u.id=wm.user_id WHERE wm.workspace_id=$1 ORDER BY CASE wm.role WHEN 'OWNER' THEN 0 WHEN 'MANAGER' THEN 1 ELSE 2 END,u.display_name`, workspaceID)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "워크스페이스 구성원을 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id uuid.UUID
		var username, email, displayName, role string
		var avatar *string
		var created time.Time
		if rows.Scan(&id, &username, &email, &displayName, &avatar, &role, &created) == nil {
			items = append(items, map[string]any{"id": id, "username": username, "email": email, "displayName": displayName, "avatarUrl": avatar, "role": role, "createdAt": created})
		}
	}
	writeData(w, 200, items)
}

func (s *Server) upsertWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	actorRole := ""
	_ = s.db.QueryRow(r.Context(), `SELECT role FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`, workspaceID, p.User.ID).Scan(&actorRole)
	if p.User.Role != "ADMIN" && actorRole != "OWNER" && actorRole != "MANAGER" {
		writeError(w, 403, "WORKSPACE_PERMISSION_DENIED", "구성원을 관리할 권한이 없습니다.")
		return
	}
	var input struct {
		UserID uuid.UUID `json:"userId"`
		Role   string    `json:"role"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Role = strings.ToUpper(input.Role)
	if input.UserID == uuid.Nil || !contains([]string{"MANAGER", "MEMBER", "VIEWER"}, input.Role) || (actorRole == "MANAGER" && input.Role == "MANAGER") {
		writeError(w, 400, "INVALID_WORKSPACE_MEMBER", "추가할 사용자와 허용된 역할을 확인해 주세요.")
		return
	}
	result, err := s.db.Exec(r.Context(), `INSERT INTO workspace_members(workspace_id,user_id,role)
		SELECT $1,$2,$3 WHERE EXISTS(SELECT 1 FROM users WHERE id=$2 AND status='ACTIVE')
		ON CONFLICT(workspace_id,user_id) DO UPDATE SET role=excluded.role
		WHERE workspace_members.role<>'OWNER'`, workspaceID, input.UserID, input.Role)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, 409, "WORKSPACE_MEMBER_SAVE_FAILED", "사용자를 추가하지 못했거나 소유자 역할은 변경할 수 없습니다.")
		return
	}
	s.audit(r, &p.User.ID, "UPSERT_WORKSPACE_MEMBER", "WORKSPACE", &workspaceID, map[string]any{"userId": input.UserID, "role": input.Role})
	writeData(w, 200, map[string]any{"userId": input.UserID, "role": input.Role})
}

func (s *Server) deleteWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	userID, ok := pathUUID(w, r, "userId")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	actorRole := ""
	_ = s.db.QueryRow(r.Context(), `SELECT role FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`, workspaceID, p.User.ID).Scan(&actorRole)
	if p.User.Role != "ADMIN" && actorRole != "OWNER" && actorRole != "MANAGER" {
		writeError(w, 403, "WORKSPACE_PERMISSION_DENIED", "구성원을 관리할 권한이 없습니다.")
		return
	}
	result, err := s.db.Exec(r.Context(), `DELETE FROM workspace_members wm USING workspaces w WHERE wm.workspace_id=$1 AND wm.user_id=$2 AND w.id=wm.workspace_id AND w.owner_id<>wm.user_id AND ($3<>'MANAGER' OR wm.role IN ('MEMBER','VIEWER'))`, workspaceID, userID, actorRole)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, 409, "WORKSPACE_MEMBER_DELETE_FAILED", "소유자이거나 제거 권한이 없는 구성원입니다.")
		return
	}
	s.audit(r, &p.User.ID, "DELETE_WORKSPACE_MEMBER", "WORKSPACE", &workspaceID, map[string]any{"userId": userID})
	w.WriteHeader(204)
}
