package httpapi

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/cryptoutil"
	"github.com/hkjang/muni/internal/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var allowedAPIScopes = []string{"api:read", "api:write", "mcp:read", "mcp:write", "ai:use"}

type keyRotationInput struct {
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

func (s *Server) hasKeyPermission(r *http.Request, p principal, permission string) bool {
	var allowed bool
	alternate := permission
	if strings.HasSuffix(permission, ":own") {
		alternate = strings.TrimSuffix(permission, ":own") + ":any"
	}
	_ = s.db.QueryRow(r.Context(), `SELECT COALESCE((permissions ? $2) OR (permissions ? $3),false) FROM key_role_policies WHERE role=$1`, p.User.Role, permission, alternate).Scan(&allowed)
	return allowed
}

func (s *Server) listUserKeys(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	if !s.hasKeyPermission(r, p, "key:read:own") {
		writeError(w, 403, "KEY_PERMISSION_DENIED", "개인 키를 조회할 권한이 없습니다.")
		return
	}
	s.writeUserKeys(w, r, p.User.ID)
}

func (s *Server) writeUserKeys(w http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	rows, err := s.db.Query(r.Context(), `SELECT id,name,fingerprint,status,version,rotated_from,expires_at,created_at,retired_at FROM user_keys WHERE user_id=$1 ORDER BY version DESC`, userID)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "개인 키를 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id uuid.UUID
		var name, fingerprint, status string
		var version int
		var from *uuid.UUID
		var expires, created, retired *time.Time
		if rows.Scan(&id, &name, &fingerprint, &status, &version, &from, &expires, &created, &retired) == nil {
			items = append(items, map[string]any{"id": id, "name": name, "fingerprint": fingerprint, "status": status, "version": version, "rotatedFrom": from, "expiresAt": expires, "createdAt": created, "retiredAt": retired})
		}
	}
	writeData(w, 200, items)
}

func (s *Server) rotateUserKey(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	if !s.hasKeyPermission(r, p, "key:rotate:own") {
		writeError(w, 403, "KEY_PERMISSION_DENIED", "개인 키를 회전할 권한이 없습니다.")
		return
	}
	var input keyRotationInput
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		input.Name = "개인 키"
	}
	if len([]rune(input.Name)) > 100 {
		writeError(w, 400, "INVALID_KEY_NAME", "키 이름은 100자 이하여야 합니다.")
		return
	}
	newID, fingerprint, err := s.rotateKey(r, p.User.ID, input)
	if err != nil {
		writeError(w, 500, "KEY_ROTATION_FAILED", "개인 키를 회전하지 못했습니다.")
		return
	}
	s.audit(r, &p.User.ID, "ROTATE_PERSONAL_KEY", "USER_KEY", &newID, map[string]any{"fingerprint": fingerprint})
	writeData(w, 201, map[string]any{"id": newID, "name": input.Name, "fingerprint": fingerprint, "status": "ACTIVE"})
}

func (s *Server) revokeUserKey(w http.ResponseWriter, r *http.Request) {
	keyID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	if !s.hasKeyPermission(r, p, "key:revoke:own") {
		writeError(w, 403, "KEY_PERMISSION_DENIED", "개인 키를 폐기할 권한이 없습니다.")
		return
	}
	result, err := s.revokeKey(r, p.User.ID, keyID)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, 409, "KEY_REVOKE_FAILED", "활성 키는 먼저 회전해야 하며, 과거 키만 폐기할 수 있습니다.")
		return
	}
	s.audit(r, &p.User.ID, "REVOKE_PERSONAL_KEY", "USER_KEY", &keyID, nil)
	w.WriteHeader(204)
}

func (s *Server) listAnyUserKeys(w http.ResponseWriter, r *http.Request) {
	targetID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	if !s.hasKeyPermission(r, p, "key:read:any") {
		writeError(w, 403, "KEY_PERMISSION_DENIED", "다른 사용자의 키를 조회할 권한이 없습니다.")
		return
	}
	s.writeUserKeys(w, r, targetID)
}

func (s *Server) rotateAnyUserKey(w http.ResponseWriter, r *http.Request) {
	targetID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	if !s.hasKeyPermission(r, p, "key:rotate:any") {
		writeError(w, 403, "KEY_PERMISSION_DENIED", "다른 사용자의 키를 회전할 권한이 없습니다.")
		return
	}
	var input keyRotationInput
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		input.Name = "관리자 회전 키"
	}
	if len([]rune(input.Name)) > 100 {
		writeError(w, 400, "INVALID_KEY_NAME", "키 이름은 100자 이하여야 합니다.")
		return
	}
	newID, fingerprint, err := s.rotateKey(r, targetID, input)
	if err != nil {
		writeError(w, 409, "KEY_ROTATION_FAILED", "대상 사용자의 개인 키를 회전하지 못했습니다.")
		return
	}
	s.audit(r, &p.User.ID, "ADMIN_ROTATE_PERSONAL_KEY", "USER_KEY", &newID, map[string]any{"targetUserId": targetID, "fingerprint": fingerprint})
	writeData(w, 201, map[string]any{"id": newID, "name": input.Name, "fingerprint": fingerprint, "status": "ACTIVE"})
}

func (s *Server) revokeAnyUserKey(w http.ResponseWriter, r *http.Request) {
	targetID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	keyID, ok := pathUUID(w, r, "keyId")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	if !s.hasKeyPermission(r, p, "key:revoke:any") {
		writeError(w, 403, "KEY_PERMISSION_DENIED", "다른 사용자의 과거 키를 폐기할 권한이 없습니다.")
		return
	}
	result, err := s.revokeKey(r, targetID, keyID)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, 409, "KEY_REVOKE_FAILED", "대상 사용자의 RETIRED 키만 폐기할 수 있습니다.")
		return
	}
	s.audit(r, &p.User.ID, "ADMIN_REVOKE_PERSONAL_KEY", "USER_KEY", &keyID, map[string]any{"targetUserId": targetID})
	w.WriteHeader(204)
}

func (s *Server) rotateKey(r *http.Request, userID uuid.UUID, input keyRotationInput) (uuid.UUID, string, error) {
	raw, err := cryptoutil.RandomBytes(32)
	if err != nil {
		return uuid.Nil, "", err
	}
	newID := uuid.New()
	var fingerprint string
	err = database.WithTx(r.Context(), s.db, func(tx pgx.Tx) error {
		var oldID uuid.UUID
		var version int
		if err := tx.QueryRow(r.Context(), `SELECT id,version FROM user_keys WHERE user_id=$1 AND status='ACTIVE' FOR UPDATE`, userID).Scan(&oldID, &version); err != nil {
			return err
		}
		next := version + 1
		wrapped, err := s.sealer.Seal(raw, "user-key:"+userID.String()+":"+stringInt(next))
		if err != nil {
			return err
		}
		fingerprint = cryptoutil.Fingerprint(raw)
		if _, err = tx.Exec(r.Context(), `UPDATE user_keys SET status='RETIRED',retired_at=now() WHERE id=$1`, oldID); err != nil {
			return err
		}
		_, err = tx.Exec(r.Context(), `INSERT INTO user_keys(id,user_id,name,fingerprint,wrapped_key,status,version,rotated_from,expires_at) VALUES($1,$2,$3,$4,$5,'ACTIVE',$6,$7,$8)`, newID, userID, input.Name, fingerprint, wrapped, next, oldID, input.ExpiresAt)
		return err
	})
	return newID, fingerprint, err
}

func (s *Server) revokeKey(r *http.Request, userID, keyID uuid.UUID) (pgconn.CommandTag, error) {
	return s.db.Exec(r.Context(), `UPDATE user_keys SET status='REVOKED',retired_at=COALESCE(retired_at,now()) WHERE id=$1 AND user_id=$2 AND status='RETIRED'`, keyID, userID)
}

func (s *Server) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	rows, err := s.db.Query(r.Context(), `SELECT id,name,prefix,scopes,expires_at,last_used_at,created_at,revoked_at FROM api_keys WHERE user_id=$1 ORDER BY created_at DESC`, p.User.ID)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "API 키를 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id uuid.UUID
		var name, prefix string
		var scopes []string
		var expires, lastUsed, created, revoked *time.Time
		if rows.Scan(&id, &name, &prefix, &scopes, &expires, &lastUsed, &created, &revoked) == nil {
			items = append(items, map[string]any{"id": id, "name": name, "prefix": prefix, "scopes": scopes, "expiresAt": expires, "lastUsedAt": lastUsed, "createdAt": created, "revokedAt": revoked})
		}
	}
	writeData(w, 200, items)
}

func (s *Server) createAPIKey(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	var input struct {
		Name      string     `json:"name"`
		Scopes    []string   `json:"scopes"`
		ExpiresAt *time.Time `json:"expiresAt"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len([]rune(input.Name)) > 100 {
		writeError(w, 400, "INVALID_KEY_NAME", "API 키 이름은 1~100자여야 합니다.")
		return
	}
	if len(input.Scopes) == 0 {
		input.Scopes = []string{"api:read"}
	}
	seen := map[string]bool{}
	for _, scope := range input.Scopes {
		if !contains(allowedAPIScopes, scope) || seen[scope] {
			writeError(w, 400, "INVALID_API_SCOPE", "API 키 범위를 확인해 주세요.")
			return
		}
		seen[scope] = true
	}
	all, _ := s.settings.GetAll(r.Context(), false)
	maxExpiry := time.Now().Add(time.Duration(all.Security.APIKeyMaxDays) * 24 * time.Hour)
	if input.ExpiresAt == nil {
		input.ExpiresAt = &maxExpiry
	} else if input.ExpiresAt.After(maxExpiry) {
		writeError(w, 400, "API_KEY_EXPIRY_TOO_LONG", "관리자 정책의 최대 API 키 수명을 초과했습니다.")
		return
	}
	prefixBytes, err := cryptoutil.RandomBytes(6)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "API_KEY_CREATE_FAILED", "API 키를 만들지 못했습니다.")
		return
	}
	prefixRaw := hex.EncodeToString(prefixBytes)
	secret, _ := cryptoutil.RandomToken(32)
	prefix := "muni_" + prefixRaw
	token := prefix + "_" + secret
	id := uuid.New()
	_, err = s.db.Exec(r.Context(), `INSERT INTO api_keys(id,user_id,name,prefix,secret_hash,scopes,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, p.User.ID, input.Name, prefix, cryptoutil.SHA256(token), input.Scopes, input.ExpiresAt)
	if err != nil {
		writeError(w, 500, "API_KEY_CREATE_FAILED", "API 키를 만들지 못했습니다.")
		return
	}
	s.audit(r, &p.User.ID, "CREATE_API_KEY", "API_KEY", &id, map[string]any{"scopes": input.Scopes})
	writeData(w, 201, map[string]any{"id": id, "name": input.Name, "token": token, "prefix": prefix, "scopes": input.Scopes, "expiresAt": input.ExpiresAt, "warning": "이 키는 다시 표시되지 않습니다."})
}
func (s *Server) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	result, err := s.db.Exec(r.Context(), `UPDATE api_keys SET revoked_at=now() WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`, id, p.User.ID)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, 404, "API_KEY_NOT_FOUND", "API 키를 찾을 수 없습니다.")
		return
	}
	s.audit(r, &p.User.ID, "REVOKE_API_KEY", "API_KEY", &id, nil)
	w.WriteHeader(204)
}

func (s *Server) listKeyPolicies(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	if !s.hasKeyPermission(r, p, "policy:manage") {
		writeError(w, 403, "KEY_PERMISSION_DENIED", "키 정책을 관리할 권한이 없습니다.")
		return
	}
	rows, err := s.db.Query(r.Context(), `SELECT role,permissions,updated_at FROM key_role_policies ORDER BY role`)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "키 권한 정책을 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var role string
		var permissions json.RawMessage
		var updated time.Time
		if rows.Scan(&role, &permissions, &updated) == nil {
			items = append(items, map[string]any{"role": role, "permissions": permissions, "updatedAt": updated})
		}
	}
	writeData(w, 200, items)
}
func (s *Server) updateKeyPolicy(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	if !s.hasKeyPermission(r, p, "policy:manage") {
		writeError(w, 403, "KEY_PERMISSION_DENIED", "키 정책을 관리할 권한이 없습니다.")
		return
	}
	role := strings.ToUpper(r.PathValue("role"))
	if role != "ADMIN" && role != "USER" {
		writeError(w, 400, "INVALID_ROLE", "지원하지 않는 역할입니다.")
		return
	}
	var input struct {
		Permissions []string `json:"permissions"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	valid := []string{"key:read:own", "key:rotate:own", "key:revoke:own", "key:read:any", "key:rotate:any", "key:revoke:any", "policy:manage"}
	for _, permission := range input.Permissions {
		if !contains(valid, permission) {
			writeError(w, 400, "INVALID_KEY_PERMISSION", "키 권한 값을 확인해 주세요.")
			return
		}
	}
	if role == "ADMIN" && !contains(input.Permissions, "policy:manage") {
		writeError(w, 409, "POLICY_LOCKOUT", "ADMIN 역할의 policy:manage 권한은 제거할 수 없습니다.")
		return
	}
	encoded, _ := json.Marshal(input.Permissions)
	_, err := s.db.Exec(r.Context(), `INSERT INTO key_role_policies(role,permissions,updated_by,updated_at) VALUES($1,$2,$3,now()) ON CONFLICT(role) DO UPDATE SET permissions=excluded.permissions,updated_by=excluded.updated_by,updated_at=now()`, role, encoded, p.User.ID)
	if err != nil {
		writeError(w, 500, "POLICY_SAVE_FAILED", "키 권한 정책을 저장하지 못했습니다.")
		return
	}
	s.audit(r, &p.User.ID, "UPDATE_KEY_POLICY", "KEY_POLICY", nil, map[string]any{"role": role, "permissions": input.Permissions})
	writeData(w, 200, map[string]any{"role": role, "permissions": input.Permissions})
}
func stringInt(value int) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	for value > 0 {
		buf = append([]byte{digits[value%10]}, buf...)
		value /= 10
	}
	return string(buf)
}
