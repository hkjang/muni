package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/richdoc"
)

type revisionRef struct {
	Revision  int       `json:"revision"`
	CreatedAt time.Time `json:"createdAt"`
	Reason    *string   `json:"reason,omitempty"`
	Name      *string   `json:"name,omitempty"`
	Author    struct {
		ID          uuid.UUID `json:"id"`
		DisplayName string    `json:"displayName"`
	} `json:"author"`
	content json.RawMessage
}

// compareRevisions reports what changed between two revisions of a document,
// block by block rather than as a text diff, so a reader sees which paragraph,
// heading, table or image moved, changed, appeared or went away.
func (s *Server) compareRevisions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	if _, err := s.documentRole(r.Context(), p.User, id, false); err != nil {
		writeError(w, 403, "DOCUMENT_PERMISSION_DENIED", "문서에 접근할 권한이 없습니다.")
		return
	}
	from, ok := pathRevision(w, r, "from")
	if !ok {
		return
	}
	to, ok := pathRevision(w, r, "to")
	if !ok {
		return
	}

	before, err := s.loadRevision(r.Context(), id, from)
	if err != nil {
		writeError(w, 404, "REVISION_NOT_FOUND", "비교할 버전을 찾을 수 없습니다.")
		return
	}
	after, err := s.loadRevision(r.Context(), id, to)
	if err != nil {
		writeError(w, 404, "REVISION_NOT_FOUND", "비교할 버전을 찾을 수 없습니다.")
		return
	}

	beforeDocument, err := richdoc.Parse(before.content)
	if err != nil {
		writeError(w, 500, "REVISION_UNREADABLE", "버전 내용을 읽지 못했습니다.")
		return
	}
	afterDocument, err := richdoc.Parse(after.content)
	if err != nil {
		writeError(w, 500, "REVISION_UNREADABLE", "버전 내용을 읽지 못했습니다.")
		return
	}

	result := richdoc.Diff(beforeDocument, afterDocument)
	writeData(w, 200, map[string]any{
		"from":    before,
		"to":      after,
		"summary": result.Summary,
		"blocks":  result.Blocks,
	})
	s.audit(r, &p.User.ID, "COMPARE_REVISIONS", "DOCUMENT", &id, map[string]any{"from": from, "to": to})
}

func pathRevision(w http.ResponseWriter, r *http.Request, name string) (int, bool) {
	value, err := strconv.Atoi(r.PathValue(name))
	if err != nil || value < 1 {
		writeError(w, 400, "INVALID_REVISION", "버전 번호가 올바르지 않습니다.")
		return 0, false
	}
	return value, true
}

func (s *Server) loadRevision(ctx context.Context, documentID uuid.UUID, revision int) (*revisionRef, error) {
	var ref revisionRef
	err := s.db.QueryRow(ctx,
		`SELECT dr.revision_no,dr.created_at,dr.reason,dr.name,dr.content_json,u.id,u.display_name
		 FROM document_revisions dr JOIN users u ON u.id=dr.author_id
		 WHERE dr.document_id=$1 AND dr.revision_no=$2`,
		documentID, revision).
		Scan(&ref.Revision, &ref.CreatedAt, &ref.Reason, &ref.Name, &ref.content, &ref.Author.ID, &ref.Author.DisplayName)
	if err != nil {
		return nil, err
	}
	return &ref, nil
}
