package httpapi

import (
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/hkjang/muni/internal/cryptoutil"
	"github.com/hkjang/muni/internal/realtime"
	"github.com/hkjang/muni/internal/settings"
	"github.com/hkjang/muni/webui"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	db       *pgxpool.Pool
	sealer   *cryptoutil.Sealer
	settings *settings.Store
	info     BuildInfo
	logger   *slog.Logger
	hub      *realtime.Hub
	mux      *http.ServeMux
	aiCompat *aiCompatibility
}

func New(db *pgxpool.Pool, sealer *cryptoutil.Sealer, info BuildInfo, logger *slog.Logger) *Server {
	s := &Server{db: db, sealer: sealer, settings: settings.NewStore(db, sealer), info: info, logger: logger, hub: realtime.NewHub(), mux: http.NewServeMux(), aiCompat: newAICompatibility()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.securityHeaders(s.requestLog(s.mux))
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.health)
	s.mux.HandleFunc("GET /readyz", s.ready)
	s.mux.HandleFunc("GET /api/v1/system/public", s.publicSystem)
	s.mux.HandleFunc("GET /api/openapi.yaml", s.openAPISpec)
	s.mux.HandleFunc("POST /api/v1/auth/login", s.login)
	s.mux.HandleFunc("GET /api/v1/auth/oidc/start", s.oidcStart)
	s.mux.HandleFunc("GET /api/v1/auth/oidc/callback", s.oidcCallback)
	s.mux.Handle("POST /api/v1/auth/logout", s.requireAuth(http.HandlerFunc(s.logout)))
	s.mux.Handle("GET /api/v1/auth/me", s.requireAuth(http.HandlerFunc(s.me)))
	s.mux.Handle("GET /api/v1/system/capabilities", s.requireAuth(http.HandlerFunc(s.systemCapabilities)))
	s.mux.Handle("GET /api/v1/users/search", s.requireAuth(http.HandlerFunc(s.searchUsers)))

	s.mux.Handle("GET /api/v1/workspaces", s.requireAuth(http.HandlerFunc(s.listWorkspaces)))
	s.mux.Handle("POST /api/v1/workspaces", s.requireAuth(http.HandlerFunc(s.createWorkspace)))
	s.mux.Handle("GET /api/v1/workspaces/{id}", s.requireAuth(http.HandlerFunc(s.getWorkspace)))
	s.mux.Handle("GET /api/v1/workspaces/{id}/members", s.requireAuth(http.HandlerFunc(s.listWorkspaceMembers)))
	s.mux.Handle("PUT /api/v1/workspaces/{id}/members", s.requireAuth(http.HandlerFunc(s.upsertWorkspaceMember)))
	s.mux.Handle("DELETE /api/v1/workspaces/{id}/members/{userId}", s.requireAuth(http.HandlerFunc(s.deleteWorkspaceMember)))
	s.mux.Handle("POST /api/v1/workspaces/{id}/folders", s.requireAuth(http.HandlerFunc(s.createFolder)))
	s.mux.Handle("GET /api/v1/workspaces/{id}/folders", s.requireAuth(http.HandlerFunc(s.listFolders)))
	s.mux.Handle("GET /api/v1/workspaces/{id}/documents", s.requireAuth(http.HandlerFunc(s.listDocuments)))

	s.mux.Handle("POST /api/v1/documents", s.requireAuth(http.HandlerFunc(s.createDocument)))
	s.mux.Handle("GET /api/v1/documents", s.requireAuth(http.HandlerFunc(s.listUserDocuments)))
	s.mux.Handle("POST /api/v1/import", s.requireAuth(http.HandlerFunc(s.importDocument)))
	s.mux.Handle("GET /api/v1/documents/{id}", s.requireAuth(http.HandlerFunc(s.getDocument)))
	s.mux.Handle("PUT /api/v1/documents/{id}", s.requireAuth(http.HandlerFunc(s.updateDocument)))
	s.mux.Handle("DELETE /api/v1/documents/{id}", s.requireAuth(http.HandlerFunc(s.deleteDocument)))
	s.mux.Handle("POST /api/v1/documents/{id}/restore", s.requireAuth(http.HandlerFunc(s.restoreDocument)))
	s.mux.Handle("POST /api/v1/documents/{id}/favorite", s.requireAuth(http.HandlerFunc(s.favoriteDocument)))
	s.mux.Handle("DELETE /api/v1/documents/{id}/favorite", s.requireAuth(http.HandlerFunc(s.unfavoriteDocument)))
	s.mux.Handle("GET /api/v1/documents/{id}/permissions", s.requireAuth(http.HandlerFunc(s.listDocumentPermissions)))
	s.mux.Handle("PUT /api/v1/documents/{id}/permissions", s.requireAuth(http.HandlerFunc(s.upsertDocumentPermission)))
	s.mux.Handle("DELETE /api/v1/documents/{id}/permissions/{permissionId}", s.requireAuth(http.HandlerFunc(s.deleteDocumentPermission)))
	s.mux.Handle("GET /api/v1/documents/{id}/revisions", s.requireAuth(http.HandlerFunc(s.listRevisions)))
	s.mux.Handle("POST /api/v1/documents/{id}/revisions/{revision}/restore", s.requireAuth(http.HandlerFunc(s.restoreRevision)))
	s.mux.Handle("PATCH /api/v1/documents/{id}/revisions/{revision}", s.requireAuth(http.HandlerFunc(s.nameRevision)))
	s.mux.Handle("GET /api/v1/documents/{id}/revisions/{from}/diff/{to}", s.requireAuth(http.HandlerFunc(s.compareRevisions)))
	s.mux.Handle("GET /api/v1/documents/{id}/comments", s.requireAuth(http.HandlerFunc(s.listComments)))
	s.mux.Handle("POST /api/v1/documents/{id}/comments", s.requireAuth(http.HandlerFunc(s.createComment)))
	s.mux.Handle("POST /api/v1/comments/{id}/resolve", s.requireAuth(http.HandlerFunc(s.resolveComment)))
	s.mux.Handle("POST /api/v1/comments/{id}/reopen", s.requireAuth(http.HandlerFunc(s.reopenComment)))
	s.mux.Handle("GET /api/v1/documents/{id}/suggestions", s.requireAuth(http.HandlerFunc(s.listSuggestions)))
	s.mux.Handle("POST /api/v1/documents/{id}/suggestions", s.requireAuth(http.HandlerFunc(s.createSuggestion)))
	s.mux.Handle("POST /api/v1/documents/{id}/ai/patch", s.requireAuth(http.HandlerFunc(s.proposeDocumentPatch)))
	s.mux.Handle("POST /api/v1/suggestions/{id}/decision", s.requireAuth(http.HandlerFunc(s.decideSuggestion)))
	s.mux.Handle("POST /api/v1/documents/{id}/workflow/submit", s.requireAuth(http.HandlerFunc(s.submitApproval)))
	s.mux.Handle("POST /api/v1/approvals/{id}/decision", s.requireAuth(http.HandlerFunc(s.decideApproval)))
	s.mux.Handle("GET /api/v1/approvals", s.requireAuth(http.HandlerFunc(s.listApprovals)))
	s.mux.Handle("GET /api/v1/search", s.requireAuth(http.HandlerFunc(s.searchDocuments)))
	s.mux.Handle("GET /api/v1/documents/{id}/export/{format}", s.requireAuth(http.HandlerFunc(s.exportDocument)))
	s.mux.Handle("POST /api/v1/documents/{id}/presentations", s.requireAuth(http.HandlerFunc(s.createPresentation)))
	s.mux.Handle("GET /api/v1/documents/{id}/presentations", s.requireAuth(http.HandlerFunc(s.listPresentations)))
	s.mux.Handle("GET /api/v1/documents/{id}/presentations/{presentationId}/status", s.requireAuth(http.HandlerFunc(s.refreshPresentation)))
	s.mux.Handle("GET /api/v1/documents/{id}/presentations/{presentationId}/download", s.requireAuth(http.HandlerFunc(s.downloadPresentation)))
	s.mux.Handle("GET /api/v1/documents/{id}/presentations/{presentationId}/sync", s.requireAuth(http.HandlerFunc(s.planPresentationSync)))
	s.mux.Handle("POST /api/v1/documents/{id}/presentations/{presentationId}/sync", s.requireAuth(http.HandlerFunc(s.applyPresentationSync)))
	s.mux.Handle("POST /api/v1/documents/{id}/presentations/{presentationId}/citations", s.requireAuth(http.HandlerFunc(s.citePresentation)))
	s.mux.Handle("DELETE /api/v1/documents/{id}/presentations/{presentationId}", s.requireAuth(http.HandlerFunc(s.unlinkPresentation)))
	s.mux.Handle("GET /api/v1/documents/{id}/attachments", s.requireAuth(http.HandlerFunc(s.listAttachments)))
	s.mux.Handle("POST /api/v1/documents/{id}/attachments", s.requireAuth(http.HandlerFunc(s.uploadAttachment)))
	s.mux.Handle("GET /api/v1/attachments/{id}", s.requireAuth(http.HandlerFunc(s.downloadAttachment)))
	s.mux.Handle("DELETE /api/v1/attachments/{id}", s.requireAuth(http.HandlerFunc(s.deleteAttachment)))
	s.mux.Handle("GET /api/v1/collab/{id}", s.requireAuth(http.HandlerFunc(s.collaboration)))

	s.mux.Handle("GET /api/v1/me/keys", s.requireAuth(http.HandlerFunc(s.listUserKeys)))
	s.mux.Handle("POST /api/v1/me/keys/rotate", s.requireAuth(http.HandlerFunc(s.rotateUserKey)))
	s.mux.Handle("DELETE /api/v1/me/keys/{id}", s.requireAuth(http.HandlerFunc(s.revokeUserKey)))
	s.mux.Handle("GET /api/v1/me/api-keys", s.requireAuth(http.HandlerFunc(s.listAPIKeys)))
	s.mux.Handle("POST /api/v1/me/api-keys", s.requireAuth(http.HandlerFunc(s.createAPIKey)))
	s.mux.Handle("DELETE /api/v1/me/api-keys/{id}", s.requireAuth(http.HandlerFunc(s.revokeAPIKey)))
	s.mux.Handle("POST /api/v1/ai/chat", s.requireAuth(http.HandlerFunc(s.aiChat)))

	s.mux.Handle("POST /mcp", s.requireAuth(http.HandlerFunc(s.mcp)))
	s.mux.Handle("POST /api/v1/mcp", s.requireAuth(http.HandlerFunc(s.mcp)))
	s.mux.Handle("GET /.well-known/oauth-protected-resource", http.HandlerFunc(s.mcpProtectedResourceMetadata))

	s.mux.Handle("GET /api/v1/admin/settings", s.requireAdmin(http.HandlerFunc(s.getSettings)))
	s.mux.Handle("PUT /api/v1/admin/settings", s.requireAdmin(http.HandlerFunc(s.saveSettings)))
	s.mux.Handle("POST /api/v1/admin/settings/test-oidc", s.requireAdmin(http.HandlerFunc(s.testOIDC)))
	s.mux.Handle("POST /api/v1/admin/settings/test-ai", s.requireAdmin(http.HandlerFunc(s.testAI)))
	s.mux.Handle("POST /api/v1/admin/settings/test-ptium", s.requireAdmin(http.HandlerFunc(s.testPtium)))
	s.mux.Handle("GET /api/v1/admin/users", s.requireAdmin(http.HandlerFunc(s.listUsers)))
	s.mux.Handle("PATCH /api/v1/admin/users/{id}", s.requireAdmin(http.HandlerFunc(s.updateUser)))
	s.mux.Handle("GET /api/v1/admin/users/{id}/keys", s.requireAdmin(http.HandlerFunc(s.listAnyUserKeys)))
	s.mux.Handle("POST /api/v1/admin/users/{id}/keys/rotate", s.requireAdmin(http.HandlerFunc(s.rotateAnyUserKey)))
	s.mux.Handle("DELETE /api/v1/admin/users/{id}/keys/{keyId}", s.requireAdmin(http.HandlerFunc(s.revokeAnyUserKey)))
	s.mux.Handle("GET /api/v1/admin/overview", s.requireAdmin(http.HandlerFunc(s.adminOverview)))
	s.mux.Handle("GET /api/v1/admin/workspaces", s.requireAdmin(http.HandlerFunc(s.adminWorkspaces)))
	s.mux.Handle("GET /api/v1/admin/documents", s.requireAdmin(http.HandlerFunc(s.adminListDocuments)))
	s.mux.Handle("POST /api/v1/admin/documents/{id}/transfer", s.requireAdmin(http.HandlerFunc(s.adminTransferDocument)))
	s.mux.Handle("DELETE /api/v1/admin/documents/{id}", s.requireAdmin(http.HandlerFunc(s.adminPurgeDocument)))
	s.mux.Handle("GET /api/v1/admin/users/{id}/sessions", s.requireAdmin(http.HandlerFunc(s.adminUserSessions)))
	s.mux.Handle("DELETE /api/v1/admin/users/{id}/sessions", s.requireAdmin(http.HandlerFunc(s.adminRevokeUserSessions)))
	s.mux.Handle("GET /api/v1/admin/audit", s.requireAdmin(http.HandlerFunc(s.listAudit)))
	s.mux.Handle("GET /api/v1/admin/audit.csv", s.requireAdmin(http.HandlerFunc(s.exportAudit)))
	s.mux.Handle("GET /api/v1/admin/ai-usage", s.requireAdmin(http.HandlerFunc(s.listAIUsage)))
	s.mux.Handle("GET /api/v1/admin/key-policies", s.requireAdmin(http.HandlerFunc(s.listKeyPolicies)))
	s.mux.Handle("PUT /api/v1/admin/key-policies/{role}", s.requireAdmin(http.HandlerFunc(s.updateKeyPolicy)))

	s.mux.HandleFunc("/", s.static)
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self' ws: wss:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		if r.URL.Path != "/healthz" && r.URL.Path != "/readyz" {
			s.logger.Info("http request", "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(started).Milliseconds())
		}
	})
}

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/mcp") {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "요청한 API를 찾을 수 없습니다.")
		return
	}
	dist, err := fs.Sub(webui.Dist, "dist")
	if err != nil {
		http.Error(w, "frontend unavailable", http.StatusInternalServerError)
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "." || name == "" {
		name = "index.html"
	}
	file, err := fs.ReadFile(dist, name)
	if err != nil {
		name = "index.html"
		file, err = fs.ReadFile(dist, name)
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if name == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	_, _ = w.Write(file)
}
