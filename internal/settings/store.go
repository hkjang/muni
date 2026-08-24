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

type All struct {
	General  General  `json:"general"`
	OIDC     OIDC     `json:"oidc"`
	AI       AI       `json:"ai"`
	Workflow Workflow `json:"workflow"`
	Security Security `json:"security"`
	Export   Export   `json:"export"`
	Ptium    Ptium    `json:"ptium"`
}

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
	decode(values, "ptium.enabled", &out.Ptium.Enabled)
	decode(values, "ptium.base_url", &out.Ptium.BaseURL)
	decode(values, "ptium.web_url", &out.Ptium.WebURL)
	decode(values, "ptium.default_theme", &out.Ptium.DefaultTheme)
	decode(values, "ptium.default_locale", &out.Ptium.DefaultLocale)
	decode(values, "ptium.timeout_seconds", &out.Ptium.TimeoutSeconds)

	out.OIDC.SecretSet = len(secrets["oidc.client_secret"]) > 0
	out.AI.APIKeySet = len(secrets["ai.api_key"]) > 0
	out.Ptium.APIKeySet = len(secrets["ptium.api_key"]) > 0
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
	plain := map[string]any{
		"general.service_name": all.General.ServiceName, "general.allow_local_login": all.General.AllowLocalLogin,
		"general.default_locale": all.General.DefaultLocale, "general.page_size": all.General.PageSize,
		"oidc.enabled": all.OIDC.Enabled, "oidc.issuer_url": all.OIDC.IssuerURL, "oidc.client_id": all.OIDC.ClientID,
		"oidc.redirect_url": all.OIDC.RedirectURL, "oidc.scopes": all.OIDC.Scopes, "oidc.auto_provision": all.OIDC.AutoProvision,
		"oidc.default_role": all.OIDC.DefaultRole, "ai.enabled": all.AI.Enabled, "ai.base_url": all.AI.BaseURL,
		"ai.model": all.AI.Model, "ai.max_tokens": all.AI.MaxTokens, "ai.timeout_seconds": all.AI.TimeoutSeconds,
		"ai.system_prompt": all.AI.SystemPrompt, "workflow.enabled": all.Workflow.Enabled,
		"workflow.required_approvals": all.Workflow.RequiredApprovals, "workflow.allow_self_approval": all.Workflow.AllowSelfApproval,
		"security.session_hours": all.Security.SessionHours, "security.api_key_max_days": all.Security.APIKeyMaxDays,
		"security.allow_public_links": all.Security.AllowPublicLinks, "security.max_upload_mb": all.Security.MaxUploadMB,
		"security.audit_reads": all.Security.AuditReads, "export.enable_pdf": all.Export.EnablePDF, "export.enable_docx": all.Export.EnableDOCX,
		"ptium.enabled": all.Ptium.Enabled, "ptium.base_url": all.Ptium.BaseURL, "ptium.web_url": all.Ptium.WebURL,
		"ptium.default_theme": all.Ptium.DefaultTheme, "ptium.default_locale": all.Ptium.DefaultLocale,
		"ptium.timeout_seconds": all.Ptium.TimeoutSeconds,
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for key, value := range plain {
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		category := strings.SplitN(key, ".", 2)[0]
		if _, err := tx.Exec(ctx, `INSERT INTO app_settings(key,category,value,is_secret,updated_by,updated_at)
			VALUES($1,$2,$3,false,$4,now()) ON CONFLICT(key) DO UPDATE SET value=excluded.value, encrypted_value=NULL,
			is_secret=false, updated_by=excluded.updated_by, updated_at=now()`, key, category, encoded, actor); err != nil {
			return err
		}
	}
	for key, value := range map[string]string{"oidc.client_secret": all.OIDC.ClientSecret, "ai.api_key": all.AI.APIKey, "ptium.api_key": all.Ptium.APIKey} {
		if value == "" { // Empty input preserves an already configured secret.
			continue
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

func Validate(all All) error {
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
	return nil
}
