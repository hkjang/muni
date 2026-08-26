package httpapi

import (
	"net/http"
	"time"

	"github.com/google/uuid"
)

// documentAccess answers the question an audit actually asks.
//
// muni recorded who read a document, who exported it, who downloaded its
// attachments — the history was complete. What no screen could answer was the
// present tense: who can open this right now. That is three tables and a
// visibility column combined by precedence rules that live in Go, so the
// honest answer was to read documentRole and work it out by hand.
//
// The rules here are the same ones documentRole applies, in the same order.
// If they ever drift, this screen starts lying about access, which is worse
// than not having it — so the test compares the two against a real database
// rather than trusting that they match.
func (s *Server) documentAccess(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}

	var doc struct {
		Title      string
		Visibility string
		Workspace  string
		OwnerID    uuid.UUID
		OwnerName  string
		Deleted    *time.Time
	}
	if err := s.db.QueryRow(r.Context(), `
		SELECT d.title, d.visibility, w.name, d.owner_id, u.display_name, d.deleted_at
		FROM documents d
		JOIN workspaces w ON w.id = d.workspace_id
		JOIN users u ON u.id = d.owner_id
		WHERE d.id = $1`, id).
		Scan(&doc.Title, &doc.Visibility, &doc.Workspace, &doc.OwnerID, &doc.OwnerName, &doc.Deleted); err != nil {
		writeError(w, 404, "DOCUMENT_NOT_FOUND", "문서를 찾을 수 없습니다.")
		return
	}

	// Precedence, lowest number first, matching documentRole: being the owner
	// beats a direct grant, which beats membership of the workspace. Only the
	// winning row is reported, because only it is what the person actually
	// gets.
	rows, err := s.db.Query(r.Context(), `
		WITH candidates AS (
			SELECT u.id, 1 AS precedence, 'OWNER' AS role, 'OWNER' AS via, NULL::timestamptz AS expires_at
			FROM users u WHERE u.id = $2

			UNION ALL
			SELECT p.subject_id, 2, p.role, 'DIRECT', p.expires_at
			FROM document_permissions p
			WHERE p.document_id = $1 AND p.subject_type = 'USER' AND p.subject_id IS NOT NULL
				AND (p.expires_at IS NULL OR p.expires_at > now())

			UNION ALL
			SELECT m.user_id, 3,
				CASE WHEN m.role IN ('OWNER','MANAGER') THEN 'EDITOR' ELSE 'VIEWER' END,
				'WORKSPACE', NULL
			FROM workspace_members m
			JOIN documents d ON d.workspace_id = m.workspace_id
			WHERE d.id = $1 AND $3 IN ('WORKSPACE','ORGANIZATION')
		)
		SELECT DISTINCT ON (c.id) c.id, u.display_name, u.email, u.status, u.role, c.role, c.via, c.expires_at
		FROM candidates c JOIN users u ON u.id = c.id
		ORDER BY c.id, c.precedence
	`, id, doc.OwnerID, doc.Visibility)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "권한을 불러오지 못했습니다.")
		return
	}
	defer rows.Close()

	entries := make([]map[string]any, 0)
	named := map[uuid.UUID]bool{}
	for rows.Next() {
		var userID uuid.UUID
		var name, email, status, accountRole, role, via string
		var expires *time.Time
		if rows.Scan(&userID, &name, &email, &status, &accountRole, &role, &via, &expires) != nil {
			continue
		}
		named[userID] = true
		entries = append(entries, map[string]any{
			"userId": userID, "displayName": name, "email": email,
			"role": role, "via": via, "expiresAt": expires,
			"suspended": status != "ACTIVE",
			// An administrator reaches every document regardless. Saying so on
			// their row stops the list reading as though this grant is what
			// gives them access — revoking it would change nothing.
			"alsoAdmin": accountRole == "ADMIN",
		})
	}

	// Administrators are not listed one by one: they can open every document
	// in the installation, so enumerating them here would make each document
	// look individually over-shared when the truth is a property of the role.
	var admins, activeUsers int
	_ = s.db.QueryRow(r.Context(),
		`SELECT count(*) FILTER (WHERE role='ADMIN' AND status='ACTIVE'), count(*) FILTER (WHERE status='ACTIVE')
		 FROM users`).Scan(&admins, &activeUsers)

	// Anyone holding a live share link can read this document without an
	// account. Leaving it out would make this screen answer "who can open
	// this" with only the people muni knows the names of, which is the wrong
	// answer in exactly the case an audit cares about.
	links := make([]map[string]any, 0)
	linkRows, err := s.db.Query(r.Context(), `
		SELECT l.id, l.label, l.token_prefix, l.password_hash IS NOT NULL,
			l.expires_at, l.max_views, l.view_count, l.last_viewed_at, u.display_name
		FROM document_links l JOIN users u ON u.id = l.created_by
		WHERE l.document_id = $1 AND l.revoked_at IS NULL
			AND (l.expires_at IS NULL OR l.expires_at > now())
			AND (l.max_views IS NULL OR l.view_count < l.max_views)
		ORDER BY l.created_at DESC`, id)
	if err == nil {
		defer linkRows.Close()
		for linkRows.Next() {
			var linkID uuid.UUID
			var label, prefix, creator string
			var hasPassword bool
			var expires, lastViewed *time.Time
			var maxViews *int
			var views int64
			if linkRows.Scan(&linkID, &label, &prefix, &hasPassword, &expires,
				&maxViews, &views, &lastViewed, &creator) == nil {
				links = append(links, map[string]any{
					"id": linkID, "label": label, "prefix": prefix,
					"hasPassword": hasPassword, "expiresAt": expires,
					"maxViews": maxViews, "viewCount": views,
					"lastViewedAt": lastViewed, "createdBy": creator,
				})
			}
		}
	}

	notes := make([]string, 0)
	everyone := map[string]any{"applies": false}
	if doc.Visibility == "ORGANIZATION" {
		everyone = map[string]any{
			"applies": true, "role": "VIEWER", "people": activeUsers,
			"reason": "공개 범위가 '조직 전체'라 사용 중인 계정이면 누구나 읽을 수 있습니다.",
		}
	}
	if doc.Visibility == "LINK" {
		// Sharing by link is now its own thing — a row with a token, an
		// expiry, and a use count — not a visibility setting. A document left
		// on the old value reaches nobody through it, and the links below are
		// the real answer.
		notes = append(notes, "공개 범위가 'LINK'로 되어 있습니다. 링크 공유는 이제 공개 범위가 아니라 개별 링크로 관리되므로, 이 값 자체로는 아무에게도 권한이 가지 않습니다. 실제 공유는 아래 '공개 링크' 목록입니다.")
	}
	if len(links) > 0 {
		notes = append(notes, "살아 있는 공개 링크가 있습니다. 이 링크를 가진 사람은 계정 없이 문서를 읽을 수 있고, muni는 그 사람이 누구인지 알지 못합니다.")
	}
	if doc.Deleted != nil {
		notes = append(notes, "휴지통에 있는 문서입니다. 목록에는 보이지 않지만 위 권한은 그대로입니다.")
	}
	for _, entry := range entries {
		if entry["suspended"] == true {
			notes = append(notes, "정지된 계정에도 권한이 남아 있습니다. 계정이 다시 열리면 그대로 접근합니다.")
			break
		}
	}

	writeData(w, 200, map[string]any{
		"document": map[string]any{
			"id": id, "title": doc.Title, "workspace": doc.Workspace,
			"visibility": doc.Visibility, "ownerId": doc.OwnerID, "ownerName": doc.OwnerName,
			"trashed": doc.Deleted != nil,
		},
		"entries":  entries,
		"everyone": everyone,
		"admins":   admins,
		"links":    links,
		"notes":    notes,
	})
}
