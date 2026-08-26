package httpapi

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// listUsers answers the questions an administrator actually has.
//
// It used to be one ILIKE and `ORDER BY created_at DESC LIMIT 50`. With three
// hundred staff that shows the fifty most recently added and nothing else —
// there was no second page. "Who has not signed in since March", "who was
// invited and never arrived", "who are the administrators" had no answer at
// all, which is a strange gap in the screen whose whole job is those
// questions.
func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	q := strings.TrimSpace(query.Get("q"))
	limit := parseLimit(query.Get("limit"), 50)
	offset, _ := strconv.Atoi(query.Get("offset"))
	if offset < 0 {
		offset = 0
	}

	status := strings.ToUpper(strings.TrimSpace(query.Get("status")))
	if !contains([]string{"", "ACTIVE", "SUSPENDED"}, status) {
		writeError(w, 400, "INVALID_STATUS", "상태 값이 올바르지 않습니다.")
		return
	}
	role := strings.ToUpper(strings.TrimSpace(query.Get("role")))
	if !contains([]string{"", "ADMIN", "USER"}, role) {
		writeError(w, 400, "INVALID_ROLE", "역할 값이 올바르지 않습니다.")
		return
	}
	// LOCAL and SSO answer "who still has a password here", which is what an
	// organisation moving onto its identity provider needs to see.
	auth := strings.ToUpper(strings.TrimSpace(query.Get("auth")))
	if !contains([]string{"", "LOCAL", "SSO"}, auth) {
		writeError(w, 400, "INVALID_AUTH", "인증 방식 값이 올바르지 않습니다.")
		return
	}
	// Zero means no filter. An account that has never signed in counts as
	// inactive however long ago it was made — that is the invitation nobody
	// accepted, and it is the row worth finding.
	inactiveDays, _ := strconv.Atoi(query.Get("inactiveDays"))
	if inactiveDays < 0 || inactiveDays > 3650 {
		inactiveDays = 0
	}
	pendingPassword := query.Get("pendingPassword") == "true"

	// The sort is a fixed set, never text from the request: an ORDER BY built
	// from a parameter is how a list endpoint becomes an injection.
	order := map[string]string{
		"recent":    "created_at DESC, id",
		"oldest":    "created_at ASC, id",
		"lastLogin": "last_login_at DESC NULLS LAST, id",
		"stale":     "last_login_at ASC NULLS FIRST, id",
		"name":      "display_name ASC, id",
	}[query.Get("sort")]
	if order == "" {
		order = "created_at DESC, id"
	}

	const where = `
		WHERE ($1='' OR username ILIKE '%'||$1||'%' OR email ILIKE '%'||$1||'%' OR display_name ILIKE '%'||$1||'%')
			AND ($2='' OR status=$2)
			AND ($3='' OR role=$3)
			AND ($4='' OR ($4='LOCAL' AND password_hash IS NOT NULL) OR ($4='SSO' AND oidc_subject IS NOT NULL))
			AND ($5=0 OR last_login_at IS NULL OR last_login_at < now() - make_interval(days => $5))
			AND (NOT $6 OR password_reset_required)`

	var total int
	if err := s.db.QueryRow(r.Context(), `SELECT count(*) FROM users`+where,
		q, status, role, auth, inactiveDays, pendingPassword).Scan(&total); err != nil {
		writeError(w, 500, "DATABASE_ERROR", "사용자 수를 세지 못했습니다.")
		return
	}

	rows, err := s.db.Query(r.Context(),
		`SELECT id,username,email,display_name,role,status,avatar_url,locale,created_at,last_login_at,password_reset_required,
			password_hash IS NOT NULL, oidc_subject IS NOT NULL
		 FROM users`+where+` ORDER BY `+order+` LIMIT $7 OFFSET $8`,
		q, status, role, auth, inactiveDays, pendingPassword, limit, offset)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "사용자 목록을 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var u User
		var lastLogin *time.Time
		var hasPassword, hasSSO bool
		if rows.Scan(&u.ID, &u.Username, &u.Email, &u.DisplayName, &u.Role, &u.Status, &u.AvatarURL,
			&u.Locale, &u.CreatedAt, &lastLogin, &u.MustChangePassword, &hasPassword, &hasSSO) == nil {
			items = append(items, map[string]any{
				"id": u.ID, "username": u.Username, "email": u.Email,
				"displayName": u.DisplayName, "role": u.Role, "status": u.Status,
				"avatarUrl": u.AvatarURL, "locale": u.Locale, "createdAt": u.CreatedAt,
				"lastLoginAt": lastLogin, "mustChangePassword": u.MustChangePassword,
				"hasPassword": hasPassword, "hasSSO": hasSSO,
			})
		}
	}
	writeData(w, 200, map[string]any{
		"items": items, "total": total, "limit": limit, "offset": offset,
	})
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

// listAudit answers the audit screen.
//
// It used to take a limit and an exact action and nothing else, which makes
// the log unusable for the question it exists to answer: what did this person
// do, or what happened to this document, between these two dates. Paging is by
// the row id rather than an offset so a busy log does not shift rows under a
// reader moving through it.
func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit := parseLimit(query.Get("limit"), 100)
	action := strings.TrimSpace(query.Get("action"))
	resource := strings.TrimSpace(strings.ToUpper(query.Get("resourceType")))
	search := strings.TrimSpace(query.Get("q"))

	var actor *uuid.UUID
	if raw := strings.TrimSpace(query.Get("actorId")); raw != "" {
		if parsed, err := uuid.Parse(raw); err == nil {
			actor = &parsed
		}
	}
	var resourceID *uuid.UUID
	if raw := strings.TrimSpace(query.Get("resourceId")); raw != "" {
		if parsed, err := uuid.Parse(raw); err == nil {
			resourceID = &parsed
		}
	}

	// A blank date means "no bound", which is what the widest view needs.
	from, to := parseDayBound(query.Get("from"), time.Time{}), parseDayBound(query.Get("to"), time.Time{})
	if !to.IsZero() {
		to = to.Add(24 * time.Hour)
	}

	var before int64
	if raw := strings.TrimSpace(query.Get("before")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil {
			before = parsed
		}
	}

	rows, err := s.db.Query(r.Context(), auditQuery, action, actor, resource, resourceID,
		nullTime(from), nullTime(to), search, before, limit)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "감사 로그를 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0, limit)
	for rows.Next() {
		if item, ok := scanAuditRow(rows); ok {
			items = append(items, item)
		}
	}
	writeData(w, 200, map[string]any{"items": items, "limit": limit})
}

// auditQuery is shared by the screen and the CSV download so the two can never
// disagree about what was filtered.
const auditQuery = `SELECT a.id,a.actor_id,u.display_name,a.action,a.resource_type,a.resource_id,a.ip,a.metadata,a.created_at
	FROM activity_logs a LEFT JOIN users u ON u.id=a.actor_id
	WHERE ($1 = '' OR a.action = $1)
		AND ($2::uuid IS NULL OR a.actor_id = $2)
		AND ($3 = '' OR a.resource_type = $3)
		AND ($4::uuid IS NULL OR a.resource_id = $4)
		AND ($5::timestamptz IS NULL OR a.created_at >= $5)
		AND ($6::timestamptz IS NULL OR a.created_at < $6)
		AND ($7 = '' OR a.action ILIKE '%'||$7||'%' OR u.display_name ILIKE '%'||$7||'%')
		AND ($8::bigint = 0 OR a.id < $8)
	ORDER BY a.id DESC LIMIT $9`

func scanAuditRow(rows pgx.Rows) (map[string]any, bool) {
	var id int64
	var actorID, resourceID *uuid.UUID
	var actor, action, resourceType *string
	var ip, metadata any
	var created time.Time
	if rows.Scan(&id, &actorID, &actor, &action, &resourceType, &resourceID, &ip, &metadata, &created) != nil {
		return nil, false
	}
	return map[string]any{
		"id": id, "actorId": actorID, "actorName": actor, "action": action,
		"resourceType": resourceType, "resourceId": resourceID, "ip": ip,
		"metadata": metadata, "createdAt": created,
	}, true
}

// exportAudit hands the filtered log over as CSV.
//
// An audit log that can only be read on screen cannot be handed to whoever
// asked for it, which is most of the reason one is kept.
func (s *Server) exportAudit(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	action := strings.TrimSpace(query.Get("action"))
	resource := strings.TrimSpace(strings.ToUpper(query.Get("resourceType")))
	search := strings.TrimSpace(query.Get("q"))
	var actor, resourceID *uuid.UUID
	if parsed, err := uuid.Parse(strings.TrimSpace(query.Get("actorId"))); err == nil {
		actor = &parsed
	}
	if parsed, err := uuid.Parse(strings.TrimSpace(query.Get("resourceId"))); err == nil {
		resourceID = &parsed
	}
	from, to := parseDayBound(query.Get("from"), time.Time{}), parseDayBound(query.Get("to"), time.Time{})
	if !to.IsZero() {
		to = to.Add(24 * time.Hour)
	}

	rows, err := s.db.Query(r.Context(), auditQuery, action, actor, resource, resourceID,
		nullTime(from), nullTime(to), search, int64(0), maxAuditExport)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "감사 로그를 불러오지 못했습니다.")
		return
	}
	defer rows.Close()

	p, _ := principalFrom(r.Context())
	s.audit(r, &p.User.ID, "EXPORT_AUDIT", "SETTINGS", nil, map[string]any{"action": action, "resourceType": resource})

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="muni-audit.csv"`)
	// A byte order mark is what makes Excel open a UTF-8 CSV as Korean rather
	// than as mojibake.
	_, _ = w.Write([]byte("\ufeff"))
	writer := csv.NewWriter(w)
	defer writer.Flush()
	_ = writer.Write([]string{"시각", "행위자", "동작", "대상 종류", "대상 ID", "IP", "상세"})
	for rows.Next() {
		item, ok := scanAuditRow(rows)
		if !ok {
			continue
		}
		metadata := ""
		if encoded, err := json.Marshal(item["metadata"]); err == nil && string(encoded) != "null" {
			metadata = string(encoded)
		}
		_ = writer.Write([]string{
			item["createdAt"].(time.Time).Format(time.RFC3339),
			stringOrEmpty(item["actorName"]),
			stringOrEmpty(item["action"]),
			stringOrEmpty(item["resourceType"]),
			uuidOrEmpty(item["resourceId"]),
			fmt.Sprint(orEmpty(item["ip"])),
			metadata,
		})
	}
}

// maxAuditExport bounds a download that would otherwise stream the whole log
// into a spreadsheet nobody can open.
const maxAuditExport = 20000

func parseDayBound(value string, fallback time.Time) time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	parsed, err := time.ParseInLocation("2006-01-02", trimmed, time.Local)
	if err != nil {
		return fallback
	}
	return parsed
}

func nullTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func stringOrEmpty(value any) string {
	if text, ok := value.(*string); ok && text != nil {
		return *text
	}
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func uuidOrEmpty(value any) string {
	if id, ok := value.(*uuid.UUID); ok && id != nil {
		return id.String()
	}
	return ""
}

func orEmpty(value any) any {
	if value == nil {
		return ""
	}
	return value
}
