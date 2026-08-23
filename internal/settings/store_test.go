package settings

import (
	"strings"
	"testing"
)

func validSettings() All {
	return All{
		General:  General{ServiceName: "muni", AllowLocalLogin: true, DefaultLocale: "ko-KR", PageSize: 30},
		OIDC:     OIDC{Enabled: false, Scopes: []string{"openid", "profile"}, DefaultRole: "USER"},
		AI:       AI{Enabled: false, MaxTokens: 32768, TimeoutSeconds: 600},
		Workflow: Workflow{RequiredApprovals: 1},
		Security: Security{SessionHours: 12, APIKeyMaxDays: 365, MaxUploadMB: 50},
	}
}

func TestValidateMaxAITokens(t *testing.T) {
	value := validSettings()
	value.AI.MaxTokens = MaxAITokens + 1
	err := Validate(value)
	if err == nil || !strings.Contains(err.Error(), "262144") {
		t.Fatalf("expected max token error, got %v", err)
	}
}

func TestValidateOIDC(t *testing.T) {
	value := validSettings()
	value.OIDC.Enabled = true
	value.OIDC.IssuerURL = "https://keycloak.internal/realms/muni"
	value.OIDC.ClientID = "muni"
	value.OIDC.ClientSecret = "secret"
	if err := Validate(value); err != nil {
		t.Fatalf("valid OIDC settings rejected: %v", err)
	}
}
