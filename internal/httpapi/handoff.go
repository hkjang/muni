package httpapi

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/cryptoutil"
	"github.com/hkjang/muni/internal/handoff"
)

// Handing a document to another service, both ways, in the shape every
// service on the network follows (HANDOFF-STANDARD).
//
// Sending: a signed-in user asks for a claim on a document they can read. The
// document is rendered then and there and kept with the claim, so what the
// other service takes is what the user saw when they pressed the button. The
// claim is the only credential for taking it — so it is random, bound to that
// one rendering, single-use, and gone in five minutes — and it is stored as a
// hash, never written to a log, and never shown in an audit row.
//
// Receiving: a browser arrives at /handoff with a source and a claim. The
// source is a value from outside; it is compared with the administrator's
// list before anything is requested, because a service that fetches whatever
// address it is handed is a way to reach every address on the network.

// handoffPagePath is where a peer sends the browser.
const handoffPagePath = "/handoff"

// handoffTransport is shared so connections to a peer are pooled.
var handoffTransport = &http.Transport{Proxy: http.ProxyFromEnvironment, ResponseHeaderTimeout: handoff.FetchTimeout, MaxIdleConnsPerHost: 4, IdleConnTimeout: 90 * time.Second}

// handoffConfig reads the peer list. A failure is "no peers": nothing is
// sent and nothing is accepted.
func (s *Server) handoffConfig(ctx context.Context) handoff.Config {
	all, err := s.settings.GetAll(ctx, false)
	if err != nil {
		return handoff.Config{}
	}
	return all.Handoff
}

// issueHandoffClaim makes a claim for one document in one format. The
// document is rendered now, the way the export endpoint renders it, and the
// rendering is what the claim hands out.
func (s *Server) issueHandoffClaim(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Resource string `json:"resource"`
		Format   string `json:"format"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(input.Resource))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "올바르지 않은 식별자입니다.")
		return
	}
	format, ok := handoff.NormalizeFormat(input.Format)
	if !ok {
		writeError(w, http.StatusBadRequest, "UNSUPPORTED_HANDOFF_FORMAT", "다른 서비스로 보낼 수 있는 형식은 markdown, docx 입니다.")
		return
	}
	p, _ := principalFrom(r.Context())
	if _, err := s.documentRole(r.Context(), p.User, id, false); err != nil {
		// The same answer for "no such document" and "not yours": a claim
		// must not be a way to learn which ids exist.
		writeError(w, http.StatusNotFound, "DOCUMENT_NOT_FOUND", "문서를 찾을 수 없습니다.")
		return
	}
	all, err := s.settings.GetAll(r.Context(), false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SETTINGS_ERROR", "설정을 불러오지 못했습니다.")
		return
	}
	if format == handoff.FormatDOCX && !all.Export.EnableDOCX {
		writeError(w, http.StatusForbidden, "DOCX_EXPORT_DISABLED", "관리자 정책에서 DOCX 내보내기가 비활성화되어 있습니다.")
		return
	}
	exportFormat := map[string]string{handoff.FormatMarkdown: "md", handoff.FormatDOCX: "docx"}[format]
	file, found, err := s.renderExport(r.Context(), id, exportFormat, p.User.DisplayName)
	if !found {
		writeError(w, http.StatusNotFound, "DOCUMENT_NOT_FOUND", "문서를 찾을 수 없습니다.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "EXPORT_FAILED", "문서를 내보내지 못했습니다: "+err.Error())
		return
	}
	claim, err := cryptoutil.RandomToken(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "HANDOFF_FAILED", "표를 만들지 못했습니다.")
		return
	}
	filename := file.filename + handoff.Extension(format)
	contentType := handoff.MediaType(format)
	expires := time.Now().Add(handoff.ClaimTTL)
	// Expired claims are swept as new ones are made: a table of five-minute
	// rows needs no scheduler of its own.
	_, _ = s.db.Exec(r.Context(), `DELETE FROM handoff_claims WHERE expires_at<now()`)
	if _, err := s.db.Exec(r.Context(), `INSERT INTO handoff_claims(claim_hash,document_id,user_id,format,filename,content_type,body,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		cryptoutil.SHA256(claim), id, p.User.ID, format, filename, contentType, file.body, expires); err != nil {
		writeError(w, http.StatusInternalServerError, "HANDOFF_FAILED", "표를 저장하지 못했습니다.")
		return
	}
	s.audit(r, &p.User.ID, "HANDOFF_ISSUE", "DOCUMENT", &id, map[string]any{"format": format, "bytes": len(file.body)})
	writeJSON(w, http.StatusCreated, map[string]any{
		"claim":        claim,
		"source":       requestOrigin(r),
		"filename":     filename,
		"content_type": contentType,
		"bytes":        len(file.body),
		"expires_at":   expires.Format(time.RFC3339),
	})
}

// serveHandoffClaim hands the rendering out to whoever holds the claim. There
// is no login here — the claim is the credential — so the row is deleted in
// the same statement that reads it, and a claim that is gone, used or expired
// gets the same 404.
func (s *Server) serveHandoffClaim(w http.ResponseWriter, r *http.Request) {
	claim := r.PathValue("claim")
	if !handoff.ValidClaim(claim) {
		writeError(w, http.StatusNotFound, "HANDOFF_CLAIM_NOT_FOUND", "표를 찾을 수 없습니다.")
		return
	}
	var documentID, userID uuid.UUID
	var format, filename, contentType string
	var body []byte
	err := s.db.QueryRow(r.Context(), `DELETE FROM handoff_claims WHERE claim_hash=$1 AND expires_at>now() RETURNING document_id,user_id,format,filename,content_type,body`,
		cryptoutil.SHA256(claim)).Scan(&documentID, &userID, &format, &filename, &contentType, &body)
	if err != nil {
		writeError(w, http.StatusNotFound, "HANDOFF_CLAIM_NOT_FOUND", "표를 찾을 수 없습니다.")
		return
	}
	w.Header().Set("Content-Type", contentType)
	// The name is percent-encoded in full (RFC 8187), not just its awkward
	// characters: the taker is a program parsing the header, not a browser
	// guessing at raw UTF-8, and a Korean title must survive the trip. It goes
	// through the same escaper as every other download here — url.PathEscape
	// was close but leaves `=`, `:` and `@` standing, and an `=` inside the
	// parameter is enough for a header parser to reject the whole value.
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="document%s"; filename*=UTF-8''%s`, filepath.Ext(filename), extValueEscape(filename)))
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
	// The trail says whose document went out and in what format; the actor
	// is the person who made the claim, since the taker has no account here.
	s.audit(r, &userID, "HANDOFF_SERVE", "DOCUMENT", &documentID, map[string]any{"format": format, "bytes": len(body)})
}

// receiveHandoff is where another service sends the browser. The person must
// be signed in here — the document becomes theirs — and the source must be on
// the list before a single byte is requested from it.
func (s *Server) receiveHandoff(w http.ResponseWriter, r *http.Request) {
	p, err := s.authenticate(r)
	if err != nil || p.User.Status != "ACTIVE" {
		http.Redirect(w, r, "/login?return_to="+url.QueryEscape(handoffPagePath+"?"+r.URL.RawQuery), http.StatusFound)
		return
	}
	if p.User.MustChangePassword {
		handoffPage(w, http.StatusForbidden, "임시 비밀번호를 먼저 바꿔 주세요", "비밀번호를 바꾼 뒤 보낸 쪽에서 다시 보내 주세요.")
		return
	}
	source := r.URL.Query().Get("source")
	claim := r.URL.Query().Get("claim")
	if source == "" || claim == "" {
		handoffPage(w, http.StatusBadRequest, "주소가 완전하지 않습니다", "보내는 서비스의 주소(source)와 표(claim)가 함께 있어야 합니다. 보낸 쪽에서 다시 시도해 주세요.")
		return
	}
	config := s.handoffConfig(r.Context())
	document, err := handoff.Fetch(r.Context(), handoff.NewClient(handoffTransport), config, source, claim)
	if err != nil {
		status, title, message := handoffFailure(err, source)
		if !errors.Is(err, handoff.ErrNotAllowed) && !errors.Is(err, handoff.ErrBadClaim) {
			s.logger.Warn("handoff fetch failed", "source", handoff.OriginOf(source), "error", err.Error())
		}
		handoffPage(w, status, title, message)
		return
	}
	peer, _ := config.Allowed(source)
	documentID, err := s.storeHandoff(r.Context(), p.User.ID, peer, document)
	if err != nil {
		s.logger.Warn("handoff import failed", "source", peer.Origin, "error", err.Error())
		handoffPage(w, http.StatusBadRequest, "문서를 읽지 못했습니다", "받은 파일을 muni 문서로 바꾸지 못했습니다: "+err.Error())
		return
	}
	s.audit(r, &p.User.ID, "HANDOFF_RECEIVE", "DOCUMENT", &documentID, map[string]any{"source": peer.Origin, "format": document.Format, "filename": document.Filename, "bytes": len(document.Body)})
	http.Redirect(w, r, "/docs/"+documentID.String(), http.StatusFound)
}

// storeHandoff turns what a peer sent into a document in the person's own
// workspace. Where it came from is written into the first revision's reason,
// so the history panel and the database both answer "which canvas was this?"
func (s *Server) storeHandoff(ctx context.Context, ownerID uuid.UUID, peer handoff.Peer, document handoff.Document) (uuid.UUID, error) {
	var workspaceID uuid.UUID
	if err := s.db.QueryRow(ctx, `SELECT id FROM workspaces WHERE owner_id=$1 AND kind='PERSONAL' AND deleted_at IS NULL ORDER BY created_at LIMIT 1`, ownerID).Scan(&workspaceID); err != nil {
		return uuid.Nil, errors.New("개인 워크스페이스가 없습니다")
	}
	parsed, err := parseUpload(ctx, handoff.Extension(document.Format), document.Body)
	if err != nil {
		return uuid.Nil, err
	}
	attachments, content, err := prepareImportedAssets(parsed.assets, parsed.content)
	if err != nil {
		return uuid.Nil, err
	}
	content, err = withBlockIDs(content)
	if err != nil {
		return uuid.Nil, err
	}
	if !validDocumentJSON(content) {
		return uuid.Nil, errors.New("문서가 너무 큽니다")
	}
	title := strings.TrimSpace(parsed.title)
	if title == "" {
		title = strings.TrimSuffix(document.Filename, filepath.Ext(document.Filename))
	}
	if title == "" {
		title = "받은 문서"
	}
	title = truncateRunes(title, 240)
	// A peer that is itself a muni wrote the title as the first heading of the
	// Markdown it sent; the same rule as the upload keeps it from showing twice.
	if parsed.titleInBody {
		if content, err = dropLeadingTitle(content, title); err != nil {
			return uuid.Nil, err
		}
	}
	documentID := uuid.New()
	err = s.storeImportedDocument(ctx, importedDocument{
		id: documentID, workspaceID: workspaceID, ownerID: ownerID,
		title: title, visibility: "RESTRICTED", content: content, text: extractDocumentText(content),
		furniture: parsed.furniture, reason: "handoff:" + peer.Origin, attachments: attachments,
	})
	if err != nil {
		return uuid.Nil, errors.New("문서를 저장하지 못했습니다")
	}
	return documentID, nil
}

// handoffFailure says in words what went wrong, for the page a person sees.
// The source is shown only when it was one the list refused: that is the one
// case where the person can do something about it (ask an administrator).
func handoffFailure(err error, source string) (int, string, string) {
	switch {
	case errors.Is(err, handoff.ErrNotAllowed):
		return http.StatusForbidden, "허용되지 않은 서비스입니다",
			fmt.Sprintf("%s 은(는) 관리자가 문서를 받도록 허용한 서비스가 아닙니다. 서비스 관리 → 서비스 설정 → 문서 넘기기에서 더할 수 있습니다.", truncateRunes(source, 200))
	case errors.Is(err, handoff.ErrBadClaim), errors.Is(err, handoff.ErrNotFound):
		return http.StatusNotFound, "표가 만료됐거나 이미 쓰였습니다",
			"표는 5분 동안 한 번만 쓸 수 있습니다. 보낸 쪽에서 다시 보내 주세요."
	case errors.Is(err, handoff.ErrRedirect):
		return http.StatusBadGateway, "보내는 서비스가 다른 곳으로 넘기려 했습니다",
			"muni 는 리다이렉트를 따라가지 않습니다. 보내는 서비스의 주소가 허용 목록에 적힌 그대로인지 관리자에게 확인해 주세요."
	case errors.Is(err, handoff.ErrUnsupported):
		return http.StatusUnsupportedMediaType, "읽을 수 없는 형식입니다",
			"muni 가 다른 서비스에서 받을 수 있는 형식은 Markdown 뿐입니다."
	case errors.Is(err, handoff.ErrTooLarge):
		return http.StatusRequestEntityTooLarge, "문서가 너무 큽니다",
			fmt.Sprintf("다른 서비스에서 받는 문서는 %dMB 까지입니다.", handoff.MaxBodyBytes>>20)
	default:
		return http.StatusBadGateway, "보내는 서비스에 닿지 못했습니다",
			"보내는 서비스가 응답하지 않았거나 30초 안에 답하지 않았습니다. 잠시 뒤 다시 시도해 주세요."
	}
}

// handoffPage is the screen a person sees when a handoff did not end in a
// document. It is plain HTML because there is no application here yet — the
// browser was sent by another service — and it stays within the page policy.
func handoffPage(w http.ResponseWriter, status int, title, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `<!doctype html><html lang="ko"><head><meta charset="utf-8"><title>%s</title><style>body{font-family:"Noto Sans KR","Malgun Gothic",sans-serif;max-width:560px;margin:15vh auto;padding:0 24px;color:#202124;line-height:1.6}h1{font-size:1.4rem}a{color:#1a56c4}</style></head><body><h1>%s</h1><p>%s</p><p><a href="/">muni 로 돌아가기</a></p></body></html>`,
		html.EscapeString(title), html.EscapeString(title), html.EscapeString(message))
}

// requestOrigin is this service as the browser reached it — what a peer
// must put in its allow list and fetch the claim from.
func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// logPath is the request path as it may be written to the log. A claim is a
// credential and lives in the path of the endpoint that redeems it, so that
// segment is replaced before the line is written.
func logPath(path string) string {
	const prefix = handoff.ClaimsPath + "/"
	if strings.HasPrefix(path, prefix) {
		return prefix + "{claim}"
	}
	return path
}
