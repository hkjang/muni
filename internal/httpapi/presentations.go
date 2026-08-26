package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/integration/ptium"
	"github.com/hkjang/muni/internal/richdoc"
	"github.com/hkjang/muni/internal/settings"
)

const maxPresentationsPerDocument = 20

// presentationLink is what muni stores: the relationship, not the deck.
type presentationLink struct {
	ID               uuid.UUID  `json:"id"`
	DocumentID       uuid.UUID  `json:"documentId"`
	DocumentRevision int        `json:"documentRevision"`
	Provider         string     `json:"provider"`
	PresentationID   string     `json:"presentationId"`
	Title            string     `json:"title"`
	Status           string     `json:"status"`
	SlideCount       int        `json:"slideCount"`
	TemplateID       *string    `json:"templateId,omitempty"`
	EditorURL        string     `json:"editorUrl,omitempty"`
	CreatedBy        uuid.UUID  `json:"createdBy"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	LastSyncedAt     *time.Time `json:"lastSyncedAt,omitempty"`
	Stale            bool       `json:"stale"`
}

func (s *Server) ptiumConfig(ctx context.Context) (ptium.Config, error) {
	all, err := s.settings.GetAll(ctx, true)
	if err != nil {
		return ptium.Config{}, err
	}
	return ptiumConfigFrom(all.Ptium), nil
}

func ptiumConfigFrom(from settings.Ptium) ptium.Config {
	return ptium.Config{
		Enabled:        from.Enabled,
		BaseURL:        from.BaseURL,
		WebURL:         from.WebURL,
		APIKey:         from.APIKey,
		DefaultTheme:   from.DefaultTheme,
		DefaultLang:    from.DefaultLocale,
		TimeoutSeconds: from.TimeoutSeconds,
	}.Normalize()
}

type createPresentationInput struct {
	Title      string `json:"title"`
	Audience   string `json:"audience"`
	Purpose    string `json:"purpose"`
	Tone       string `json:"tone"`
	Language   string `json:"language"`
	SlideCount int    `json:"slideCount"`
	Minutes    int    `json:"minutes"`
	Detail     string `json:"detail"`
	Theme      string `json:"theme"`
	TemplateID string `json:"templateId"`
}

func (input createPresentationInput) options() ptium.Options {
	return ptium.Options{
		Title:      strings.TrimSpace(input.Title),
		Audience:   strings.TrimSpace(input.Audience),
		Purpose:    strings.TrimSpace(input.Purpose),
		Tone:       strings.TrimSpace(input.Tone),
		Language:   strings.TrimSpace(input.Language),
		SlideCount: input.SlideCount,
		Minutes:    input.Minutes,
		Detail:     strings.TrimSpace(input.Detail),
		Theme:      strings.TrimSpace(input.Theme),
		TemplateID: strings.TrimSpace(input.TemplateID),
	}
}

// createPresentation turns the current revision of a document into a deck.
func (s *Server) createPresentation(w http.ResponseWriter, r *http.Request) {
	documentID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	role, err := s.documentRole(r.Context(), p.User, documentID, false)
	if !documentAllowed(w, role, err, "COMMENTER") {
		return
	}
	config, err := s.ptiumConfig(r.Context())
	if err != nil || !config.Usable() {
		writeError(w, 409, "PTIUM_NOT_CONFIGURED", "관리자 설정에서 발표자료 연동을 설정해 주세요.")
		return
	}
	var input createPresentationInput
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.SlideCount < 0 || input.SlideCount > 50 {
		writeError(w, 400, "INVALID_SLIDE_COUNT", "슬라이드 수는 1~50 사이여야 합니다.")
		return
	}

	var count int
	_ = s.db.QueryRow(r.Context(), `SELECT count(*) FROM document_presentations WHERE document_id=$1`, documentID).Scan(&count)
	if count >= maxPresentationsPerDocument {
		writeError(w, 400, "TOO_MANY_PRESENTATIONS", "이 문서에 연결된 발표자료가 너무 많습니다. 사용하지 않는 항목을 정리해 주세요.")
		return
	}

	brief, revision, err := s.documentBrief(r.Context(), documentID, input.options())
	if err != nil {
		writeError(w, 404, "DOCUMENT_NOT_FOUND", "문서를 찾을 수 없습니다.")
		return
	}

	client := ptium.NewClient(config)
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(config.TimeoutSeconds)*time.Second)
	defer cancel()
	presentation, err := client.Generate(ctx, brief, input.options())
	if err != nil {
		s.logger.Warn("ptium generation failed", "document_id", documentID, "error", err.Error())
		writeError(w, ptium.HTTPStatus(err), "PTIUM_REQUEST_FAILED", err.Error())
		return
	}

	id := uuid.New()
	optionsJSON, _ := json.Marshal(input)
	if _, err := s.db.Exec(r.Context(),
		`INSERT INTO document_presentations(id,document_id,document_revision,provider,presentation_id,title,status,slide_count,template_id,options,created_by,last_synced_at)
		 VALUES($1,$2,$3,'ptium',$4,$5,$6,$7,$8,$9,$10,now())`,
		id, documentID, revision, presentation.ID, presentation.Title, presentation.Status,
		presentation.SlideCount, nullString(presentation.TemplateID), optionsJSON, p.User.ID); err != nil {
		// The deck exists in Ptium but muni could not record it; say so rather
		// than leaving an orphan the person cannot reach.
		s.logger.Error("presentation link was not stored", "document_id", documentID, "presentation_id", presentation.ID, "error", err)
		writeError(w, 500, "PRESENTATION_LINK_FAILED", "발표자료는 생성되었지만 문서와 연결하지 못했습니다: "+presentation.ID)
		return
	}
	s.audit(r, &p.User.ID, "CREATE_PRESENTATION", "DOCUMENT", &documentID,
		map[string]any{"presentationId": presentation.ID, "revision": revision})

	link, err := s.presentationLink(r.Context(), id, config)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "발표자료 정보를 불러오지 못했습니다.")
		return
	}
	writeData(w, 201, link)
}

// listPresentations shows the decks made from this document.
func (s *Server) listPresentations(w http.ResponseWriter, r *http.Request) {
	documentID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	if _, err := s.documentRole(r.Context(), p.User, documentID, false); err != nil {
		writeError(w, 403, "DOCUMENT_PERMISSION_DENIED", "문서에 접근할 권한이 없습니다.")
		return
	}
	config, _ := s.ptiumConfig(r.Context())
	rows, err := s.db.Query(r.Context(), presentationSelect+` WHERE dp.document_id=$1 ORDER BY dp.created_at DESC`, documentID)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "발표자료 목록을 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]presentationLink, 0)
	for rows.Next() {
		link, err := scanPresentation(rows, config)
		if err == nil {
			items = append(items, link)
		}
	}
	writeData(w, 200, items)
}

// refreshPresentation asks Ptium how generation is going and records the answer.
func (s *Server) refreshPresentation(w http.ResponseWriter, r *http.Request) {
	link, config, ok := s.presentationForRequest(w, r, "VIEWER")
	if !ok {
		return
	}
	client := ptium.NewClient(config)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	presentation, err := client.Get(ctx, link.PresentationID)
	if err != nil {
		writeError(w, ptium.HTTPStatus(err), "PTIUM_REQUEST_FAILED", err.Error())
		return
	}
	if _, err := s.db.Exec(r.Context(),
		`UPDATE document_presentations SET status=$2,slide_count=$3,title=$4,updated_at=now(),last_synced_at=now() WHERE id=$1`,
		link.ID, presentation.Status, presentation.SlideCount, presentation.Title); err != nil {
		s.logger.Warn("presentation status was not stored", "id", link.ID, "error", err)
	}
	refreshed, err := s.presentationLink(r.Context(), link.ID, config)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "발표자료 정보를 불러오지 못했습니다.")
		return
	}
	writeData(w, 200, map[string]any{
		"presentation":    refreshed,
		"generationNotes": presentation.GenerationNotes,
	})
}

// downloadPresentation streams the built file from Ptium. muni does not keep a
// copy: the deck can be edited in Ptium afterwards, and a stored file would go
// stale the moment that happens.
func (s *Server) downloadPresentation(w http.ResponseWriter, r *http.Request) {
	link, config, ok := s.presentationForRequest(w, r, "VIEWER")
	if !ok {
		return
	}
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format != "pdf" {
		format = "pptx"
	}
	client := ptium.NewClient(config)
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(config.TimeoutSeconds)*time.Second)
	defer cancel()
	body, contentType, err := client.Export(ctx, link.PresentationID, format)
	if err != nil {
		writeError(w, ptium.HTTPStatus(err), "PTIUM_EXPORT_FAILED", err.Error())
		return
	}
	defer body.Close()

	p, _ := principalFrom(r.Context())
	filename := safeFilename(link.Title)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="presentation.%s"; filename*=UTF-8''%s.%s`,
		format, urlPathEscape(filename), format))
	w.WriteHeader(200)
	if _, err := io.Copy(w, body); err != nil {
		s.logger.Warn("presentation download was interrupted", "id", link.ID, "error", err)
		return
	}
	s.audit(r, &p.User.ID, "DOWNLOAD_PRESENTATION", "DOCUMENT", &link.DocumentID,
		map[string]any{"presentationId": link.PresentationID, "format": format})
}

// unlinkPresentation drops the link, and the deck with it unless the caller
// asked to keep it.
func (s *Server) unlinkPresentation(w http.ResponseWriter, r *http.Request) {
	link, config, ok := s.presentationForRequest(w, r, "EDITOR")
	if !ok {
		return
	}
	if r.URL.Query().Get("keepRemote") != "true" {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err := ptium.NewClient(config).Delete(ctx, link.PresentationID); err != nil {
			// A deck already gone from Ptium should still unlink here.
			s.logger.Warn("ptium deck was not deleted", "presentation_id", link.PresentationID, "error", err)
		}
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM document_presentations WHERE id=$1`, link.ID); err != nil {
		writeError(w, 500, "DATABASE_ERROR", "연결을 해제하지 못했습니다.")
		return
	}
	p, _ := principalFrom(r.Context())
	s.audit(r, &p.User.ID, "DELETE_PRESENTATION", "DOCUMENT", &link.DocumentID,
		map[string]any{"presentationId": link.PresentationID})
	w.WriteHeader(204)
}

// documentBrief reads the document and converts it for a generator.
func (s *Server) documentBrief(ctx context.Context, documentID uuid.UUID, options ptium.Options) (ptium.Brief, int, error) {
	var title string
	var revision int
	var content json.RawMessage
	if err := s.db.QueryRow(ctx, `SELECT title,revision_no,content_json FROM documents WHERE id=$1`, documentID).
		Scan(&title, &revision, &content); err != nil {
		return ptium.Brief{}, 0, err
	}
	document, err := richdoc.Parse(content)
	if err != nil {
		return ptium.Brief{}, 0, err
	}
	source := ptium.BriefSource{
		Type: "muni", DocumentID: documentID.String(), Revision: revision, Title: title,
	}
	return ptium.BuildBrief(document, source, options), revision, nil
}

const presentationSelect = `SELECT dp.id,dp.document_id,dp.document_revision,dp.provider,dp.presentation_id,dp.title,
	dp.status,dp.slide_count,dp.template_id,dp.created_by,dp.created_at,dp.updated_at,dp.last_synced_at,d.revision_no
	FROM document_presentations dp JOIN documents d ON d.id=dp.document_id`

type scanner interface {
	Scan(dest ...any) error
}

func scanPresentation(row scanner, config ptium.Config) (presentationLink, error) {
	var link presentationLink
	var currentRevision int
	if err := row.Scan(&link.ID, &link.DocumentID, &link.DocumentRevision, &link.Provider, &link.PresentationID,
		&link.Title, &link.Status, &link.SlideCount, &link.TemplateID, &link.CreatedBy, &link.CreatedAt,
		&link.UpdatedAt, &link.LastSyncedAt, &currentRevision); err != nil {
		return presentationLink{}, err
	}
	// The deck was built from an older revision, so the document has moved on.
	link.Stale = currentRevision > link.DocumentRevision
	link.EditorURL = config.EditorURL(link.PresentationID)
	return link, nil
}

func (s *Server) presentationLink(ctx context.Context, id uuid.UUID, config ptium.Config) (presentationLink, error) {
	row := s.db.QueryRow(ctx, presentationSelect+` WHERE dp.id=$1`, id)
	return scanPresentation(row, config)
}

// presentationForRequest resolves the link named by the path and checks that
// the caller may act on the document behind it.
func (s *Server) presentationForRequest(w http.ResponseWriter, r *http.Request, minimum string) (presentationLink, ptium.Config, bool) {
	documentID, ok := pathUUID(w, r, "id")
	if !ok {
		return presentationLink{}, ptium.Config{}, false
	}
	linkID, err := uuid.Parse(r.PathValue("presentationId"))
	if err != nil {
		writeError(w, 400, "INVALID_PRESENTATION", "발표자료 식별자가 올바르지 않습니다.")
		return presentationLink{}, ptium.Config{}, false
	}
	p, _ := principalFrom(r.Context())
	role, err := s.documentRole(r.Context(), p.User, documentID, false)
	if err != nil {
		writeError(w, 403, "DOCUMENT_PERMISSION_DENIED", "문서에 접근할 권한이 없습니다.")
		return presentationLink{}, ptium.Config{}, false
	}
	if minimum != "VIEWER" && !requireDocumentRole(w, role, minimum) {
		return presentationLink{}, ptium.Config{}, false
	}
	config, err := s.ptiumConfig(r.Context())
	if err != nil || !config.Usable() {
		writeError(w, 409, "PTIUM_NOT_CONFIGURED", "관리자 설정에서 발표자료 연동을 설정해 주세요.")
		return presentationLink{}, ptium.Config{}, false
	}
	link, err := s.presentationLink(r.Context(), linkID, config)
	if err != nil || link.DocumentID != documentID {
		writeError(w, 404, "PRESENTATION_NOT_FOUND", "연결된 발표자료를 찾을 수 없습니다.")
		return presentationLink{}, ptium.Config{}, false
	}
	return link, config, true
}

// testPtium checks an administrator's connection settings before they are used.
func (s *Server) testPtium(w http.ResponseWriter, r *http.Request) {
	var input settings.Ptium
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.APIKey == "" {
		all, _ := s.settings.GetAll(r.Context(), true)
		input.APIKey = all.Ptium.APIKey
	}
	input.Enabled = true
	config := ptiumConfigFrom(input)
	if !config.Usable() {
		writeError(w, 400, "PTIUM_CONFIG_REQUIRED", "Ptium 주소와 API key가 필요합니다.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if err := ptium.NewClient(config).Ping(ctx); err != nil {
		var apiError *ptium.APIError
		if errors.As(err, &apiError) && (apiError.Status == 401 || apiError.Status == 403) {
			writeError(w, 400, "PTIUM_TEST_FAILED", "API key가 거절되었습니다: "+apiError.Message)
			return
		}
		writeError(w, 502, "PTIUM_TEST_FAILED", err.Error())
		return
	}
	writeData(w, 200, map[string]any{"ok": true, "baseUrl": config.BaseURL, "editorUrl": config.WebURL})
}
