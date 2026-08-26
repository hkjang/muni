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

// route records every pattern as it is registered, so a test can hold the
// published API document against what the service actually serves. The spec
// drifted to a third of the routes before anything noticed.
func (s *Server) handle(pattern string, handler http.Handler) {
	s.patterns = append(s.patterns, pattern)
	s.mux.Handle(pattern, handler)
}

func (s *Server) handleFunc(pattern string, handler http.HandlerFunc) {
	s.handle(pattern, handler)
}

// Patterns is the list of routes this server serves, in registration order.
func (s *Server) Patterns() []string { return s.patterns }

type Server struct {
	db       *pgxpool.Pool
	sealer   *cryptoutil.Sealer
	settings *settings.Store
	info     BuildInfo
	logger   *slog.Logger
	hub      *realtime.Hub
	mux      *http.ServeMux
	patterns []string
	aiCompat *aiCompatibility
	logins   *loginAttempts
	metrics  *metrics
}

func New(db *pgxpool.Pool, sealer *cryptoutil.Sealer, info BuildInfo, logger *slog.Logger) *Server {
	s := &Server{db: db, sealer: sealer, settings: settings.NewStore(db, sealer), info: info, logger: logger, hub: realtime.NewHub(), mux: http.NewServeMux(), aiCompat: newAICompatibility(), logins: newLoginAttempts(), metrics: newMetrics()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.securityHeaders(s.requestLog(s.mux))
}

func (s *Server) routes() {
	s.handleFunc("GET /healthz", s.health)
	s.handleFunc("GET /readyz", s.ready)
	s.handleFunc("GET /api/v1/system/public", s.publicSystem)
	s.handleFunc("GET /api/openapi.yaml", s.openAPISpec)
	s.handleFunc("POST /api/v1/auth/login", s.login)
	s.handleFunc("GET /api/v1/auth/oidc/start", s.oidcStart)
	s.handleFunc("GET /api/v1/auth/oidc/callback", s.oidcCallback)
	s.handle("POST /api/v1/auth/logout", s.requireAuth(http.HandlerFunc(s.logout)))
	s.handle("GET /api/v1/auth/me", s.requireAuth(http.HandlerFunc(s.me)))
	s.handle("GET /api/v1/system/capabilities", s.requireAuth(http.HandlerFunc(s.systemCapabilities)))
	s.handle("GET /api/v1/users/search", s.requireAuth(http.HandlerFunc(s.searchUsers)))

	s.handle("GET /api/v1/workspaces", s.requireAuth(http.HandlerFunc(s.listWorkspaces)))
	s.handle("POST /api/v1/workspaces", s.requireAuth(http.HandlerFunc(s.createWorkspace)))
	s.handle("GET /api/v1/workspaces/{id}", s.requireAuth(http.HandlerFunc(s.getWorkspace)))
	s.handle("GET /api/v1/workspaces/{id}/members", s.requireAuth(http.HandlerFunc(s.listWorkspaceMembers)))
	s.handle("PUT /api/v1/workspaces/{id}/members", s.requireAuth(http.HandlerFunc(s.upsertWorkspaceMember)))
	s.handle("DELETE /api/v1/workspaces/{id}/members/{userId}", s.requireAuth(http.HandlerFunc(s.deleteWorkspaceMember)))
	s.handle("POST /api/v1/workspaces/{id}/folders", s.requireAuth(http.HandlerFunc(s.createFolder)))
	s.handle("GET /api/v1/workspaces/{id}/folders", s.requireAuth(http.HandlerFunc(s.listFolders)))
	s.handle("PATCH /api/v1/folders/{id}", s.requireAuth(http.HandlerFunc(s.updateFolder)))
	s.handle("DELETE /api/v1/folders/{id}", s.requireAuth(http.HandlerFunc(s.deleteFolder)))
	s.handle("GET /api/v1/workspaces/{id}/documents", s.requireAuth(http.HandlerFunc(s.listDocuments)))
	s.handle("GET /api/v1/workspaces/{id}/export.zip", s.requireAuth(http.HandlerFunc(s.exportWorkspace)))
	s.handle("GET /api/v1/workspaces/{id}/tags", s.requireAuth(http.HandlerFunc(s.listWorkspaceTags)))
	s.handle("PUT /api/v1/documents/{id}/tags", s.requireAuth(http.HandlerFunc(s.setDocumentTags)))
	s.handle("GET /api/v1/workspaces/{id}/templates", s.requireAuth(http.HandlerFunc(s.listTemplates)))
	s.handle("POST /api/v1/workspaces/{id}/templates", s.requireAuth(http.HandlerFunc(s.createTemplate)))
	s.handle("PATCH /api/v1/templates/{id}", s.requireAuth(http.HandlerFunc(s.updateTemplate)))
	s.handle("DELETE /api/v1/templates/{id}", s.requireAuth(http.HandlerFunc(s.deleteTemplate)))

	s.handle("POST /api/v1/documents", s.requireAuth(http.HandlerFunc(s.createDocument)))
	s.handle("GET /api/v1/documents", s.requireAuth(http.HandlerFunc(s.listUserDocuments)))
	s.handle("POST /api/v1/import", s.requireAuth(http.HandlerFunc(s.importDocument)))
	s.handle("GET /api/v1/documents/{id}", s.requireAuth(http.HandlerFunc(s.getDocument)))
	s.handle("PUT /api/v1/documents/{id}", s.requireAuth(http.HandlerFunc(s.updateDocument)))
	s.handle("DELETE /api/v1/documents/{id}", s.requireAuth(http.HandlerFunc(s.deleteDocument)))
	s.handle("POST /api/v1/documents/bulk", s.requireAuth(http.HandlerFunc(s.bulkDocuments)))
	s.handle("POST /api/v1/documents/{id}/duplicate", s.requireAuth(http.HandlerFunc(s.duplicateDocument)))
	s.handle("POST /api/v1/documents/{id}/move", s.requireAuth(http.HandlerFunc(s.moveDocument)))
	s.handle("POST /api/v1/documents/{id}/restore", s.requireAuth(http.HandlerFunc(s.restoreDocument)))
	s.handle("POST /api/v1/documents/{id}/favorite", s.requireAuth(http.HandlerFunc(s.favoriteDocument)))
	s.handle("DELETE /api/v1/documents/{id}/favorite", s.requireAuth(http.HandlerFunc(s.unfavoriteDocument)))
	s.handle("GET /api/v1/documents/{id}/permissions", s.requireAuth(http.HandlerFunc(s.listDocumentPermissions)))
	s.handle("PUT /api/v1/documents/{id}/permissions", s.requireAuth(http.HandlerFunc(s.upsertDocumentPermission)))
	s.handle("DELETE /api/v1/documents/{id}/permissions/{permissionId}", s.requireAuth(http.HandlerFunc(s.deleteDocumentPermission)))
	s.handle("GET /api/v1/documents/{id}/links", s.requireAuth(http.HandlerFunc(s.listDocumentLinks)))
	s.handle("POST /api/v1/documents/{id}/links", s.requireAuth(http.HandlerFunc(s.createDocumentLink)))
	s.handle("DELETE /api/v1/documents/{id}/links/{linkId}", s.requireAuth(http.HandlerFunc(s.revokeDocumentLink)))
	// Outside requireAuth on purpose: the person holding the link has no
	// account here. Everything that would normally be decided by who they are
	// is decided by the link row instead.
	s.handleFunc("POST /api/v1/public/documents/{token}", s.openPublicDocument)
	s.handle("GET /api/v1/documents/{id}/revisions", s.requireAuth(http.HandlerFunc(s.listRevisions)))
	s.handle("POST /api/v1/documents/{id}/revisions/{revision}/restore", s.requireAuth(http.HandlerFunc(s.restoreRevision)))
	s.handle("PATCH /api/v1/documents/{id}/revisions/{revision}", s.requireAuth(http.HandlerFunc(s.nameRevision)))
	s.handle("GET /api/v1/documents/{id}/revisions/{from}/diff/{to}", s.requireAuth(http.HandlerFunc(s.compareRevisions)))
	s.handle("GET /api/v1/documents/{id}/comments", s.requireAuth(http.HandlerFunc(s.listComments)))
	s.handle("POST /api/v1/documents/{id}/comments", s.requireAuth(http.HandlerFunc(s.createComment)))
	s.handle("POST /api/v1/comments/{id}/resolve", s.requireAuth(http.HandlerFunc(s.resolveComment)))
	s.handle("POST /api/v1/comments/{id}/reopen", s.requireAuth(http.HandlerFunc(s.reopenComment)))
	s.handle("GET /api/v1/documents/{id}/suggestions", s.requireAuth(http.HandlerFunc(s.listSuggestions)))
	s.handle("POST /api/v1/documents/{id}/suggestions", s.requireAuth(http.HandlerFunc(s.createSuggestion)))
	s.handle("POST /api/v1/documents/{id}/ai/patch", s.requireAuth(http.HandlerFunc(s.proposeDocumentPatch)))
	s.handle("POST /api/v1/suggestions/{id}/decision", s.requireAuth(http.HandlerFunc(s.decideSuggestion)))
	s.handle("POST /api/v1/documents/{id}/workflow/submit", s.requireAuth(http.HandlerFunc(s.submitApproval)))
	s.handle("POST /api/v1/approvals/{id}/decision", s.requireAuth(http.HandlerFunc(s.decideApproval)))
	s.handle("GET /api/v1/approvals", s.requireAuth(http.HandlerFunc(s.listApprovals)))
	s.handle("GET /api/v1/search", s.requireAuth(http.HandlerFunc(s.searchDocuments)))
	s.handle("GET /api/v1/documents/{id}/export/{format}", s.requireAuth(http.HandlerFunc(s.exportDocument)))
	s.handle("POST /api/v1/documents/{id}/presentations", s.requireAuth(http.HandlerFunc(s.createPresentation)))
	s.handle("GET /api/v1/documents/{id}/presentations", s.requireAuth(http.HandlerFunc(s.listPresentations)))
	s.handle("GET /api/v1/documents/{id}/presentations/{presentationId}/status", s.requireAuth(http.HandlerFunc(s.refreshPresentation)))
	s.handle("GET /api/v1/documents/{id}/presentations/{presentationId}/download", s.requireAuth(http.HandlerFunc(s.downloadPresentation)))
	s.handle("GET /api/v1/documents/{id}/presentations/{presentationId}/sync", s.requireAuth(http.HandlerFunc(s.planPresentationSync)))
	s.handle("POST /api/v1/documents/{id}/presentations/{presentationId}/sync", s.requireAuth(http.HandlerFunc(s.applyPresentationSync)))
	s.handle("POST /api/v1/documents/{id}/presentations/{presentationId}/citations", s.requireAuth(http.HandlerFunc(s.citePresentation)))
	s.handle("DELETE /api/v1/documents/{id}/presentations/{presentationId}", s.requireAuth(http.HandlerFunc(s.unlinkPresentation)))
	s.handle("GET /api/v1/documents/{id}/attachments", s.requireAuth(http.HandlerFunc(s.listAttachments)))
	s.handle("POST /api/v1/documents/{id}/attachments", s.requireAuth(http.HandlerFunc(s.uploadAttachment)))
	s.handle("GET /api/v1/attachments/{id}", s.requireAuth(http.HandlerFunc(s.downloadAttachment)))
	s.handle("DELETE /api/v1/attachments/{id}", s.requireAuth(http.HandlerFunc(s.deleteAttachment)))
	s.handle("GET /api/v1/collab/{id}", s.requireAuth(http.HandlerFunc(s.collaboration)))

	s.handle("GET /api/v1/notifications", s.requireAuth(http.HandlerFunc(s.listNotifications)))
	s.handle("POST /api/v1/notifications/{id}/read", s.requireAuth(http.HandlerFunc(s.readNotification)))
	s.handle("POST /api/v1/notifications/read-all", s.requireAuth(http.HandlerFunc(s.readAllNotifications)))
	s.handle("POST /api/v1/me/password", s.requireAuth(http.HandlerFunc(s.changeOwnPassword)))
	s.handle("GET /api/v1/me/keys", s.requireAuth(http.HandlerFunc(s.listUserKeys)))
	s.handle("POST /api/v1/me/keys/rotate", s.requireAuth(http.HandlerFunc(s.rotateUserKey)))
	s.handle("DELETE /api/v1/me/keys/{id}", s.requireAuth(http.HandlerFunc(s.revokeUserKey)))
	s.handle("GET /api/v1/me/api-keys", s.requireAuth(http.HandlerFunc(s.listAPIKeys)))
	s.handle("POST /api/v1/me/api-keys", s.requireAuth(http.HandlerFunc(s.createAPIKey)))
	s.handle("DELETE /api/v1/me/api-keys/{id}", s.requireAuth(http.HandlerFunc(s.revokeAPIKey)))
	s.handle("POST /api/v1/ai/chat", s.requireAuth(http.HandlerFunc(s.aiChat)))

	s.handle("POST /mcp", s.requireAuth(http.HandlerFunc(s.mcp)))
	s.handle("POST /api/v1/mcp", s.requireAuth(http.HandlerFunc(s.mcp)))
	s.handle("GET /.well-known/oauth-protected-resource", http.HandlerFunc(s.mcpProtectedResourceMetadata))

	s.handle("GET /api/v1/admin/settings", s.requireAdmin(http.HandlerFunc(s.getSettings)))
	s.handle("PUT /api/v1/admin/settings", s.requireAdmin(http.HandlerFunc(s.saveSettings)))
	s.handle("POST /api/v1/admin/settings/test-oidc", s.requireAdmin(http.HandlerFunc(s.testOIDC)))
	s.handle("POST /api/v1/admin/settings/test-ai", s.requireAdmin(http.HandlerFunc(s.testAI)))
	s.handle("POST /api/v1/admin/settings/test-ptium", s.requireAdmin(http.HandlerFunc(s.testPtium)))
	s.handle("POST /api/v1/admin/settings/test-smtp", s.requireAdmin(http.HandlerFunc(s.testSMTP)))
	s.handle("GET /api/v1/admin/users", s.requireAdmin(http.HandlerFunc(s.listUsers)))
	s.handle("POST /api/v1/admin/users", s.requireAdmin(http.HandlerFunc(s.createUser)))
	s.handle("POST /api/v1/admin/users/import", s.requireAdmin(http.HandlerFunc(s.importUsers)))
	s.handle("GET /api/v1/admin/users/{id}/belongings", s.requireAdmin(http.HandlerFunc(s.userBelongings)))
	s.handle("POST /api/v1/admin/users/{id}/offboard", s.requireAdmin(http.HandlerFunc(s.offboardUser)))
	s.handle("PATCH /api/v1/admin/users/{id}", s.requireAdmin(http.HandlerFunc(s.updateUser)))
	s.handle("GET /api/v1/admin/users/{id}/keys", s.requireAdmin(http.HandlerFunc(s.listAnyUserKeys)))
	s.handle("POST /api/v1/admin/users/{id}/keys/rotate", s.requireAdmin(http.HandlerFunc(s.rotateAnyUserKey)))
	s.handle("DELETE /api/v1/admin/users/{id}/keys/{keyId}", s.requireAdmin(http.HandlerFunc(s.revokeAnyUserKey)))
	s.handle("GET /metrics", s.requireAdmin(http.HandlerFunc(s.serveMetrics)))
	s.handle("GET /api/v1/admin/overview", s.requireAdmin(http.HandlerFunc(s.adminOverview)))
	s.handle("GET /api/v1/admin/retention/preview", s.requireAdmin(http.HandlerFunc(s.previewRetention)))
	s.handle("POST /api/v1/admin/retention/run", s.requireAdmin(http.HandlerFunc(s.runRetention)))
	s.handle("GET /api/v1/admin/workspaces", s.requireAdmin(http.HandlerFunc(s.adminWorkspaces)))
	s.handle("POST /api/v1/admin/workspaces/{id}/transfer", s.requireAdmin(http.HandlerFunc(s.adminTransferWorkspace)))
	s.handle("DELETE /api/v1/admin/workspaces/{id}", s.requireAdmin(http.HandlerFunc(s.adminArchiveWorkspace)))
	s.handle("POST /api/v1/admin/workspaces/{id}/restore", s.requireAdmin(http.HandlerFunc(s.adminRestoreWorkspace)))
	s.handle("GET /api/v1/admin/documents", s.requireAdmin(http.HandlerFunc(s.adminListDocuments)))
	s.handle("POST /api/v1/admin/documents/{id}/transfer", s.requireAdmin(http.HandlerFunc(s.adminTransferDocument)))
	s.handle("GET /api/v1/admin/documents/{id}/access", s.requireAdmin(http.HandlerFunc(s.documentAccess)))
	s.handle("DELETE /api/v1/admin/documents/{id}", s.requireAdmin(http.HandlerFunc(s.adminPurgeDocument)))
	s.handle("POST /api/v1/admin/users/{id}/password", s.requireAdmin(http.HandlerFunc(s.resetUserPassword)))
	s.handle("GET /api/v1/admin/users/{id}/sessions", s.requireAdmin(http.HandlerFunc(s.adminUserSessions)))
	s.handle("DELETE /api/v1/admin/users/{id}/sessions", s.requireAdmin(http.HandlerFunc(s.adminRevokeUserSessions)))
	s.handle("GET /api/v1/admin/audit", s.requireAdmin(http.HandlerFunc(s.listAudit)))
	s.handle("GET /api/v1/admin/audit.csv", s.requireAdmin(http.HandlerFunc(s.exportAudit)))
	s.handle("GET /api/v1/admin/ai-usage", s.requireAdmin(http.HandlerFunc(s.listAIUsage)))
	s.handle("GET /api/v1/admin/key-policies", s.requireAdmin(http.HandlerFunc(s.listKeyPolicies)))
	s.handle("PUT /api/v1/admin/key-policies/{role}", s.requireAdmin(http.HandlerFunc(s.updateKeyPolicy)))

	s.handleFunc("/", s.static)
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
		// The status is not readable from a ResponseWriter after the fact, so
		// it is remembered on the way past.
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		elapsed := time.Since(started)
		if r.URL.Path != "/healthz" && r.URL.Path != "/readyz" {
			s.logger.Info("http request", "method", r.Method, "path", r.URL.Path,
				"status", recorder.status, "duration_ms", elapsed.Milliseconds())
		}
		// The scrape itself is left out: counting it makes a graph of how
		// often it is scraped, which nobody wanted to know.
		if r.URL.Path != "/metrics" {
			s.metrics.observe(r.Method, recorder.status, elapsed)
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
