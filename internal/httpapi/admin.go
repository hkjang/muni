package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *Server) searchUsers(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(query)) < 2 {
		writeData(w, http.StatusOK, []any{})
		return
	}
	rows, err := s.db.Query(r.Context(), `SELECT id,display_name,username,email FROM users
		WHERE status='ACTIVE' AND id<>$1 AND (display_name ILIKE '%'||$2||'%' OR username ILIKE '%'||$2||'%' OR email ILIKE '%'||$2||'%')
		ORDER BY display_name LIMIT 20`, p.User.ID, query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "사용자를 검색하지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id uuid.UUID
		var displayName, username, email string
		if rows.Scan(&id, &displayName, &username, &email) == nil {
			items = append(items, map[string]any{"id": id, "displayName": displayName, "username": username, "email": email})
		}
	}
	writeData(w, http.StatusOK, items)
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := parseLimit(r.URL.Query().Get("limit"), 50)
	rows, err := s.db.Query(r.Context(), `SELECT id,username,email,display_name,role,status,avatar_url,locale,created_at,last_login_at FROM users WHERE $1='' OR username ILIKE '%'||$1||'%' OR email ILIKE '%'||$1||'%' OR display_name ILIKE '%'||$1||'%' ORDER BY created_at DESC LIMIT $2`, q, limit)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "사용자 목록을 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var u User
		var lastLogin *time.Time
		if rows.Scan(&u.ID, &u.Username, &u.Email, &u.DisplayName, &u.Role, &u.Status, &u.AvatarURL, &u.Locale, &u.CreatedAt, &lastLogin) == nil {
			items = append(items, map[string]any{"id": u.ID, "username": u.Username, "email": u.Email, "displayName": u.DisplayName, "role": u.Role, "status": u.Status, "avatarUrl": u.AvatarURL, "locale": u.Locale, "createdAt": u.CreatedAt, "lastLoginAt": lastLogin})
		}
	}
	writeData(w, 200, items)
}
func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	var input struct {
		Role        *string `json:"role"`
		Status      *string `json:"status"`
		DisplayName *string `json:"displayName"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Role != nil && !contains([]string{"ADMIN", "USER"}, *input.Role) {
		writeError(w, 400, "INVALID_ROLE", "역할 값이 올바르지 않습니다.")
		return
	}
	if input.Status != nil && !contains([]string{"ACTIVE", "SUSPENDED"}, *input.Status) {
		writeError(w, 400, "INVALID_STATUS", "상태 값이 올바르지 않습니다.")
		return
	}
	if id == p.User.ID && ((input.Role != nil && *input.Role != "ADMIN") || (input.Status != nil && *input.Status != "ACTIVE")) {
		writeError(w, 409, "SELF_LOCKOUT", "현재 관리자 자신의 권한을 낮추거나 계정을 정지할 수 없습니다.")
		return
	}
	if input.DisplayName != nil {
		v := strings.TrimSpace(*input.DisplayName)
		if v == "" || len([]rune(v)) > 100 {
			writeError(w, 400, "INVALID_DISPLAY_NAME", "표시 이름을 확인해 주세요.")
			return
		}
		input.DisplayName = &v
	}
	result, err := s.db.Exec(r.Context(), `UPDATE users SET role=COALESCE($2,role),status=COALESCE($3,status),display_name=COALESCE($4,display_name),updated_at=now() WHERE id=$1`, id, input.Role, input.Status, input.DisplayName)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, 404, "USER_NOT_FOUND", "사용자를 찾을 수 없습니다.")
		return
	}
	s.audit(r, &p.User.ID, "UPDATE_USER", "USER", &id, map[string]any{"role": input.Role, "status": input.Status})
	writeData(w, 200, map[string]any{"id": id})
}
func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	limit := parseLimit(r.URL.Query().Get("limit"), 100)
	action := strings.TrimSpace(r.URL.Query().Get("action"))
	rows, err := s.db.Query(r.Context(), `SELECT a.id,a.actor_id,u.display_name,a.action,a.resource_type,a.resource_id,a.ip,a.metadata,a.created_at FROM activity_logs a LEFT JOIN users u ON u.id=a.actor_id WHERE $1='' OR a.action=$1 ORDER BY a.created_at DESC LIMIT $2`, action, limit)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "감사 로그를 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var actorID, resourceID *uuid.UUID
		var actor, action, resourceType *string
		var ip any
		var metadata any
		var created time.Time
		if rows.Scan(&id, &actorID, &actor, &action, &resourceType, &resourceID, &ip, &metadata, &created) == nil {
			items = append(items, map[string]any{"id": id, "actorId": actorID, "actorName": actor, "action": action, "resourceType": resourceType, "resourceId": resourceID, "ip": ip, "metadata": metadata, "createdAt": created})
		}
	}
	writeData(w, 200, items)
}
