package httpapi

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/database"
	"github.com/jackc/pgx/v5"
)

// Acting on several documents at once.
//
// Filing a month of meeting records, tagging everything for an audit, clearing
// out a finished project: all of them were one document at a time, which is
// why folders and tags were tidy in nobody's workspace.

// maxBulkDocuments bounds one request. Past this it is not a person selecting
// documents, and each one is a permission check of its own.
const maxBulkDocuments = 200

// bulkResult says what happened to one document. A batch reports per document
// rather than succeeding or failing as a whole: refusing to file forty
// documents because the forty-first is waiting for approval helps nobody, and
// silently skipping it would be worse.
type bulkResult struct {
	ID     uuid.UUID `json:"id"`
	Status string    `json:"status"`
	Reason string    `json:"reason,omitempty"`
}

func (s *Server) bulkDocuments(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	var input struct {
		IDs      []uuid.UUID `json:"ids"`
		Action   string      `json:"action"`
		FolderID **uuid.UUID `json:"folderId"`
		Tags     []string    `json:"tags"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Action = strings.ToLower(strings.TrimSpace(input.Action))
	if !contains([]string{"trash", "restore", "move", "addtags", "removetags"}, input.Action) {
		writeError(w, 400, "INVALID_ACTION", "지원하지 않는 동작입니다.")
		return
	}
	if len(input.IDs) == 0 {
		writeError(w, 400, "NO_DOCUMENTS", "문서를 선택해 주세요.")
		return
	}
	if len(input.IDs) > maxBulkDocuments {
		writeError(w, 400, "TOO_MANY_DOCUMENTS", "한 번에 200건까지 처리할 수 있습니다.")
		return
	}
	tags := normalizeTagNames(input.Tags)
	if strings.HasSuffix(input.Action, "tags") && len(tags) == 0 {
		writeError(w, 400, "NO_TAGS", "태그를 입력해 주세요.")
		return
	}

	results := make([]bulkResult, 0, len(input.IDs))
	changed := 0
	for _, id := range input.IDs {
		result := s.applyBulkAction(r, p, id, input.Action, input.FolderID, tags)
		if result.Status == "ok" {
			changed++
		}
		results = append(results, result)
	}

	s.audit(r, &p.User.ID, "BULK_"+strings.ToUpper(input.Action), "DOCUMENT", nil,
		map[string]any{"requested": len(input.IDs), "changed": changed})
	writeData(w, 200, map[string]any{"results": results, "changed": changed})
}

// applyBulkAction does one document, and says why when it does not.
func (s *Server) applyBulkAction(
	r *http.Request, p principal, id uuid.UUID, action string,
	folder **uuid.UUID, tags []string,
) bulkResult {
	skip := func(reason string) bulkResult {
		return bulkResult{ID: id, Status: "skipped", Reason: reason}
	}

	// Trash and restore both look at a document that may already be deleted,
	// so the lookup has to include those.
	includeDeleted := action == "restore"
	role, err := s.documentRole(r.Context(), p.User, id, includeDeleted)
	if err != nil {
		return skip("문서를 찾을 수 없습니다")
	}
	required := "EDITOR"
	if action == "trash" || action == "restore" {
		required = "OWNER"
	}
	if roleRank[role] < roleRank[required] {
		return skip("권한이 없습니다")
	}

	switch action {
	case "trash":
		tag, err := s.db.Exec(r.Context(),
			`UPDATE documents SET deleted_at=now(), updated_at=now()
			 WHERE id=$1 AND deleted_at IS NULL AND workflow_status <> 'PENDING'`, id)
		if err != nil {
			return skip("삭제하지 못했습니다")
		}
		if tag.RowsAffected() == 0 {
			return skip("승인 대기 중이거나 이미 휴지통에 있습니다")
		}
		s.hub.CloseDocument(id)

	case "restore":
		tag, err := s.db.Exec(r.Context(),
			`UPDATE documents SET deleted_at=NULL, updated_at=now() WHERE id=$1 AND deleted_at IS NOT NULL`, id)
		if err != nil || tag.RowsAffected() == 0 {
			return skip("휴지통에 있는 문서가 아닙니다")
		}

	case "move":
		if folder == nil {
			return skip("옮길 폴더가 지정되지 않았습니다")
		}
		var target any
		if *folder != nil {
			// The folder has to belong to the document's own workspace; a bulk
			// move does not carry documents across workspaces, where the
			// people who can see them would change.
			var valid bool
			_ = s.db.QueryRow(r.Context(),
				`SELECT EXISTS(SELECT 1 FROM folders f JOIN documents d ON d.workspace_id=f.workspace_id
				 WHERE f.id=$1 AND d.id=$2 AND f.deleted_at IS NULL)`, **folder, id).Scan(&valid)
			if !valid {
				return skip("이 문서의 워크스페이스에 없는 폴더입니다")
			}
			target = **folder
		}
		if _, err := s.db.Exec(r.Context(),
			`UPDATE documents SET folder_id=$2, updated_at=now() WHERE id=$1 AND deleted_at IS NULL`, id, target); err != nil {
			return skip("옮기지 못했습니다")
		}

	case "addtags", "removetags":
		var workspaceID uuid.UUID
		if err := s.db.QueryRow(r.Context(), `SELECT workspace_id FROM documents WHERE id=$1`, id).Scan(&workspaceID); err != nil {
			return skip("문서를 찾을 수 없습니다")
		}
		err := database.WithTx(r.Context(), s.db, func(tx pgx.Tx) error {
			for _, name := range tags {
				if action == "removetags" {
					if _, err := tx.Exec(r.Context(),
						`DELETE FROM document_tags dt USING tags t
						 WHERE dt.tag_id=t.id AND dt.document_id=$1 AND t.workspace_id=$2 AND t.name=$3`,
						id, workspaceID, name); err != nil {
						return err
					}
					continue
				}
				var tagID uuid.UUID
				if err := tx.QueryRow(r.Context(),
					`INSERT INTO tags(workspace_id,name) VALUES($1,$2)
					 ON CONFLICT (workspace_id,name) DO UPDATE SET name=excluded.name RETURNING id`,
					workspaceID, name).Scan(&tagID); err != nil {
					return err
				}
				if _, err := tx.Exec(r.Context(),
					`INSERT INTO document_tags(document_id,tag_id) VALUES($1,$2) ON CONFLICT DO NOTHING`,
					id, tagID); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return skip("태그를 바꾸지 못했습니다")
		}
	}

	return bulkResult{ID: id, Status: "ok"}
}
