package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/cryptoutil"
	"github.com/hkjang/muni/internal/database"
	"github.com/jackc/pgx/v5"
)

// Public share links.
//
// Everything here runs for someone muni has never met, so the rules are
// deliberately narrow:
//
//   - Read only. An anonymous editor would produce changes with nobody to
//     attribute them to.
//   - The token is never stored. A prefix locates the row and a hash proves
//     the rest, so a copy of the database is not a set of working links.
//   - The administrator setting actually governs it, in both directions. With
//     link sharing off, no link can be made and no existing link opens — a
//     switch that only stops new ones is not the switch an administrator
//     thinks they are throwing.
//   - Nothing about the workspace, the owner, or the other documents leaves
//     through this door. The recipient gets the document and nothing else.

const linkTokenPrefixLength = 12

// linkAttempts limits password guessing against a protected link. The token
// itself has enough entropy that guessing it is not a concern; a four-digit
// password somebody chose is a different matter.
type documentLink struct {
	ID           uuid.UUID
	DocumentID   uuid.UUID
	Role         string
	PasswordHash *string
	ExpiresAt    *time.Time
	MaxViews     *int
	ViewCount    int64
	RevokedAt    *time.Time
}

func (s *Server) createDocumentLink(w http.ResponseWriter, r *http.Request) {
	documentID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	role, err := s.documentRole(r.Context(), p.User, documentID, false)
	if !documentAllowed(w, role, err, "OWNER") {
		return
	}
	all, err := s.settings.GetAll(r.Context(), true)
	if err != nil {
		writeError(w, 500, "SETTINGS_UNAVAILABLE", "설정을 읽지 못했습니다.")
		return
	}
	if !all.Security.AllowPublicLinks {
		writeError(w, 403, "PUBLIC_LINK_DISABLED", "관리자 정책에서 링크 공유가 비활성화되어 있습니다.")
		return
	}

	var input struct {
		Label     string     `json:"label"`
		Password  string     `json:"password"`
		ExpiresAt *time.Time `json:"expiresAt"`
		MaxViews  *int       `json:"maxViews"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if len([]rune(input.Label)) > 100 {
		writeError(w, 400, "INVALID_LABEL", "이름은 100자 이하여야 합니다.")
		return
	}
	if input.ExpiresAt != nil && input.ExpiresAt.Before(time.Now()) {
		// Making a link that is already expired is always a mistake, and a
		// silent one — it looks created and never works.
		writeError(w, 400, "INVALID_EXPIRY", "만료 시각이 이미 지났습니다.")
		return
	}
	if input.MaxViews != nil && (*input.MaxViews < 1 || *input.MaxViews > 100000) {
		writeError(w, 400, "INVALID_MAX_VIEWS", "열람 횟수는 1~100000 사이여야 합니다.")
		return
	}
	var passwordHash *string
	if input.Password != "" {
		// Deliberately not the account password policy: this is a shared
		// secret typed once by someone outside the organisation, and demanding
		// twelve characters gets it written in the same email as the link.
		if len([]rune(input.Password)) < 4 {
			writeError(w, 400, "WEAK_LINK_PASSWORD", "링크 비밀번호는 4자 이상이어야 합니다.")
			return
		}
		hash, err := database.HashPassword(input.Password)
		if err != nil {
			writeError(w, 500, "HASH_FAILED", "비밀번호를 저장하지 못했습니다.")
			return
		}
		passwordHash = &hash
	}

	token, err := cryptoutil.RandomToken(32)
	if err != nil {
		writeError(w, 500, "TOKEN_FAILED", "링크를 만들지 못했습니다.")
		return
	}
	prefix := token[:linkTokenPrefixLength]

	var id uuid.UUID
	err = s.db.QueryRow(r.Context(), `
		INSERT INTO document_links(document_id,token_prefix,token_hash,label,password_hash,expires_at,max_views,created_by)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		documentID, prefix, cryptoutil.SHA256(token), strings.TrimSpace(input.Label),
		passwordHash, input.ExpiresAt, input.MaxViews, p.User.ID).Scan(&id)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "링크를 저장하지 못했습니다.")
		return
	}

	s.audit(r, &p.User.ID, "CREATE_DOCUMENT_LINK", "DOCUMENT", &documentID, map[string]any{
		"linkId": id, "hasPassword": passwordHash != nil,
		"expiresAt": input.ExpiresAt, "maxViews": input.MaxViews,
	})
	// The token is returned here and nowhere else. It is not stored, so no
	// later request can produce it again.
	writeData(w, 201, map[string]any{
		"id": id, "token": token, "path": "/s/" + token,
		"expiresAt": input.ExpiresAt, "maxViews": input.MaxViews,
		"hasPassword": passwordHash != nil,
	})
}

func (s *Server) listDocumentLinks(w http.ResponseWriter, r *http.Request) {
	documentID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	role, err := s.documentRole(r.Context(), p.User, documentID, false)
	if !documentAllowed(w, role, err, "OWNER") {
		return
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT l.id, l.label, l.token_prefix, l.password_hash IS NOT NULL, l.expires_at,
			l.max_views, l.view_count, l.last_viewed_at, l.revoked_at, l.created_at, u.display_name
		FROM document_links l JOIN users u ON u.id = l.created_by
		WHERE l.document_id = $1 ORDER BY l.created_at DESC`, documentID)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "링크를 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id uuid.UUID
		var label, prefix, creator string
		var hasPassword bool
		var expires, lastViewed, revoked, created *time.Time
		var maxViews *int
		var views int64
		if rows.Scan(&id, &label, &prefix, &hasPassword, &expires, &maxViews,
			&views, &lastViewed, &revoked, &created, &creator) != nil {
			continue
		}
		items = append(items, map[string]any{
			"id": id, "label": label,
			// Enough to tell two links apart on screen, not enough to use.
			"prefix": prefix, "hasPassword": hasPassword,
			"expiresAt": expires, "maxViews": maxViews, "viewCount": views,
			"lastViewedAt": lastViewed, "revokedAt": revoked,
			"createdAt": created, "createdBy": creator,
			"active": linkIsUsable(revoked, expires, maxViews, views),
		})
	}
	writeData(w, 200, items)
}

func (s *Server) revokeDocumentLink(w http.ResponseWriter, r *http.Request) {
	documentID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	linkID, ok := pathUUID(w, r, "linkId")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	role, err := s.documentRole(r.Context(), p.User, documentID, false)
	if !documentAllowed(w, role, err, "OWNER") {
		return
	}
	// Revoked rather than deleted: the row is the record that this document
	// was shared outside, how often it was opened, and when it stopped.
	result, err := s.db.Exec(r.Context(),
		`UPDATE document_links SET revoked_at=now() WHERE id=$1 AND document_id=$2 AND revoked_at IS NULL`,
		linkID, documentID)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, 404, "LINK_NOT_FOUND", "링크를 찾을 수 없거나 이미 해지되었습니다.")
		return
	}
	s.audit(r, &p.User.ID, "REVOKE_DOCUMENT_LINK", "DOCUMENT", &documentID, map[string]any{"linkId": linkID})
	writeData(w, 200, map[string]any{"ok": true})
}

func linkIsUsable(revoked, expires *time.Time, maxViews *int, views int64) bool {
	if revoked != nil {
		return false
	}
	if expires != nil && expires.Before(time.Now()) {
		return false
	}
	if maxViews != nil && views >= int64(*maxViews) {
		return false
	}
	return true
}

var errLinkPasswordRequired = errors.New("link password required")

// openPublicDocument serves a document to whoever has the link.
//
// It is a POST because the password travels in the body. In a URL it would sit
// in browser history, in any proxy log along the way, and in the Referer
// header of every image the page loads.
func (s *Server) openPublicDocument(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if len(token) <= linkTokenPrefixLength {
		writeError(w, 404, "LINK_NOT_FOUND", "링크를 찾을 수 없습니다.")
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	// The body is optional — the first request usually has none — so a decode
	// failure is not an error here. It just means no password was offered.
	_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&input)

	all, err := s.settings.GetAll(r.Context(), true)
	if err != nil {
		writeError(w, 500, "SETTINGS_UNAVAILABLE", "설정을 읽지 못했습니다.")
		return
	}
	if !all.Security.AllowPublicLinks {
		// Turning the policy off has to close the links that already exist.
		// A switch that only stops new ones is not what an administrator
		// reaches for when a document has gone somewhere it should not.
		writeError(w, 403, "PUBLIC_LINK_DISABLED", "이 서비스에서 링크 공유가 중지되었습니다.")
		return
	}

	key := loginKey(r, token[:linkTokenPrefixLength])
	if s.logins.blocked(key) {
		writeError(w, 429, "TOO_MANY_ATTEMPTS", "시도가 너무 많습니다. 잠시 후 다시 시도해 주세요.")
		return
	}

	var link documentLink
	var storedHash []byte
	var title, content string
	var updatedAt time.Time
	var deletedAt *time.Time
	err = s.db.QueryRow(r.Context(), `
		SELECT l.id, l.document_id, l.role, l.password_hash, l.expires_at, l.max_views,
			l.view_count, l.revoked_at, l.token_hash, d.title, d.content_json::text, d.updated_at, d.deleted_at
		FROM document_links l JOIN documents d ON d.id = l.document_id
		WHERE l.token_prefix = $1`, token[:linkTokenPrefixLength]).
		Scan(&link.ID, &link.DocumentID, &link.Role, &link.PasswordHash, &link.ExpiresAt,
			&link.MaxViews, &link.ViewCount, &link.RevokedAt, &storedHash, &title, &content, &updatedAt, &deletedAt)
	if err != nil {
		// Whether the prefix matched nothing or the rest of the token was
		// wrong is not something the answer should distinguish.
		s.logins.fail(key)
		writeError(w, 404, "LINK_NOT_FOUND", "링크를 찾을 수 없습니다.")
		return
	}
	if subtle.ConstantTimeCompare(storedHash, cryptoutil.SHA256(token)) != 1 {
		s.logins.fail(key)
		writeError(w, 404, "LINK_NOT_FOUND", "링크를 찾을 수 없습니다.")
		return
	}
	if deletedAt != nil {
		writeError(w, 404, "LINK_NOT_FOUND", "링크를 찾을 수 없습니다.")
		return
	}
	if !linkIsUsable(link.RevokedAt, link.ExpiresAt, link.MaxViews, link.ViewCount) {
		writeError(w, 410, "LINK_EXPIRED", "이 링크는 더 이상 사용할 수 없습니다.")
		return
	}
	if link.PasswordHash != nil {
		if input.Password == "" {
			writeError(w, 401, "LINK_PASSWORD_REQUIRED", "비밀번호가 필요한 링크입니다.")
			return
		}
		if !database.VerifyPassword(*link.PasswordHash, input.Password) {
			s.logins.fail(key)
			writeError(w, 401, "LINK_PASSWORD_INVALID", "비밀번호가 올바르지 않습니다.")
			return
		}
	}
	s.logins.succeed(key)

	// Counting the view and enforcing the ceiling happen in the same statement,
	// so two people opening a last-view link at once cannot both get through.
	var counted int64
	err = s.db.QueryRow(r.Context(), `
		UPDATE document_links SET view_count = view_count + 1, last_viewed_at = now()
		WHERE id = $1 AND (max_views IS NULL OR view_count < max_views)
		RETURNING view_count`, link.ID).Scan(&counted)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 410, "LINK_EXPIRED", "이 링크는 더 이상 사용할 수 없습니다.")
		return
	}
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "문서를 열지 못했습니다.")
		return
	}

	// Recorded against the document, with no actor: nobody signed in. The
	// address is what the audit log has to work with.
	s.audit(r, nil, "VIEW_DOCUMENT_LINK", "DOCUMENT", &link.DocumentID,
		map[string]any{"linkId": link.ID, "viewCount": counted})

	service := strings.TrimSpace(all.General.ServiceName)
	if service == "" {
		service = "muni"
	}
	// Deliberately narrow. No workspace, no owner, no neighbouring documents,
	// no identity of anyone inside the organisation.
	writeData(w, 200, map[string]any{
		"title": title, "content": string(liftStoredImages(json.RawMessage(content))), "updatedAt": updatedAt,
		"role": link.Role, "serviceName": service,
	})
}
