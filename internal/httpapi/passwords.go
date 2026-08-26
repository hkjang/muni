package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/hkjang/muni/internal/database"
)

// minPasswordLength matches the rule the first administrator's password has to
// meet at boot. One rule, applied everywhere a password is set.
const minPasswordLength = 12

// checkPassword refuses what should not become a password.
//
// Length is the only requirement, deliberately: complexity rules push people
// towards short passwords with a digit stuck on the end, and this is a
// document platform, not a bank.
func checkPassword(value string) error {
	if utf8.RuneCountInString(value) < minPasswordLength {
		return errors.New("비밀번호는 12자 이상이어야 합니다")
	}
	if utf8.RuneCountInString(value) > 256 {
		return errors.New("비밀번호는 256자 이하여야 합니다")
	}
	if strings.TrimSpace(value) == "" {
		return errors.New("비밀번호에 공백만 넣을 수는 없습니다")
	}
	return nil
}

// changeOwnPassword lets a person change their own password.
//
// There was no way to. A local account kept whatever password it was created
// with for as long as it existed, which is not a policy anyone would choose —
// it just never got written.
func (s *Server) changeOwnPassword(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	var input struct {
		Current string `json:"currentPassword"`
		Next    string `json:"newPassword"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}

	var stored *string
	if err := s.db.QueryRow(r.Context(), `SELECT password_hash FROM users WHERE id=$1`, p.User.ID).Scan(&stored); err != nil {
		writeError(w, 404, "USER_NOT_FOUND", "사용자를 찾을 수 없습니다.")
		return
	}
	if stored == nil || *stored == "" {
		// An account that signs in through the identity provider has no
		// password here to change, and inventing one would be a second way in
		// that nobody asked for.
		writeError(w, 409, "NO_LOCAL_PASSWORD", "SSO로 로그인하는 계정입니다. 비밀번호는 인증 제공자에서 변경하세요.")
		return
	}
	if !database.VerifyPassword(*stored, input.Current) {
		writeError(w, 403, "CURRENT_PASSWORD_INVALID", "현재 비밀번호가 일치하지 않습니다.")
		return
	}
	if err := checkPassword(input.Next); err != nil {
		writeError(w, 400, "INVALID_PASSWORD", err.Error())
		return
	}
	if input.Current == input.Next {
		writeError(w, 400, "PASSWORD_UNCHANGED", "이전과 다른 비밀번호를 입력해 주세요.")
		return
	}

	hash, err := database.HashPassword(input.Next)
	if err != nil {
		writeError(w, 500, "HASH_FAILED", "비밀번호를 저장하지 못했습니다.")
		return
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE users SET password_hash=$2, updated_at=now() WHERE id=$1`, p.User.ID, hash); err != nil {
		writeError(w, 500, "DATABASE_ERROR", "비밀번호를 저장하지 못했습니다.")
		return
	}
	// Every other session was opened with the old password. This one is left
	// alone so changing a password does not sign you out of the page you are
	// standing on.
	revoked := int64(0)
	if tag, err := s.db.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1 AND token_hash<>$2`, p.User.ID, p.SessionHash); err == nil {
		revoked = tag.RowsAffected()
	}
	s.audit(r, &p.User.ID, "CHANGE_PASSWORD", "USER", &p.User.ID, map[string]any{"otherSessionsEnded": revoked})
	writeData(w, 200, map[string]any{"ok": true, "otherSessionsEnded": revoked})
}

// resetUserPassword lets an administrator set someone's password.
//
// This is the "locked out on a Monday morning" path, and without it the answer
// was to write an argon2 hash into the database by hand.
func (s *Server) resetUserPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if err := checkPassword(input.Password); err != nil {
		writeError(w, 400, "INVALID_PASSWORD", err.Error())
		return
	}
	hash, err := database.HashPassword(input.Password)
	if err != nil {
		writeError(w, 500, "HASH_FAILED", "비밀번호를 저장하지 못했습니다.")
		return
	}
	result, err := s.db.Exec(r.Context(), `UPDATE users SET password_hash=$2, updated_at=now() WHERE id=$1`, id, hash)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, 404, "USER_NOT_FOUND", "사용자를 찾을 수 없습니다.")
		return
	}
	// Whoever is holding a session opened with the old password should not
	// keep it: a reset is usually a response to something.
	revoked := int64(0)
	if tag, err := s.db.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, id); err == nil {
		revoked = tag.RowsAffected()
	}
	p, _ := principalFrom(r.Context())
	s.audit(r, &p.User.ID, "RESET_PASSWORD", "USER", &id, map[string]any{"sessionsEnded": revoked})
	writeData(w, 200, map[string]any{"ok": true, "sessionsEnded": revoked})
}
