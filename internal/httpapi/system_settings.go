package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/hkjang/muni/internal/settings"
)

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeData(w, http.StatusOK, map[string]any{"status": "ok", "service": "muni", "version": s.info.Version})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "데이터베이스에 연결할 수 없습니다.")
		return
	}
	writeData(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) publicSystem(w http.ResponseWriter, r *http.Request) {
	all, err := s.settings.GetAll(r.Context(), false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SETTINGS_ERROR", "서비스 정보를 불러오지 못했습니다.")
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"serviceName": all.General.ServiceName, "version": s.info.Version, "commit": s.info.Commit,
		"localLoginEnabled": all.General.AllowLocalLogin, "oidcEnabled": all.OIDC.Enabled,
		"oidcLoginUrl": "/api/v1/auth/oidc/start", "maxAiTokens": settings.MaxAITokens,
	})
}

func (s *Server) systemCapabilities(w http.ResponseWriter, r *http.Request) {
	all, err := s.settings.GetAll(r.Context(), false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SETTINGS_ERROR", "서비스 기능 설정을 불러오지 못했습니다.")
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"workflowEnabled": all.Workflow.Enabled,
		"aiEnabled":       all.AI.Enabled,
		"pdfExport":       all.Export.EnablePDF,
		"docxExport":      all.Export.EnableDOCX,
		"presentations":   all.Ptium.Enabled && strings.TrimSpace(all.Ptium.BaseURL) != "",
		"maxAiTokens":     all.AI.MaxTokens,
	})
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	all, err := s.settings.GetAll(r.Context(), false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SETTINGS_ERROR", "설정을 불러오지 못했습니다.")
		return
	}
	writeData(w, http.StatusOK, all)
}

func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request) {
	var input settings.All
	if !decodeJSON(w, r, &input) {
		return
	}
	p, _ := principalFrom(r.Context())
	if err := s.settings.Save(r.Context(), input, p.User.ID); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_SETTINGS", err.Error())
		return
	}
	s.audit(r, &p.User.ID, "UPDATE_SETTINGS", "SETTINGS", nil, map[string]any{"categories": []string{"general", "oidc", "ai", "workflow", "security", "export", "ptium", "retention", "smtp"}})
	all, _ := s.settings.GetAll(r.Context(), false)
	writeData(w, http.StatusOK, all)
}

func (s *Server) testOIDC(w http.ResponseWriter, r *http.Request) {
	var input settings.OIDC
	if !decodeJSON(w, r, &input) {
		return
	}
	if strings.TrimSpace(input.IssuerURL) == "" {
		writeError(w, http.StatusBadRequest, "OIDC_ISSUER_REQUIRED", "Issuer URL을 입력해 주세요.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	provider, err := oidc.NewProvider(ctx, input.IssuerURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, "OIDC_TEST_FAILED", "OIDC discovery에 실패했습니다: "+err.Error())
		return
	}
	var claims map[string]any
	if err := provider.Claims(&claims); err != nil {
		writeError(w, http.StatusBadGateway, "OIDC_TEST_FAILED", "OIDC metadata를 확인하지 못했습니다.")
		return
	}
	writeData(w, http.StatusOK, map[string]any{"ok": true, "issuer": claims["issuer"], "authorizationEndpoint": claims["authorization_endpoint"], "tokenEndpoint": claims["token_endpoint"]})
}
