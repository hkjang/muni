package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/cryptoutil"
	"github.com/hkjang/muni/internal/handoff"
	"github.com/hkjang/muni/internal/tracking"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const MaxAITokens = 262144

type General struct {
	ServiceName     string `json:"serviceName"`
	AllowLocalLogin bool   `json:"allowLocalLogin"`
	DefaultLocale   string `json:"defaultLocale"`
	PageSize        int    `json:"pageSize"`
}

type OIDC struct {
	Enabled       bool     `json:"enabled"`
	IssuerURL     string   `json:"issuerUrl"`
	ClientID      string   `json:"clientId"`
	ClientSecret  string   `json:"clientSecret,omitempty"`
	SecretSet     bool     `json:"secretSet"`
	RedirectURL   string   `json:"redirectUrl"`
	Scopes        []string `json:"scopes"`
	AutoProvision bool     `json:"autoProvision"`
	DefaultRole   string   `json:"defaultRole"`
	// AutoLogin signs a visitor in without a login screen when the identity
	// provider still has a session for them (OIDC prompt=none). Off by default:
	// a fresh install must behave exactly as before, and the browser's request
	// for a silent attempt is honoured only while this is on.
	AutoLogin bool `json:"autoLogin"`
}

type AI struct {
	Enabled        bool   `json:"enabled"`
	BaseURL        string `json:"baseUrl"`
	APIKey         string `json:"apiKey,omitempty"`
	APIKeySet      bool   `json:"apiKeySet"`
	Model          string `json:"model"`
	MaxTokens      int    `json:"maxTokens"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
	SystemPrompt   string `json:"systemPrompt"`
}

type Workflow struct {
	Enabled           bool `json:"enabled"`
	RequiredApprovals int  `json:"requiredApprovals"`
	AllowSelfApproval bool `json:"allowSelfApproval"`
}

type Security struct {
	SessionHours     int  `json:"sessionHours"`
	APIKeyMaxDays    int  `json:"apiKeyMaxDays"`
	AllowPublicLinks bool `json:"allowPublicLinks"`
	MaxUploadMB      int  `json:"maxUploadMb"`
	AuditReads       bool `json:"auditReads"`
}

type Export struct {
	EnablePDF  bool `json:"enablePdf"`
	EnableDOCX bool `json:"enableDocx"`
}

// Ptium connects muni to a presentation service. muni sends documents there
// and keeps the link; it never reads Ptium's database.
type Ptium struct {
	Enabled        bool   `json:"enabled"`
	BaseURL        string `json:"baseUrl"`
	WebURL         string `json:"webUrl"`
	APIKey         string `json:"apiKey,omitempty"`
	APIKeySet      bool   `json:"apiKeySet"`
	DefaultTheme   string `json:"defaultTheme"`
	DefaultLocale  string `json:"defaultLocale"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
}

// Mail is the organisation's own SMTP relay and what muni sends through it.
//
// muni sends through it and nowhere else: there is no hosted sending service
// and no outbound connection to anywhere an operator did not configure, which
// is the only arrangement that works on a closed network. The stored keys
// (`mail.enabled`, `mail.smtp_host`, ...) are the ones every internal service
// uses, so an operator who has set one up has set them all up.
type Mail struct {
	Enabled  bool   `json:"enabled"`
	SMTPHost string `json:"smtpHost"`
	// SMTPPort is 25 when unset: an internal relay usually listens there,
	// without credentials and without TLS.
	SMTPPort int    `json:"smtpPort"`
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
	// PasswordSet reports whether one is stored, so the form can say so
	// without ever sending it back.
	PasswordSet bool `json:"passwordSet"`
	// Security is "auto", "none", "starttls" or "tls". Auto follows what the
	// relay advertises, which is what a relay nobody documented needs.
	Security string `json:"security"`
	// SkipTLSVerify accepts a certificate that does not verify, for an
	// internal server on a private certificate authority.
	SkipTLSVerify bool   `json:"skipTlsVerify"`
	FromAddress   string `json:"fromAddress"`
	FromName      string `json:"fromName"`
	// BaseURL is what a link in an email points at. Without it the mail says
	// what happened but not where.
	BaseURL        string `json:"baseUrl"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
	// Notify switches each kind of mail off on its own, for an administrator
	// who wants approvals mailed but not mentions.
	Notify MailNotify `json:"notify"`
}

// MailNotify is one switch per event muni mails about. Every switch is on
// unless it was stored off, so a new event never needs a settings change.
type MailNotify struct {
	ApprovalRequest  bool `json:"approvalRequest"`
	ApprovalDecision bool `json:"approvalDecision"`
	Mention          bool `json:"mention"`
	APIKeyExpiring   bool `json:"apiKeyExpiring"`
}

// AllMailNotifications is the default: every event mailed.
func AllMailNotifications() MailNotify {
	return MailNotify{ApprovalRequest: true, ApprovalDecision: true, Mention: true, APIKeyExpiring: true}
}

// Retention is how long muni keeps what it no longer needs.
//
// Every value is a number of days, and zero means keep it forever — which is
// what muni did before this existed, so an upgrade changes nothing until an
// administrator asks for it. Nothing here is a substitute for a backup: what
// is removed is removed.
type Retention struct {
	// TrashDays purges documents that have been in the trash this long.
	TrashDays int `json:"trashDays"`
	// RevisionDays drops old versions of a document, and RevisionKeep is how
	// many of the newest are kept whatever their age. A version an author
	// named is never dropped.
	RevisionDays int `json:"revisionDays"`
	RevisionKeep int `json:"revisionKeep"`
	// AuditDays and AIAuditDays trim the two logs.
	AuditDays   int `json:"auditDays"`
	AIAuditDays int `json:"aiAuditDays"`
}

// MinRevisionKeep is the floor on how many versions survive a cleanup. A
// policy that could leave a document with no history at all is not a retention
// policy, it is data loss with a schedule.
const MinRevisionKeep = 5

// Normalize brings a stored or submitted policy into range.
func (r Retention) Normalize() Retention {
	clamp := func(value int) int {
		if value < 0 || value > 3650 {
			return 0
		}
		return value
	}
	r.TrashDays = clamp(r.TrashDays)
	r.RevisionDays = clamp(r.RevisionDays)
	r.AuditDays = clamp(r.AuditDays)
	r.AIAuditDays = clamp(r.AIAuditDays)
	if r.RevisionKeep < MinRevisionKeep {
		r.RevisionKeep = MinRevisionKeep
	}
	if r.RevisionKeep > 1000 {
		r.RevisionKeep = 1000
	}
	return r
}

type All struct {
	General   General   `json:"general"`
	OIDC      OIDC      `json:"oidc"`
	AI        AI        `json:"ai"`
	Workflow  Workflow  `json:"workflow"`
	Security  Security  `json:"security"`
	Export    Export    `json:"export"`
	Ptium     Ptium     `json:"ptium"`
	Retention Retention `json:"retention"`
	Mail      Mail      `json:"mail"`
	// Tracking is the visitor tracking snippet. Off by default: a fresh
	// install serves the page exactly as before until an administrator asks.
	Tracking tracking.Config `json:"tracking"`
	// Handoff is the list of services documents may be sent to and taken
	// from. Empty by default, and while empty no button appears and no
	// source is accepted.
	Handoff handoff.Config `json:"handoff"`
}

// legacyMailPasswordKey is where the relay password was stored before the
// mail keys took their standard names. See migration 019.
const legacyMailPasswordKey = "smtp.password"

type Store struct {
	db     *pgxpool.Pool
	sealer *cryptoutil.Sealer
}

func NewStore(db *pgxpool.Pool, sealer *cryptoutil.Sealer) *Store {
	return &Store{db: db, sealer: sealer}
}

func (s *Store) GetAll(ctx context.Context, includeSecrets bool) (All, error) {
	values := map[string]json.RawMessage{}
	secrets := map[string][]byte{}
	rows, err := s.db.Query(ctx, `SELECT key,value,encrypted_value,is_secret FROM app_settings`)
	if err != nil {
		return All{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var value []byte
		var encrypted []byte
		var secret bool
		if err := rows.Scan(&key, &value, &encrypted, &secret); err != nil {
			return All{}, err
		}
		if secret {
			secrets[key] = encrypted
		} else {
			values[key] = value
		}
	}
	if err := rows.Err(); err != nil {
		return All{}, err
	}

	var out All
	decode(values, "general.service_name", &out.General.ServiceName)
	decode(values, "general.allow_local_login", &out.General.AllowLocalLogin)
	decode(values, "general.default_locale", &out.General.DefaultLocale)
	decode(values, "general.page_size", &out.General.PageSize)
	decode(values, "oidc.enabled", &out.OIDC.Enabled)
	decode(values, "oidc.issuer_url", &out.OIDC.IssuerURL)
	decode(values, "oidc.client_id", &out.OIDC.ClientID)
	decode(values, "oidc.redirect_url", &out.OIDC.RedirectURL)
	decode(values, "oidc.scopes", &out.OIDC.Scopes)
	decode(values, "oidc.auto_provision", &out.OIDC.AutoProvision)
	decode(values, "oidc.default_role", &out.OIDC.DefaultRole)
	decode(values, "oidc.auto_login", &out.OIDC.AutoLogin)
	decode(values, "ai.enabled", &out.AI.Enabled)
	decode(values, "ai.base_url", &out.AI.BaseURL)
	decode(values, "ai.model", &out.AI.Model)
	decode(values, "ai.max_tokens", &out.AI.MaxTokens)
	decode(values, "ai.timeout_seconds", &out.AI.TimeoutSeconds)
	decode(values, "ai.system_prompt", &out.AI.SystemPrompt)
	decode(values, "workflow.enabled", &out.Workflow.Enabled)
	decode(values, "workflow.required_approvals", &out.Workflow.RequiredApprovals)
	decode(values, "workflow.allow_self_approval", &out.Workflow.AllowSelfApproval)
	decode(values, "security.session_hours", &out.Security.SessionHours)
	decode(values, "security.api_key_max_days", &out.Security.APIKeyMaxDays)
	decode(values, "security.allow_public_links", &out.Security.AllowPublicLinks)
	decode(values, "security.max_upload_mb", &out.Security.MaxUploadMB)
	decode(values, "security.audit_reads", &out.Security.AuditReads)
	decode(values, "export.enable_pdf", &out.Export.EnablePDF)
	decode(values, "export.enable_docx", &out.Export.EnableDOCX)
	out.Mail.Notify = AllMailNotifications()
	decode(values, "mail.enabled", &out.Mail.Enabled)
	decode(values, "mail.smtp_host", &out.Mail.SMTPHost)
	decode(values, "mail.smtp_port", &out.Mail.SMTPPort)
	decode(values, "mail.username", &out.Mail.Username)
	decode(values, "mail.security", &out.Mail.Security)
	decode(values, "mail.from_address", &out.Mail.FromAddress)
	decode(values, "mail.from_name", &out.Mail.FromName)
	decode(values, "mail.skip_tls_verify", &out.Mail.SkipTLSVerify)
	decode(values, "mail.base_url", &out.Mail.BaseURL)
	decode(values, "mail.timeout_seconds", &out.Mail.TimeoutSeconds)
	decode(values, "mail.notify_approval_request", &out.Mail.Notify.ApprovalRequest)
	decode(values, "mail.notify_approval_decision", &out.Mail.Notify.ApprovalDecision)
	decode(values, "mail.notify_mention", &out.Mail.Notify.Mention)
	decode(values, "mail.notify_api_key_expiring", &out.Mail.Notify.APIKeyExpiring)
	decode(values, "retention.trash_days", &out.Retention.TrashDays)
	decode(values, "retention.revision_days", &out.Retention.RevisionDays)
	decode(values, "retention.revision_keep", &out.Retention.RevisionKeep)
	decode(values, "retention.audit_days", &out.Retention.AuditDays)
	decode(values, "retention.ai_audit_days", &out.Retention.AIAuditDays)
	out.Retention = out.Retention.Normalize()
	decode(values, "ptium.enabled", &out.Ptium.Enabled)
	decode(values, "ptium.base_url", &out.Ptium.BaseURL)
	decode(values, "ptium.web_url", &out.Ptium.WebURL)
	decode(values, "ptium.default_theme", &out.Ptium.DefaultTheme)
	decode(values, "ptium.default_locale", &out.Ptium.DefaultLocale)
	decode(values, "ptium.timeout_seconds", &out.Ptium.TimeoutSeconds)
	// The proxy is the default for a collector that was never configured,
	// because it is the one arrangement that leaves the policy alone.
	out.Tracking.MomentoProxy = true
	decode(values, "tracking.enabled", &out.Tracking.Enabled)
	decode(values, "tracking.provider", &out.Tracking.Provider)
	decode(values, "tracking.momento_url", &out.Tracking.MomentoURL)
	decode(values, "tracking.momento_site_id", &out.Tracking.MomentoSiteID)
	decode(values, "tracking.momento_proxy", &out.Tracking.MomentoProxy)
	decode(values, "tracking.measurement_id", &out.Tracking.MeasurementID)
	decode(values, "tracking.matomo_url", &out.Tracking.MatomoURL)
	decode(values, "tracking.matomo_site_id", &out.Tracking.MatomoSiteID)
	decode(values, "tracking.custom_snippet", &out.Tracking.CustomSnippet)
	decode(values, "tracking.allowed_hosts", &out.Tracking.AllowedHosts)
	decode(values, "tracking.include_admin", &out.Tracking.IncludeAdmin)
	decode(values, "tracking.placement", &out.Tracking.Placement)
	out.Tracking = out.Tracking.Normalize()
	decode(values, "handoff.peers", &out.Handoff.Peers)
	out.Handoff = out.Handoff.Normalize()

	out.OIDC.SecretSet = len(secrets["oidc.client_secret"]) > 0
	out.AI.APIKeySet = len(secrets["ai.api_key"]) > 0
	out.Ptium.APIKeySet = len(secrets["ptium.api_key"]) > 0
	// The password was stored under its old name before the keys were
	// renamed, and a sealed value cannot be renamed in place: the name is
	// part of what it is sealed with. The old row keeps working until an
	// administrator saves a new password, which is written under the new one.
	passwordKey := "mail.password"
	if len(secrets[passwordKey]) == 0 && len(secrets[legacyMailPasswordKey]) > 0 {
		passwordKey = legacyMailPasswordKey
	}
	out.Mail.PasswordSet = len(secrets[passwordKey]) > 0
	if includeSecrets {
		if out.OIDC.SecretSet {
			plain, err := s.sealer.Open(secrets["oidc.client_secret"], "setting:oidc.client_secret")
			if err != nil {
				return All{}, err
			}
			out.OIDC.ClientSecret = string(plain)
		}
		if out.AI.APIKeySet {
			plain, err := s.sealer.Open(secrets["ai.api_key"], "setting:ai.api_key")
			if err != nil {
				return All{}, err
			}
			out.AI.APIKey = string(plain)
		}
		if out.Ptium.APIKeySet {
			plain, err := s.sealer.Open(secrets["ptium.api_key"], "setting:ptium.api_key")
			if err != nil {
				return All{}, err
			}
			out.Ptium.APIKey = string(plain)
		}
		if out.Mail.PasswordSet {
			plain, err := s.sealer.Open(secrets[passwordKey], "setting:"+passwordKey)
			if err != nil {
				return All{}, err
			}
			out.Mail.Password = string(plain)
		}
	}
	return out, nil
}

func decode(values map[string]json.RawMessage, key string, target any) {
	if value, ok := values[key]; ok {
		_ = json.Unmarshal(value, target)
	}
}

func (s *Store) Save(ctx context.Context, all All, actor uuid.UUID) error {
	if err := Validate(all); err != nil {
		return err
	}
	all.Tracking = all.Tracking.Normalize()
	all.Handoff = all.Handoff.Normalize()
	plain := map[string]any{
		"general.service_name": all.General.ServiceName, "general.allow_local_login": all.General.AllowLocalLogin,
		"general.default_locale": all.General.DefaultLocale, "general.page_size": all.General.PageSize,
		"oidc.enabled": all.OIDC.Enabled, "oidc.issuer_url": all.OIDC.IssuerURL, "oidc.client_id": all.OIDC.ClientID,
		"oidc.redirect_url": all.OIDC.RedirectURL, "oidc.scopes": all.OIDC.Scopes, "oidc.auto_provision": all.OIDC.AutoProvision,
		"oidc.default_role": all.OIDC.DefaultRole, "oidc.auto_login": all.OIDC.AutoLogin,
		"ai.enabled": all.AI.Enabled, "ai.base_url": all.AI.BaseURL,
		"ai.model": all.AI.Model, "ai.max_tokens": all.AI.MaxTokens, "ai.timeout_seconds": all.AI.TimeoutSeconds,
		"ai.system_prompt": all.AI.SystemPrompt, "workflow.enabled": all.Workflow.Enabled,
		"workflow.required_approvals": all.Workflow.RequiredApprovals, "workflow.allow_self_approval": all.Workflow.AllowSelfApproval,
		"security.session_hours": all.Security.SessionHours, "security.api_key_max_days": all.Security.APIKeyMaxDays,
		"security.allow_public_links": all.Security.AllowPublicLinks, "security.max_upload_mb": all.Security.MaxUploadMB,
		"security.audit_reads": all.Security.AuditReads, "export.enable_pdf": all.Export.EnablePDF, "export.enable_docx": all.Export.EnableDOCX,
		"ptium.enabled": all.Ptium.Enabled, "ptium.base_url": all.Ptium.BaseURL, "ptium.web_url": all.Ptium.WebURL,
		"ptium.default_theme": all.Ptium.DefaultTheme, "ptium.default_locale": all.Ptium.DefaultLocale,
		"ptium.timeout_seconds": all.Ptium.TimeoutSeconds,
		"retention.trash_days":  all.Retention.TrashDays, "retention.revision_days": all.Retention.RevisionDays,
		"retention.revision_keep": all.Retention.RevisionKeep, "retention.audit_days": all.Retention.AuditDays,
		"retention.ai_audit_days": all.Retention.AIAuditDays,
		"mail.enabled":            all.Mail.Enabled, "mail.smtp_host": all.Mail.SMTPHost, "mail.smtp_port": all.Mail.SMTPPort,
		"mail.username": all.Mail.Username, "mail.security": all.Mail.Security,
		"mail.from_address": all.Mail.FromAddress, "mail.from_name": all.Mail.FromName,
		"mail.skip_tls_verify": all.Mail.SkipTLSVerify, "mail.base_url": all.Mail.BaseURL,
		"mail.timeout_seconds":         all.Mail.TimeoutSeconds,
		"mail.notify_approval_request": all.Mail.Notify.ApprovalRequest, "mail.notify_approval_decision": all.Mail.Notify.ApprovalDecision,
		"mail.notify_mention": all.Mail.Notify.Mention, "mail.notify_api_key_expiring": all.Mail.Notify.APIKeyExpiring,
		"tracking.enabled": all.Tracking.Enabled, "tracking.provider": all.Tracking.Provider,
		"tracking.momento_url": all.Tracking.MomentoURL, "tracking.momento_site_id": all.Tracking.MomentoSiteID,
		"tracking.momento_proxy": all.Tracking.MomentoProxy, "tracking.measurement_id": all.Tracking.MeasurementID,
		"tracking.matomo_url": all.Tracking.MatomoURL, "tracking.matomo_site_id": all.Tracking.MatomoSiteID,
		"tracking.custom_snippet": all.Tracking.CustomSnippet, "tracking.allowed_hosts": all.Tracking.AllowedHosts,
		"tracking.include_admin": all.Tracking.IncludeAdmin, "tracking.placement": all.Tracking.Placement,
		"handoff.peers": all.Handoff.Peers,
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for key, value := range plain {
		if err := putPlain(ctx, tx, key, value, actor); err != nil {
			return err
		}
	}
	for key, value := range map[string]string{"oidc.client_secret": all.OIDC.ClientSecret, "ai.api_key": all.AI.APIKey, "ptium.api_key": all.Ptium.APIKey, "mail.password": all.Mail.Password} {
		if value == "" { // Empty input preserves an already configured secret.
			continue
		}
		if key == "mail.password" {
			// A new password under the new name retires the one under the old.
			if _, err := tx.Exec(ctx, `DELETE FROM app_settings WHERE key=$1`, legacyMailPasswordKey); err != nil {
				return err
			}
		}
		encrypted, err := s.sealer.Seal([]byte(value), "setting:"+key)
		if err != nil {
			return err
		}
		category := strings.SplitN(key, ".", 2)[0]
		if _, err := tx.Exec(ctx, `INSERT INTO app_settings(key,category,encrypted_value,is_secret,updated_by,updated_at)
			VALUES($1,$2,$3,true,$4,now()) ON CONFLICT(key) DO UPDATE SET value=NULL, encrypted_value=excluded.encrypted_value,
			is_secret=true, updated_by=excluded.updated_by, updated_at=now()`, key, category, encrypted, actor); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// PutValue stores one plain setting on its own. Save writes the whole form,
// which is right for the settings screen and wrong for a one-click fix that
// must not carry whatever else the form happened to hold.
func (s *Store) PutValue(ctx context.Context, key string, value any, actor uuid.UUID) error {
	return putPlain(ctx, s.db, key, value, actor)
}

type execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

func putPlain(ctx context.Context, db execer, key string, value any, actor uuid.UUID) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	category := strings.SplitN(key, ".", 2)[0]
	_, err = db.Exec(ctx, `INSERT INTO app_settings(key,category,value,is_secret,updated_by,updated_at)
		VALUES($1,$2,$3,false,$4,now()) ON CONFLICT(key) DO UPDATE SET value=excluded.value, encrypted_value=NULL,
		is_secret=false, updated_by=excluded.updated_by, updated_at=now()`, key, category, encoded, actor)
	return err
}

func Validate(all All) error {
	if all.Mail.Enabled {
		if strings.TrimSpace(all.Mail.SMTPHost) == "" {
			return errors.New("메일 서버 주소가 필요합니다")
		}
		if strings.TrimSpace(all.Mail.FromAddress) == "" && strings.TrimSpace(all.Mail.Username) == "" {
			return errors.New("보내는 주소가 필요합니다")
		}
		if all.Mail.SMTPPort < 0 || all.Mail.SMTPPort > 65535 {
			return errors.New("메일 서버 포트가 올바르지 않습니다")
		}
		switch strings.ToLower(strings.TrimSpace(all.Mail.Security)) {
		case "", "auto", "none", "starttls", "tls":
		default:
			return errors.New("메일 보안 방식은 auto·none·starttls·tls 중 하나여야 합니다")
		}
		if all.Mail.TimeoutSeconds < 0 || all.Mail.TimeoutSeconds > 300 {
			return errors.New("메일 제한 시간은 0~300초여야 합니다")
		}
		if all.Mail.BaseURL != "" {
			base, err := url.Parse(all.Mail.BaseURL)
			if err != nil || base.Scheme == "" || base.Host == "" {
				return errors.New("메일에 넣을 서비스 주소가 올바르지 않습니다")
			}
		}
	}
	// A policy that would leave a document without history is refused rather
	// than quietly corrected, so an administrator sees what they asked for.
	if all.Retention.RevisionKeep != 0 && all.Retention.RevisionKeep < MinRevisionKeep {
		return errors.New("버전은 최소 5개까지는 남겨야 합니다")
	}
	for _, days := range []int{all.Retention.TrashDays, all.Retention.RevisionDays, all.Retention.AuditDays, all.Retention.AIAuditDays} {
		if days < 0 || days > 3650 {
			return errors.New("보존 기간은 0~3650일이어야 합니다")
		}
	}
	all.General.ServiceName = strings.TrimSpace(all.General.ServiceName)
	if all.General.ServiceName == "" || len([]rune(all.General.ServiceName)) > 60 {
		return errors.New("서비스 이름은 1~60자여야 합니다")
	}
	if all.General.PageSize < 10 || all.General.PageSize > 100 {
		return errors.New("페이지 크기는 10~100이어야 합니다")
	}
	if all.OIDC.Enabled {
		issuer, err := url.Parse(all.OIDC.IssuerURL)
		if err != nil || issuer.Scheme == "" || issuer.Host == "" {
			return errors.New("OIDC issuer URL이 올바르지 않습니다")
		}
		if strings.TrimSpace(all.OIDC.ClientID) == "" {
			return errors.New("OIDC client ID가 필요합니다")
		}
		if !all.OIDC.SecretSet && strings.TrimSpace(all.OIDC.ClientSecret) == "" {
			return errors.New("OIDC client secret이 필요합니다")
		}
		if !slices.Contains(all.OIDC.Scopes, "openid") {
			return errors.New("OIDC scope에는 openid가 포함되어야 합니다")
		}
		if all.OIDC.RedirectURL != "" {
			redirect, err := url.Parse(all.OIDC.RedirectURL)
			if err != nil || redirect.Scheme == "" || redirect.Host == "" {
				return errors.New("OIDC redirect URL이 올바르지 않습니다")
			}
		}
	}
	if all.OIDC.DefaultRole != "USER" && all.OIDC.DefaultRole != "ADMIN" {
		return errors.New("OIDC 기본 역할이 올바르지 않습니다")
	}
	if all.AI.Enabled {
		baseURL, err := url.Parse(all.AI.BaseURL)
		if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
			return errors.New("AI API base URL이 올바르지 않습니다")
		}
		if strings.TrimSpace(all.AI.Model) == "" {
			return errors.New("AI 모델 이름이 필요합니다")
		}
	}
	if all.AI.MaxTokens < 1 || all.AI.MaxTokens > MaxAITokens {
		return fmt.Errorf("AI max token은 1~%d여야 합니다", MaxAITokens)
	}
	if all.AI.TimeoutSeconds < 30 || all.AI.TimeoutSeconds > 3600 {
		return errors.New("AI 제한 시간은 30~3600초여야 합니다")
	}
	if all.Workflow.RequiredApprovals < 1 || all.Workflow.RequiredApprovals > 10 {
		return errors.New("필요 승인 수는 1~10이어야 합니다")
	}
	if all.Security.SessionHours < 1 || all.Security.SessionHours > 720 {
		return errors.New("세션 시간은 1~720시간이어야 합니다")
	}
	if all.Security.APIKeyMaxDays < 1 || all.Security.APIKeyMaxDays > 3650 {
		return errors.New("API 키 최대 수명은 1~3650일이어야 합니다")
	}
	if all.Security.MaxUploadMB < 1 || all.Security.MaxUploadMB > 1024 {
		return errors.New("업로드 한도는 1~1024MB여야 합니다")
	}
	if all.Ptium.Enabled {
		base, err := url.Parse(all.Ptium.BaseURL)
		if err != nil || base.Scheme == "" || base.Host == "" {
			return errors.New("Ptium 주소가 올바르지 않습니다")
		}
		if all.Ptium.WebURL != "" {
			web, err := url.Parse(all.Ptium.WebURL)
			if err != nil || web.Scheme == "" || web.Host == "" {
				return errors.New("Ptium 편집기 주소가 올바르지 않습니다")
			}
		}
		if !all.Ptium.APIKeySet && strings.TrimSpace(all.Ptium.APIKey) == "" {
			return errors.New("Ptium API key가 필요합니다")
		}
	}
	if all.Ptium.TimeoutSeconds != 0 && (all.Ptium.TimeoutSeconds < 5 || all.Ptium.TimeoutSeconds > 900) {
		return errors.New("Ptium 제한 시간은 5~900초여야 합니다")
	}
	if err := all.Tracking.Validate(); err != nil {
		return err
	}
	return all.Handoff.Validate()
}
