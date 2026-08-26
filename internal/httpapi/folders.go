package httpapi

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/database"
	"github.com/jackc/pgx/v5"
)

// Folders could be created and listed and nothing else: no rename, no delete,
// no moving one inside another. A folder chosen by mistake stayed chosen.

// updateFolder renames a folder or moves it under another one.
func (s *Server) updateFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	workspaceID, ok := s.folderWorkspace(w, r, id, p)
	if !ok {
		return
	}
	var input struct {
		Name *string `json:"name"`
		// ParentID moves the folder. A null moves it to the top of the
		// workspace, which is why it is a pointer to a pointer: "not given"
		// and "given as null" are different instructions.
		ParentID **uuid.UUID `json:"parentId"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Name != nil {
		trimmed := strings.TrimSpace(*input.Name)
		if trimmed == "" || len([]rune(trimmed)) > 120 {
			writeError(w, 400, "INVALID_FOLDER", "폴더 이름을 확인해 주세요.")
			return
		}
		input.Name = &trimmed
	}

	if input.ParentID != nil && *input.ParentID != nil {
		parent := **input.ParentID
		if parent == id {
			writeError(w, 400, "FOLDER_CYCLE", "폴더를 자기 자신 안으로 옮길 수 없습니다.")
			return
		}
		var sameWorkspace bool
		_ = s.db.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM folders WHERE id=$1 AND workspace_id=$2 AND deleted_at IS NULL)`,
			parent, workspaceID).Scan(&sameWorkspace)
		if !sameWorkspace {
			writeError(w, 400, "INVALID_PARENT", "같은 워크스페이스의 폴더로만 옮길 수 있습니다.")
			return
		}
		// A folder moved inside its own descendant takes the whole branch out
		// of the tree, where nothing can reach it again.
		descendant, err := s.folderIsDescendant(r, parent, id)
		if err != nil {
			writeError(w, 500, "DATABASE_ERROR", "폴더 위치를 확인하지 못했습니다.")
			return
		}
		if descendant {
			writeError(w, 400, "FOLDER_CYCLE", "폴더를 그 하위 폴더 안으로 옮길 수 없습니다.")
			return
		}
	}

	var parentValue any
	if input.ParentID != nil {
		if *input.ParentID == nil {
			parentValue = nil
		} else {
			parentValue = **input.ParentID
		}
	}
	result, err := s.db.Exec(r.Context(), `
		UPDATE folders SET name = COALESCE($2, name),
			parent_id = CASE WHEN $4 THEN $3::uuid ELSE parent_id END,
			updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL`,
		id, input.Name, parentValue, input.ParentID != nil)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, 404, "FOLDER_NOT_FOUND", "폴더를 찾을 수 없습니다.")
		return
	}
	s.audit(r, &p.User.ID, "UPDATE_FOLDER", "FOLDER", &id, nil)
	writeData(w, 200, map[string]any{"id": id})
}

// deleteFolder removes a folder, leaving what was in it where a person can
// still find it.
//
// The documents are moved to the folder's parent rather than deleted: somebody
// tidying folders is not asking for their documents to go, and a document
// that vanished with a folder would have to be hunted for in the trash.
func (s *Server) deleteFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	if _, ok := s.folderWorkspace(w, r, id, p); !ok {
		return
	}

	var moved int64
	err := database.WithTx(r.Context(), s.db, func(tx pgx.Tx) error {
		var parent *uuid.UUID
		if err := tx.QueryRow(r.Context(), `SELECT parent_id FROM folders WHERE id=$1 AND deleted_at IS NULL`, id).Scan(&parent); err != nil {
			return err
		}
		tag, err := tx.Exec(r.Context(),
			`UPDATE documents SET folder_id=$2, updated_at=now() WHERE folder_id=$1`, id, parent)
		if err != nil {
			return err
		}
		moved = tag.RowsAffected()
		// Child folders move up too, for the same reason.
		if _, err := tx.Exec(r.Context(),
			`UPDATE folders SET parent_id=$2, updated_at=now() WHERE parent_id=$1 AND deleted_at IS NULL`, id, parent); err != nil {
			return err
		}
		_, err = tx.Exec(r.Context(), `UPDATE folders SET deleted_at=now() WHERE id=$1`, id)
		return err
	})
	if err != nil {
		writeError(w, 404, "FOLDER_NOT_FOUND", "폴더를 삭제하지 못했습니다.")
		return
	}
	s.audit(r, &p.User.ID, "DELETE_FOLDER", "FOLDER", &id, map[string]any{"documentsMoved": moved})
	writeData(w, 200, map[string]any{"id": id, "documentsMoved": moved})
}

// folderWorkspace finds the folder and checks the person may reorganise it.
func (s *Server) folderWorkspace(w http.ResponseWriter, r *http.Request, id uuid.UUID, p principal) (uuid.UUID, bool) {
	var workspaceID uuid.UUID
	if err := s.db.QueryRow(r.Context(),
		`SELECT workspace_id FROM folders WHERE id=$1 AND deleted_at IS NULL`, id).Scan(&workspaceID); err != nil {
		writeError(w, 404, "FOLDER_NOT_FOUND", "폴더를 찾을 수 없습니다.")
		return uuid.Nil, false
	}
	if p.User.Role != "ADMIN" {
		var role string
		if s.db.QueryRow(r.Context(),
			`SELECT role FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`,
			workspaceID, p.User.ID).Scan(&role) != nil || role == "VIEWER" {
			writeError(w, 403, "WORKSPACE_PERMISSION_DENIED", "폴더를 바꿀 권한이 없습니다.")
			return uuid.Nil, false
		}
	}
	return workspaceID, true
}

// folderIsDescendant reports whether candidate sits somewhere under ancestor.
func (s *Server) folderIsDescendant(r *http.Request, candidate, ancestor uuid.UUID) (bool, error) {
	var found bool
	// The recursive walk is bounded by the tree itself; a cycle cannot already
	// exist because this is the check that prevents one.
	err := s.db.QueryRow(r.Context(), `
		WITH RECURSIVE up AS (
			SELECT id, parent_id FROM folders WHERE id = $1
			UNION ALL
			SELECT f.id, f.parent_id FROM folders f JOIN up ON f.id = up.parent_id
		)
		SELECT EXISTS(SELECT 1 FROM up WHERE id = $2)`, candidate, ancestor).Scan(&found)
	return found, err
}

// moveDocument puts a document in another folder, or in another workspace.
//
// A folder could only be chosen when the document was created, so one put in
// the wrong place stayed there and folders could not be reorganised at all.
func (s *Server) moveDocument(w http.ResponseWriter, r *http.Request) {
	documentID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	role, err := s.documentRole(r.Context(), p.User, documentID, false)
	if !documentAllowed(w, role, err, "EDITOR") {
		return
	}
	var input struct {
		WorkspaceID *uuid.UUID  `json:"workspaceId"`
		FolderID    **uuid.UUID `json:"folderId"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}

	var currentWorkspace uuid.UUID
	if err := s.db.QueryRow(r.Context(), `SELECT workspace_id FROM documents WHERE id=$1`, documentID).Scan(&currentWorkspace); err != nil {
		writeError(w, 404, "DOCUMENT_NOT_FOUND", "문서를 찾을 수 없습니다.")
		return
	}
	target := currentWorkspace
	if input.WorkspaceID != nil && *input.WorkspaceID != currentWorkspace {
		target = *input.WorkspaceID
		// Moving a document into a workspace is a decision about that
		// workspace, so the mover has to be able to make it.
		var memberRole string
		if s.db.QueryRow(r.Context(),
			`SELECT role FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`,
			target, p.User.ID).Scan(&memberRole) != nil || memberRole == "VIEWER" {
			if p.User.Role != "ADMIN" {
				writeError(w, 403, "WORKSPACE_PERMISSION_DENIED", "옮길 워크스페이스에 문서를 만들 권한이 없습니다.")
				return
			}
		}
	}

	var folderValue any
	if input.FolderID != nil && *input.FolderID != nil {
		folder := **input.FolderID
		var valid bool
		_ = s.db.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM folders WHERE id=$1 AND workspace_id=$2 AND deleted_at IS NULL)`,
			folder, target).Scan(&valid)
		if !valid {
			writeError(w, 400, "INVALID_FOLDER", "대상 워크스페이스에 없는 폴더입니다.")
			return
		}
		folderValue = folder
	}
	// A document that changed workspace cannot stay in a folder of the old
	// one, so an unspecified folder means the top of the target workspace.
	clearFolder := input.FolderID != nil || target != currentWorkspace

	result, err := s.db.Exec(r.Context(), `
		UPDATE documents SET workspace_id=$2,
			folder_id = CASE WHEN $4 THEN $3::uuid ELSE folder_id END,
			updated_at=now()
		WHERE id=$1 AND deleted_at IS NULL`,
		documentID, target, folderValue, clearFolder)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, 409, "MOVE_FAILED", "문서를 옮기지 못했습니다.")
		return
	}
	if target != currentWorkspace {
		// The people who could open it may have changed with the workspace.
		s.hub.CloseDocument(documentID)
	}
	s.audit(r, &p.User.ID, "MOVE_DOCUMENT", "DOCUMENT", &documentID,
		map[string]any{"workspaceId": target, "folderChanged": clearFolder})
	s.getDocumentByID(w, r, documentID)
}
