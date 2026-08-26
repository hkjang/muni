package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// overview is what an operator is asked about: how much is in the system, who
// is using it, and whether the parts that reach outside are working.
//
// The settings form used to be the first thing an administrator saw, which
// answers none of those and invites editing configuration to find out.
func (s *Server) adminOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var users struct {
		Total     int64 `json:"total"`
		Active    int64 `json:"active"`
		Suspended int64 `json:"suspended"`
		Admins    int64 `json:"admins"`
		Recent    int64 `json:"recentLogins"`
	}
	if err := s.db.QueryRow(ctx, `
		SELECT count(*),
			count(*) FILTER (WHERE status = 'ACTIVE'),
			count(*) FILTER (WHERE status = 'SUSPENDED'),
			count(*) FILTER (WHERE role = 'ADMIN'),
			count(*) FILTER (WHERE last_login_at >= now() - interval '7 days')
		FROM users`).Scan(&users.Total, &users.Active, &users.Suspended, &users.Admins, &users.Recent); err != nil {
		writeError(w, 500, "DATABASE_ERROR", "운영 현황을 불러오지 못했습니다.")
		return
	}

	var documents struct {
		Total    int64 `json:"total"`
		Trashed  int64 `json:"trashed"`
		Created  int64 `json:"createdThisWeek"`
		Edited   int64 `json:"editedThisWeek"`
		Pending  int64 `json:"pendingApproval"`
		Revision int64 `json:"revisions"`
	}
	_ = s.db.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE deleted_at IS NULL),
			count(*) FILTER (WHERE deleted_at IS NOT NULL),
			count(*) FILTER (WHERE deleted_at IS NULL AND created_at >= now() - interval '7 days'),
			count(*) FILTER (WHERE deleted_at IS NULL AND updated_at >= now() - interval '7 days'),
			count(*) FILTER (WHERE deleted_at IS NULL AND workflow_status = 'PENDING')
		FROM documents`).Scan(&documents.Total, &documents.Trashed, &documents.Created, &documents.Edited, &documents.Pending)
	_ = s.db.QueryRow(ctx, `SELECT count(*) FROM document_revisions`).Scan(&documents.Revision)

	var storage struct {
		Workspaces  int64 `json:"workspaces"`
		Attachments int64 `json:"attachments"`
		Bytes       int64 `json:"bytes"`
		Sessions    int64 `json:"sessions"`
		APIKeys     int64 `json:"apiKeys"`
	}
	_ = s.db.QueryRow(ctx, `SELECT count(*) FROM workspaces WHERE deleted_at IS NULL`).Scan(&storage.Workspaces)
	_ = s.db.QueryRow(ctx, `SELECT count(*), coalesce(sum(size_bytes), 0) FROM attachments`).Scan(&storage.Attachments, &storage.Bytes)
	_ = s.db.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE expires_at > now()`).Scan(&storage.Sessions)
	_ = s.db.QueryRow(ctx, `SELECT count(*) FROM api_keys WHERE revoked_at IS NULL`).Scan(&storage.APIKeys)

	writeData(w, 200, map[string]any{
		"users":     users,
		"documents": documents,
		"storage":   storage,
		"checks":    s.systemChecks(ctx),
		"activity":  s.recentAdminActivity(ctx),
		"build": map[string]any{
			"version": s.info.Version,
			"commit":  s.info.Commit,
		},
	})
}

// check is one thing that can be working or not.
type check struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	State   string `json:"state"` // ok | off | warn
	Detail  string `json:"detail"`
	Setting string `json:"setting,omitempty"`
}

// systemChecks reports on the parts that reach outside the process. Nothing
// here calls a remote service: an overview that waits on someone else's
// gateway is an overview that does not load.
func (s *Server) systemChecks(ctx context.Context) []check {
	checks := make([]check, 0, 6)

	dbState, dbDetail := "ok", "연결됨"
	if err := s.db.Ping(ctx); err != nil {
		dbState, dbDetail = "warn", "연결 실패: "+err.Error()
	} else {
		var applied int
		var latest string
		if s.db.QueryRow(ctx, `SELECT count(*), coalesce(max(version), '')FROM schema_migrations`).Scan(&applied, &latest) == nil {
			dbDetail = "마이그레이션 " + strings.TrimSuffix(latest, ".sql")
		}
	}
	checks = append(checks, check{Key: "database", Label: "데이터베이스", State: dbState, Detail: dbDetail})

	all, err := s.settings.GetAll(ctx, false)
	if err != nil {
		return checks
	}

	aiState, aiDetail := "off", "설정되지 않음"
	switch {
	case !all.AI.Enabled:
		aiDetail = "비활성"
	case strings.TrimSpace(all.AI.BaseURL) == "" || strings.TrimSpace(all.AI.Model) == "":
		aiState, aiDetail = "warn", "활성화되었지만 주소나 모델이 비어 있음"
	default:
		aiState, aiDetail = "ok", all.AI.Model
	}
	checks = append(checks, check{Key: "ai", Label: "AI", State: aiState, Detail: aiDetail, Setting: "/admin/settings"})

	oidcState, oidcDetail := "off", "로컬 로그인만 사용"
	if all.OIDC.Enabled {
		if strings.TrimSpace(all.OIDC.IssuerURL) == "" {
			oidcState, oidcDetail = "warn", "활성화되었지만 Issuer가 비어 있음"
		} else {
			oidcState, oidcDetail = "ok", all.OIDC.IssuerURL
		}
	}
	checks = append(checks, check{Key: "oidc", Label: "OIDC SSO", State: oidcState, Detail: oidcDetail, Setting: "/admin/settings"})

	ptiumState, ptiumDetail := "off", "설정되지 않음"
	if all.Ptium.Enabled {
		switch {
		case strings.TrimSpace(all.Ptium.BaseURL) == "":
			ptiumState, ptiumDetail = "warn", "활성화되었지만 주소가 비어 있음"
		case !all.Ptium.APIKeySet:
			ptiumState, ptiumDetail = "warn", "API key가 없음"
		default:
			ptiumState, ptiumDetail = "ok", all.Ptium.BaseURL
		}
	}
	checks = append(checks, check{Key: "ptium", Label: "발표자료 연동", State: ptiumState, Detail: ptiumDetail, Setting: "/admin/settings"})

	pdfState, pdfDetail := "off", "내보내기 정책에서 꺼져 있음"
	if all.Export.EnablePDF {
		if binary, err := chromiumBinary(); err == nil {
			pdfState, pdfDetail = "ok", binary
		} else {
			pdfState, pdfDetail = "warn", "켜져 있지만 브라우저를 찾지 못함"
		}
	}
	checks = append(checks, check{Key: "pdf", Label: "PDF 내보내기", State: pdfState, Detail: pdfDetail, Setting: "/admin/settings"})

	return checks
}

func (s *Server) recentAdminActivity(ctx context.Context) []map[string]any {
	items := make([]map[string]any, 0, 8)
	rows, err := s.db.Query(ctx, `
		SELECT a.action, u.display_name, a.created_at
		FROM activity_logs a LEFT JOIN users u ON u.id = a.actor_id
		WHERE a.action NOT IN ('READ_DOCUMENT')
		ORDER BY a.created_at DESC LIMIT 8`)
	if err != nil {
		return items
	}
	defer rows.Close()
	for rows.Next() {
		var action string
		var actor *string
		var created time.Time
		if rows.Scan(&action, &actor, &created) == nil {
			items = append(items, map[string]any{"action": action, "actorName": actor, "createdAt": created})
		}
	}
	return items
}

// adminWorkspaces lists every workspace with the two numbers that say whether
// it is in use. There was no way to see a workspace an administrator was not a
// member of, which is most of them.
func (s *Server) adminWorkspaces(w http.ResponseWriter, r *http.Request) {
	limit := parseLimit(r.URL.Query().Get("limit"), 100)
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	rows, err := s.db.Query(r.Context(), `
		SELECT w.id, w.name, w.slug, w.kind, w.owner_id, u.display_name,
			(SELECT count(*) FROM workspace_members m WHERE m.workspace_id = w.id),
			(SELECT count(*) FROM documents d WHERE d.workspace_id = w.id AND d.deleted_at IS NULL),
			(SELECT max(d.updated_at) FROM documents d WHERE d.workspace_id = w.id AND d.deleted_at IS NULL),
			w.created_at
		FROM workspaces w JOIN users u ON u.id = w.owner_id
		WHERE w.deleted_at IS NULL
			AND ($1 = '' OR w.name ILIKE '%'||$1||'%' OR w.slug ILIKE '%'||$1||'%')
		ORDER BY w.created_at DESC LIMIT $2`, query, limit)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "워크스페이스 목록을 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0, limit)
	for rows.Next() {
		var id, ownerID uuid.UUID
		var name, slug, kind, owner string
		var members, documents int64
		var lastEdit *time.Time
		var created time.Time
		if rows.Scan(&id, &name, &slug, &kind, &ownerID, &owner, &members, &documents, &lastEdit, &created) == nil {
			items = append(items, map[string]any{
				"id": id, "name": name, "slug": slug, "kind": kind,
				"ownerId": ownerID, "ownerName": owner,
				"members": members, "documents": documents,
				"lastEditedAt": lastEdit, "createdAt": created,
			})
		}
	}
	writeData(w, 200, items)
}
