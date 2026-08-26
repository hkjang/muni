package httpapi

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/database"
	"github.com/jackc/pgx/v5"
)

// Tags are how a workspace groups documents across folders: 상반기, 대외비,
// 기획팀 검토. The tables and the search that reads them have existed from the
// start, and there was no way to put a tag on anything.

// maxTagsPerDocument keeps a document's tag list readable, and keeps one
// import from creating a hundred tags nobody chose.
const maxTagsPerDocument = 20

// listWorkspaceTags returns the tags a workspace has, with how many documents
// carry each — which is what says whether a tag is in use or was a one-off.
func (s *Server) listWorkspaceTags(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	if !s.workspaceMember(r.Context(), workspaceID, p.User) {
		writeError(w, 403, "WORKSPACE_PERMISSION_DENIED", "워크스페이스에 접근할 권한이 없습니다.")
		return
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT t.id, t.name, t.color,
			(SELECT count(*) FROM document_tags dt JOIN documents d ON d.id = dt.document_id
			 WHERE dt.tag_id = t.id AND d.deleted_at IS NULL)
		FROM tags t WHERE t.workspace_id = $1 ORDER BY t.name`, workspaceID)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "태그를 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0, 32)
	for rows.Next() {
		var id uuid.UUID
		var name, color string
		var documents int64
		if rows.Scan(&id, &name, &color, &documents) == nil {
			items = append(items, map[string]any{
				"id": id, "name": name, "color": color, "documents": documents,
			})
		}
	}
	writeData(w, 200, items)
}

// setDocumentTags replaces a document's tags with the names given.
//
// Names rather than ids: a person types a tag, and whether it already exists
// in the workspace is muni's problem, not theirs. A name that is new becomes a
// tag; one that is not is reused, so the same word does not become three tags
// with different capitalisation.
func (s *Server) setDocumentTags(w http.ResponseWriter, r *http.Request) {
	documentID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	role, err := s.documentRole(r.Context(), p.User, documentID, false)
	if err != nil || !requireDocumentRole(w, role, "EDITOR") {
		return
	}
	var input struct {
		Tags []string `json:"tags"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}

	names := normalizeTagNames(input.Tags)
	if len(names) > maxTagsPerDocument {
		writeError(w, 400, "TOO_MANY_TAGS", "태그는 문서당 20개까지입니다.")
		return
	}

	var workspaceID uuid.UUID
	if err := s.db.QueryRow(r.Context(), `SELECT workspace_id FROM documents WHERE id=$1`, documentID).Scan(&workspaceID); err != nil {
		writeError(w, 404, "DOCUMENT_NOT_FOUND", "문서를 찾을 수 없습니다.")
		return
	}

	err = database.WithTx(r.Context(), s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(), `DELETE FROM document_tags WHERE document_id=$1`, documentID); err != nil {
			return err
		}
		for _, name := range names {
			var tagID uuid.UUID
			// citext and the unique constraint together make the name the
			// identity, so a race between two people adding the same tag ends
			// with one tag rather than an error.
			if err := tx.QueryRow(r.Context(),
				`INSERT INTO tags(workspace_id,name) VALUES($1,$2)
				 ON CONFLICT (workspace_id,name) DO UPDATE SET name=excluded.name
				 RETURNING id`, workspaceID, name).Scan(&tagID); err != nil {
				return err
			}
			if _, err := tx.Exec(r.Context(),
				`INSERT INTO document_tags(document_id,tag_id) VALUES($1,$2) ON CONFLICT DO NOTHING`,
				documentID, tagID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "태그를 저장하지 못했습니다.")
		return
	}
	s.audit(r, &p.User.ID, "SET_DOCUMENT_TAGS", "DOCUMENT", &documentID, map[string]any{"tags": names})
	writeData(w, 200, map[string]any{"tags": names})
}

// normalizeTagNames trims, drops the empty ones, folds duplicates and bounds
// the length, so what is stored is what a reader would call a tag.
func normalizeTagNames(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value)
		// A tag is a label, not a sentence.
		if name == "" || len([]rune(name)) > 40 {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, name)
	}
	return out
}
