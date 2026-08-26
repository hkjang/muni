package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/database"
	"github.com/jackc/pgx/v5"
)

// adminListDocuments finds a document anywhere in the system.
//
// Every other way of reaching a document goes through a permission check, so
// the questions an operator is handed — where did this go, who owns the thing
// nobody can open, what is sitting in the trash — had no screen that could
// answer them.
func (s *Server) adminListDocuments(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit := parseLimit(query.Get("limit"), 50)
	search := strings.TrimSpace(query.Get("q"))
	scope := strings.ToLower(strings.TrimSpace(query.Get("scope")))

	var workspace *uuid.UUID
	if parsed, err := uuid.Parse(strings.TrimSpace(query.Get("workspaceId"))); err == nil {
		workspace = &parsed
	}

	// The three states an operator asks about, and only those.
	trashed := "IS NULL"
	switch scope {
	case "trashed":
		trashed = "IS NOT NULL"
	case "all":
		trashed = "IS NOT NULL OR d.deleted_at IS NULL"
	}

	rows, err := s.db.Query(r.Context(), `
		SELECT d.id, d.title, d.workspace_id, w.name, d.owner_id, u.display_name,
			d.status, d.workflow_status, d.revision_no, d.updated_at, d.deleted_at
		FROM documents d
		JOIN workspaces w ON w.id = d.workspace_id
		JOIN users u ON u.id = d.owner_id
		WHERE (d.deleted_at `+trashed+`)
			AND ($1 = '' OR d.title ILIKE '%'||$1||'%' OR u.display_name ILIKE '%'||$1||'%')
			AND ($2::uuid IS NULL OR d.workspace_id = $2)
		ORDER BY d.updated_at DESC LIMIT $3`, search, workspace, limit)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "문서 목록을 불러오지 못했습니다.")
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0, limit)
	for rows.Next() {
		var id, workspaceID, ownerID uuid.UUID
		var title, workspaceName, owner, status, workflow string
		var revision int
		var updated time.Time
		var deleted *time.Time
		if rows.Scan(&id, &title, &workspaceID, &workspaceName, &ownerID, &owner,
			&status, &workflow, &revision, &updated, &deleted) == nil {
			items = append(items, map[string]any{
				"id": id, "title": title,
				"workspaceId": workspaceID, "workspaceName": workspaceName,
				"ownerId": ownerID, "ownerName": owner,
				"status": status, "workflowStatus": workflow,
				"revision": revision, "updatedAt": updated, "deletedAt": deleted,
			})
		}
	}
	writeData(w, 200, items)
}

// adminTransferDocument moves a document to another owner.
//
// An owner who leaves takes their documents with them: nobody else can share,
// delete or restore them. Transferring also makes the new owner a member of
// the workspace when they are not one already — an owner who cannot see the
// document in their own list has not really been given it.
func (s *Server) adminTransferDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var input struct {
		OwnerID uuid.UUID `json:"ownerId"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.OwnerID == uuid.Nil {
		writeError(w, 400, "OWNER_REQUIRED", "새 소유자를 선택해 주세요.")
		return
	}
	p, _ := principalFrom(r.Context())

	err := database.WithTx(r.Context(), s.db, func(tx pgx.Tx) error {
		var status string
		if err := tx.QueryRow(r.Context(), `SELECT status FROM users WHERE id=$1`, input.OwnerID).Scan(&status); err != nil {
			return err
		}
		if status != "ACTIVE" {
			return errInactiveOwner
		}
		var workspaceID uuid.UUID
		if err := tx.QueryRow(r.Context(),
			`UPDATE documents SET owner_id=$2, updated_at=now() WHERE id=$1 RETURNING workspace_id`,
			id, input.OwnerID).Scan(&workspaceID); err != nil {
			return err
		}
		_, err := tx.Exec(r.Context(),
			`INSERT INTO workspace_members(workspace_id,user_id,role) VALUES($1,$2,'MEMBER')
			 ON CONFLICT (workspace_id,user_id) DO NOTHING`, workspaceID, input.OwnerID)
		return err
	})
	if err == errInactiveOwner {
		writeError(w, 409, "OWNER_INACTIVE", "활성 상태인 사용자에게만 넘길 수 있습니다.")
		return
	}
	if err != nil {
		writeError(w, 404, "DOCUMENT_NOT_FOUND", "문서 또는 사용자를 찾을 수 없습니다.")
		return
	}
	// Whoever has it open is holding the old permissions.
	s.hub.CloseDocument(id)
	s.audit(r, &p.User.ID, "TRANSFER_DOCUMENT", "DOCUMENT", &id,
		map[string]any{"ownerId": input.OwnerID})
	writeData(w, 200, map[string]any{"id": id, "ownerId": input.OwnerID})
}

var errInactiveOwner = &inactiveOwnerError{}

type inactiveOwnerError struct{}

func (e *inactiveOwnerError) Error() string { return "owner is not active" }

// adminPurgeDocument deletes a document and everything attached to it, with no
// way back.
//
// Only a document already in the trash can be purged. Emptying the trash is
// something an operator is asked to do — a document that had to go, a
// workspace being wound up — and doing it in the database by hand is how
// referenced rows get left behind.
func (s *Server) adminPurgeDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())

	var title string
	var deletedAt *time.Time
	if err := s.db.QueryRow(r.Context(), `SELECT title,deleted_at FROM documents WHERE id=$1`, id).Scan(&title, &deletedAt); err != nil {
		writeError(w, 404, "DOCUMENT_NOT_FOUND", "문서를 찾을 수 없습니다.")
		return
	}
	if deletedAt == nil {
		// Purging straight from the working set would make an accidental click
		// unrecoverable; the trash is the confirmation step.
		writeError(w, 409, "DOCUMENT_NOT_TRASHED", "휴지통에 있는 문서만 완전히 삭제할 수 있습니다.")
		return
	}

	s.hub.CloseDocument(id)
	// Revisions, comments, suggestions, attachments and permissions are all
	// declared ON DELETE CASCADE, so the row is the whole job.
	if _, err := s.db.Exec(r.Context(), `DELETE FROM documents WHERE id=$1`, id); err != nil {
		writeError(w, 500, "DELETE_FAILED", "문서를 삭제하지 못했습니다.")
		return
	}
	s.audit(r, &p.User.ID, "PURGE_DOCUMENT", "DOCUMENT", &id, map[string]any{"title": title})
	w.WriteHeader(204)
}

// adminUserSessions lists where a person is signed in.
func (s *Server) adminUserSessions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT created_at, last_seen_at, expires_at, ip, user_agent
		FROM sessions WHERE user_id=$1 AND expires_at > now()
		ORDER BY last_seen_at DESC LIMIT 50`, id)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "세션을 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0, 8)
	for rows.Next() {
		var created, lastSeen, expires time.Time
		var ip any
		var agent *string
		if rows.Scan(&created, &lastSeen, &expires, &ip, &agent) == nil {
			items = append(items, map[string]any{
				"createdAt": created, "lastSeenAt": lastSeen, "expiresAt": expires,
				"ip": ip, "userAgent": agent,
			})
		}
	}
	writeData(w, 200, items)
}

// adminRevokeUserSessions signs a person out everywhere.
//
// Suspending an account already stops the next request, because the account is
// read fresh on each one. This is for the other case — a laptop left on a
// train — where the account should stay usable and the sessions should not.
func (s *Server) adminRevokeUserSessions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	tag, err := s.db.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, id)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "세션을 종료하지 못했습니다.")
		return
	}
	s.audit(r, &p.User.ID, "REVOKE_SESSIONS", "USER", &id,
		map[string]any{"sessions": tag.RowsAffected()})
	writeData(w, 200, map[string]any{"revoked": tag.RowsAffected()})
}
