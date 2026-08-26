package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/cryptoutil"
	"github.com/hkjang/muni/internal/database"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"
)

const sessionCookie = "muni_session"

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := s.authenticate(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "로그인이 필요합니다.")
			return
		}
		if p.User.Status != "ACTIVE" {
			writeError(w, http.StatusForbidden, "ACCOUNT_SUSPENDED", "사용할 수 없는 계정입니다.")
			return
		}
		if p.APIKeyID != nil && !scopeAllowsRequest(p.Scopes, r) {
			writeError(w, http.StatusForbidden, "API_SCOPE_DENIED", "API 키 범위가 이 요청을 허용하지 않습니다.")
			return
		}
		if p.APIKeyID == nil && !sameOrigin(r) {
			writeError(w, http.StatusForbidden, "INVALID_ORIGIN", "요청 출처를 확인할 수 없습니다.")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, p)))
	})
}

func scopeAllowsRequest(scopes []string, r *http.Request) bool {
	has := func(scope string) bool {
		for _, item := range scopes {
			if item == scope {
				return true
			}
		}
		return false
	}
	if r.URL.Path == "/mcp" || r.URL.Path == "/api/v1/mcp" {
		if r.Header.Get("Mcp-Method") == "tools/call" || strings.Contains(r.Header.Get("Mcp-Name"), "create") || strings.Contains(r.Header.Get("Mcp-Name"), "update") {
			return has("mcp:write") || has("mcp:read")
		}
		return has("mcp:read") || has("mcp:write")
	}
	if r.URL.Path == "/api/v1/ai/chat" {
		return has("ai:use")
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return has("api:read") || has("api:write")
	}
	return has("api:write")
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := principalFrom(r.Context())
		if p.User.Role != "ADMIN" {
			writeError(w, http.StatusForbidden, "ADMIN_REQUIRED", "서비스 관리자 권한이 필요합니다.")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func sameOrigin(r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return true
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // Non-browser clients use a session only when explicitly supplied.
	}
	parsed, err := url.Parse(origin)
	return err == nil && strings.EqualFold(parsed.Host, r.Host)
}

func (s *Server) authenticate(r *http.Request) (principal, error) {
	authorization := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(authorization), "bearer ") {
		token := strings.TrimSpace(authorization[7:])
		if strings.HasPrefix(token, "muni_") {
			return s.authenticateAPIKey(r.Context(), token)
		}
	}
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		return principal{}, errors.New("session cookie missing")
	}
	return s.authenticateSession(r.Context(), cookie.Value)
}

func (s *Server) authenticateSession(ctx context.Context, token string) (principal, error) {
	var p principal
	err := s.db.QueryRow(ctx, `SELECT u.id,u.username,u.email,u.display_name,u.role,u.status,u.avatar_url,u.locale,u.created_at
		FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now()`, cryptoutil.SHA256(token)).Scan(
		&p.User.ID, &p.User.Username, &p.User.Email, &p.User.DisplayName, &p.User.Role, &p.User.Status, &p.User.AvatarURL, &p.User.Locale, &p.User.CreatedAt)
	if err != nil {
		return principal{}, err
	}
	p.SessionHash = cryptoutil.SHA256(token)
	_, _ = s.db.Exec(ctx, `UPDATE sessions SET last_seen_at=now() WHERE token_hash=$1 AND last_seen_at<now()-interval '5 minutes'`, p.SessionHash)
	return p, nil
}

func (s *Server) authenticateAPIKey(ctx context.Context, token string) (principal, error) {
	const prefixLength = len("muni_") + 12
	if len(token) <= prefixLength || token[prefixLength] != '_' {
		return principal{}, errors.New("malformed API key")
	}
	prefix := token[:prefixLength]
	var p principal
	var expected []byte
	var keyID uuid.UUID
	err := s.db.QueryRow(ctx, `SELECT k.id,k.secret_hash,k.scopes,u.id,u.username,u.email,u.display_name,u.role,u.status,u.avatar_url,u.locale,u.created_at
		FROM api_keys k JOIN users u ON u.id=k.user_id WHERE k.prefix=$1 AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at>now())`, prefix).Scan(
		&keyID, &expected, &p.Scopes, &p.User.ID, &p.User.Username, &p.User.Email, &p.User.DisplayName, &p.User.Role, &p.User.Status, &p.User.AvatarURL, &p.User.Locale, &p.User.CreatedAt)
	if err != nil || subtle.ConstantTimeCompare(expected, cryptoutil.SHA256(token)) != 1 {
		return principal{}, errors.New("invalid API key")
	}
	p.APIKeyID = &keyID
	_, _ = s.db.Exec(ctx, `UPDATE api_keys SET last_used_at=now() WHERE id=$1 AND (last_used_at IS NULL OR last_used_at<now()-interval '5 minutes')`, keyID)
	return p, nil
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	settings, err := s.settings.GetAll(r.Context(), false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SETTINGS_ERROR", "로그인 설정을 불러오지 못했습니다.")
		return
	}
	if !settings.General.AllowLocalLogin {
		writeError(w, http.StatusForbidden, "LOCAL_LOGIN_DISABLED", "로컬 로그인이 비활성화되어 있습니다.")
		return
	}
	var input struct {
		Identity string `json:"identity"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	// Guessing has to cost something. A delay alone is served in parallel, so
	// a hundred attempts sent together all finish together.
	key := loginKey(r, input.Identity)
	if s.logins != nil && s.logins.blocked(key) {
		s.audit(r, nil, "LOGIN_BLOCKED", "USER", nil, map[string]any{"identity": truncate(input.Identity, 80)})
		writeError(w, http.StatusTooManyRequests, "TOO_MANY_ATTEMPTS",
			"로그인 시도가 너무 많습니다. 잠시 후 다시 시도해 주세요.")
		return
	}

	var user User
	var passwordHash *string
	err = s.db.QueryRow(r.Context(), `SELECT id,username,email,display_name,password_hash,role,status,avatar_url,locale,created_at
		FROM users WHERE username=$1 OR email=$1`, strings.ToLower(strings.TrimSpace(input.Identity))).Scan(
		&user.ID, &user.Username, &user.Email, &user.DisplayName, &passwordHash, &user.Role, &user.Status, &user.AvatarURL, &user.Locale, &user.CreatedAt)
	if err != nil || passwordHash == nil || !database.VerifyPassword(*passwordHash, input.Password) {
		if s.logins != nil {
			s.logins.fail(key)
		}
		time.Sleep(200 * time.Millisecond)
		writeError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "아이디 또는 비밀번호가 올바르지 않습니다.")
		return
	}
	if s.logins != nil {
		s.logins.succeed(key)
	}
	if user.Status != "ACTIVE" {
		writeError(w, http.StatusForbidden, "ACCOUNT_SUSPENDED", "사용할 수 없는 계정입니다.")
		return
	}
	if err := s.createSession(w, r, user.ID, settings.Security.SessionHours); err != nil {
		writeError(w, http.StatusInternalServerError, "SESSION_ERROR", "세션을 만들지 못했습니다.")
		return
	}
	_, _ = s.db.Exec(r.Context(), `UPDATE users SET last_login_at=now() WHERE id=$1`, user.ID)
	s.audit(r, &user.ID, "LOGIN_LOCAL", "SESSION", nil, nil)
	writeData(w, http.StatusOK, user)
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request, userID uuid.UUID, hours int) error {
	if hours < 1 {
		hours = 12
	}
	token, err := cryptoutil.RandomToken(32)
	if err != nil {
		return err
	}
	expires := time.Now().Add(time.Duration(hours) * time.Hour)
	_, err = s.db.Exec(r.Context(), `INSERT INTO sessions(token_hash,user_id,expires_at,ip,user_agent) VALUES($1,$2,$3,$4,$5)`,
		cryptoutil.SHA256(token), userID, expires, clientIP(r), truncate(r.UserAgent(), 1000))
	if err != nil {
		return err
	}
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int(time.Until(expires).Seconds())})
	return nil
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		_, _ = s.db.Exec(r.Context(), `DELETE FROM sessions WHERE token_hash=$1`, cryptoutil.SHA256(cookie.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	writeData(w, http.StatusOK, map[string]any{"user": p.User, "build": s.info})
}

func (s *Server) oidcStart(w http.ResponseWriter, r *http.Request) {
	all, err := s.settings.GetAll(r.Context(), true)
	if err != nil || !all.OIDC.Enabled {
		writeError(w, http.StatusNotFound, "OIDC_DISABLED", "SSO 로그인이 설정되지 않았습니다.")
		return
	}
	provider, err := oidc.NewProvider(r.Context(), all.OIDC.IssuerURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, "OIDC_DISCOVERY_FAILED", "SSO 서버에 연결할 수 없습니다.")
		return
	}
	state, _ := cryptoutil.RandomToken(32)
	nonce, _ := cryptoutil.RandomToken(24)
	verifier := oauth2.GenerateVerifier()
	returnTo := safeReturnPath(r.URL.Query().Get("return_to"))
	_, err = s.db.Exec(r.Context(), `INSERT INTO oidc_states(state_hash,verifier,nonce,return_to,expires_at) VALUES($1,$2,$3,$4,now()+interval '10 minutes')`, cryptoutil.SHA256(state), verifier, nonce, returnTo)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "OIDC_STATE_ERROR", "SSO 요청을 시작하지 못했습니다.")
		return
	}
	config := s.oauthConfig(r, all.OIDC.ClientID, all.OIDC.ClientSecret, all.OIDC.RedirectURL, all.OIDC.Scopes, provider.Endpoint())
	location := config.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("nonce", nonce))
	http.Redirect(w, r, location, http.StatusFound)
}

func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	if oidcError := r.URL.Query().Get("error"); oidcError != "" {
		http.Redirect(w, r, "/login?error="+url.QueryEscape(oidcError), http.StatusFound)
		return
	}
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	var verifier, nonce, returnTo string
	err := s.db.QueryRow(r.Context(), `DELETE FROM oidc_states WHERE state_hash=$1 AND expires_at>now() RETURNING verifier,nonce,return_to`, cryptoutil.SHA256(state)).Scan(&verifier, &nonce, &returnTo)
	if err != nil || code == "" {
		http.Redirect(w, r, "/login?error=invalid_oidc_state", http.StatusFound)
		return
	}
	all, err := s.settings.GetAll(r.Context(), true)
	if err != nil || !all.OIDC.Enabled {
		http.Redirect(w, r, "/login?error=oidc_disabled", http.StatusFound)
		return
	}
	provider, err := oidc.NewProvider(r.Context(), all.OIDC.IssuerURL)
	if err != nil {
		http.Redirect(w, r, "/login?error=oidc_discovery", http.StatusFound)
		return
	}
	config := s.oauthConfig(r, all.OIDC.ClientID, all.OIDC.ClientSecret, all.OIDC.RedirectURL, all.OIDC.Scopes, provider.Endpoint())
	token, err := config.Exchange(r.Context(), code, oauth2.VerifierOption(verifier))
	if err != nil {
		http.Redirect(w, r, "/login?error=oidc_exchange", http.StatusFound)
		return
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		http.Redirect(w, r, "/login?error=missing_id_token", http.StatusFound)
		return
	}
	idToken, err := provider.Verifier(&oidc.Config{ClientID: all.OIDC.ClientID}).Verify(r.Context(), rawIDToken)
	if err != nil {
		http.Redirect(w, r, "/login?error=invalid_id_token", http.StatusFound)
		return
	}
	var claims struct {
		Subject           string `json:"sub"`
		Nonce             string `json:"nonce"`
		Email             string `json:"email"`
		EmailVerified     bool   `json:"email_verified"`
		PreferredUsername string `json:"preferred_username"`
		Name              string `json:"name"`
		Picture           string `json:"picture"`
	}
	if err := idToken.Claims(&claims); err != nil || claims.Subject == "" || subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(nonce)) != 1 {
		http.Redirect(w, r, "/login?error=invalid_oidc_claims", http.StatusFound)
		return
	}
	user, err := s.findOrProvisionOIDCUser(r.Context(), claims.Subject, claims.Email, claims.PreferredUsername, claims.Name, claims.Picture, all.OIDC.AutoProvision, all.OIDC.DefaultRole, all.General.DefaultLocale)
	if err != nil {
		http.Redirect(w, r, "/login?error=account_not_provisioned", http.StatusFound)
		return
	}
	if err := s.createSession(w, r, user.ID, all.Security.SessionHours); err != nil {
		http.Redirect(w, r, "/login?error=session_failed", http.StatusFound)
		return
	}
	_, _ = s.db.Exec(r.Context(), `UPDATE users SET last_login_at=now() WHERE id=$1`, user.ID)
	s.audit(r, &user.ID, "LOGIN_OIDC", "SESSION", nil, nil)
	http.Redirect(w, r, safeReturnPath(returnTo), http.StatusFound)
}

func (s *Server) oauthConfig(r *http.Request, clientID, secret, configuredRedirect string, scopes []string, endpoint oauth2.Endpoint) oauth2.Config {
	redirect := configuredRedirect
	if redirect == "" {
		scheme := "http"
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		redirect = scheme + "://" + r.Host + "/api/v1/auth/oidc/callback"
	}
	return oauth2.Config{ClientID: clientID, ClientSecret: secret, RedirectURL: redirect, Endpoint: endpoint, Scopes: scopes}
}

func (s *Server) findOrProvisionOIDCUser(ctx context.Context, subject, email, username, name, picture string, provision bool, role, locale string) (User, error) {
	var user User
	err := s.db.QueryRow(ctx, `SELECT id,username,email,display_name,role,status,avatar_url,locale,created_at FROM users WHERE oidc_subject=$1`, subject).Scan(
		&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.Role, &user.Status, &user.AvatarURL, &user.Locale, &user.CreatedAt)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) || !provision || strings.TrimSpace(email) == "" {
		return User{}, errors.New("user is not provisioned")
	}
	if username == "" {
		username = strings.Split(email, "@")[0]
	}
	username = normalizeUsername(username)
	if name == "" {
		name = username
	}
	if role != "ADMIN" {
		role = "USER"
	}
	locale = strings.TrimSpace(locale)
	if locale == "" {
		locale = "ko-KR"
	}
	userID := uuid.New()
	workspaceID := uuid.New()
	dataKey, err := cryptoutil.RandomBytes(32)
	if err != nil {
		return User{}, err
	}
	wrapped, err := s.sealer.Seal(dataKey, "user-key:"+userID.String()+":1")
	if err != nil {
		return User{}, err
	}
	err = database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		candidate := username
		for attempt := 0; attempt < 20; attempt++ {
			_, err := tx.Exec(ctx, `INSERT INTO users(id,username,email,display_name,role,status,oidc_subject,avatar_url,locale) VALUES($1,$2,$3,$4,$5,'ACTIVE',$6,NULLIF($7,''),$8)`,
				userID, candidate, strings.ToLower(email), name, role, subject, picture, locale)
			if err == nil {
				username = candidate
				break
			}
			if !strings.Contains(err.Error(), "users_username_key") {
				return err
			}
			candidate = fmt.Sprintf("%s-%d", username, attempt+2)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workspaces(id,name,slug,kind,owner_id) VALUES($1,$2,$3,'PERSONAL',$4)`, workspaceID, name+"의 문서", "personal-"+userID.String(), userID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workspace_members(workspace_id,user_id,role) VALUES($1,$2,'OWNER')`, workspaceID, userID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO user_keys(user_id,name,fingerprint,wrapped_key,status,version) VALUES($1,'기본 개인 키',$2,$3,'ACTIVE',1)`, userID, cryptoutil.Fingerprint(dataKey), wrapped)
		return err
	})
	if err != nil {
		return User{}, err
	}
	return User{ID: userID, Username: username, Email: strings.ToLower(email), DisplayName: name, Role: role, Status: "ACTIVE", Locale: locale, CreatedAt: time.Now()}, nil
}

func normalizeUsername(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		}
	}
	if b.Len() < 3 {
		encoded := base64.RawURLEncoding.EncodeToString([]byte(value)) + "000000"
		return "user-" + encoded[:6]
	}
	return truncate(b.String(), 48)
}

func clientIP(r *http.Request) any {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if net.ParseIP(host) == nil {
		return nil
	}
	return host
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func (s *Server) audit(r *http.Request, actor *uuid.UUID, action, resourceType string, resourceID *uuid.UUID, metadata any) {
	if metadata == nil {
		metadata = map[string]any{}
	}
	_, err := s.db.Exec(r.Context(), `INSERT INTO activity_logs(actor_id,action,resource_type,resource_id,ip,user_agent,metadata) VALUES($1,$2,$3,$4,$5,$6,$7)`,
		actor, action, resourceType, resourceID, clientIP(r), truncate(r.UserAgent(), 1000), metadata)
	if err != nil {
		s.logger.Warn("audit log failed", "action", action, "error", err)
	}
}
