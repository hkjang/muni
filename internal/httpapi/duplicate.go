package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/database"
	"github.com/jackc/pgx/v5"
)

// duplicateDocument makes a copy of a document to work from.
//
// Starting the next month's report from last month's is the common case, and
// the alternatives were saving a template — which is a decision about the
// whole workspace for something one person needs once — or selecting the whole
// document and pasting it into a new one, which loses the tags and the folder.
func (s *Server) duplicateDocument(w http.ResponseWriter, r *http.Request) {
	sourceID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	// Copying is a read of the original and a write of something new, so being
	// able to read it is enough — the copy belongs to whoever made it.
	role, err := s.documentRole(r.Context(), p.User, sourceID, false)
	if err != nil || roleRank[role] < roleRank["VIEWER"] {
		writeError(w, 403, "DOCUMENT_PERMISSION_DENIED", "문서를 읽을 권한이 없습니다.")
		return
	}

	var input struct {
		Title string `json:"title"`
	}
	if r.ContentLength > 0 && !decodeJSON(w, r, &input) {
		return
	}

	var workspaceID uuid.UUID
	var folderID *uuid.UUID
	var title, visibility string
	var content json.RawMessage
	if err := s.db.QueryRow(r.Context(),
		`SELECT workspace_id, folder_id, title, visibility, content_json FROM documents WHERE id=$1`,
		sourceID).Scan(&workspaceID, &folderID, &title, &visibility, &content); err != nil {
		writeError(w, 404, "DOCUMENT_NOT_FOUND", "문서를 찾을 수 없습니다.")
		return
	}

	// Making a copy is making a document, so the same rule applies.
	var memberRole string
	if s.db.QueryRow(r.Context(),
		`SELECT role FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`,
		workspaceID, p.User.ID).Scan(&memberRole) != nil || memberRole == "VIEWER" {
		if p.User.Role != "ADMIN" {
			writeError(w, 403, "WORKSPACE_PERMISSION_DENIED", "이 워크스페이스에 문서를 만들 권한이 없습니다.")
			return
		}
	}

	newTitle := strings.TrimSpace(input.Title)
	if newTitle == "" {
		newTitle = copyTitle(title)
	}
	if len([]rune(newTitle)) > 240 {
		writeError(w, 400, "INVALID_TITLE", "제목은 240자 이하여야 합니다.")
		return
	}

	id := uuid.New()
	text := extractDocumentText(content)
	err = database.WithTx(r.Context(), s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(),
			`INSERT INTO documents(id,workspace_id,folder_id,owner_id,title,visibility,content_json,content_text,revision_no,workflow_status)
			 VALUES($1,$2,$3,$4,$5,$6,$7,$8,1,'NONE')`,
			id, workspaceID, folderID, p.User.ID, newTitle, visibility, content, text); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(),
			`INSERT INTO document_revisions(document_id,revision_no,content_json,content_text,author_id,reason)
			 VALUES($1,1,$2,$3,$4,'duplicate')`, id, content, text, p.User.ID); err != nil {
			return err
		}
		// The tags come with it: a copy of the 대외비 report is still 대외비,
		// and re-tagging by hand is how that gets forgotten.
		_, err := tx.Exec(r.Context(),
			`INSERT INTO document_tags(document_id,tag_id) SELECT $1, tag_id FROM document_tags WHERE document_id=$2`,
			id, sourceID)
		return err
	})
	if err != nil {
		writeError(w, 500, "DUPLICATE_FAILED", "문서를 복제하지 못했습니다.")
		return
	}

	// Comments, suggestions, approvals and the version history stay with the
	// original. They are a record of what happened to that document, and a
	// copy has had nothing happen to it yet.
	s.audit(r, &p.User.ID, "DUPLICATE_DOCUMENT", "DOCUMENT", &id, map[string]any{"from": sourceID})
	s.getDocumentByID(w, r, id)
}

// copyTitle names the copy.
//
// Repeated copying should not build "사본의 사본의 사본": a title that already
// ends in the marker gets a number instead.
func copyTitle(title string) string {
	const marker = " (사본)"
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return "제목 없는 문서" + marker
	}
	if !strings.HasSuffix(trimmed, marker) && !strings.Contains(trimmed, marker+" ") {
		return trimmed + marker
	}
	base := trimmed
	if index := strings.LastIndex(trimmed, marker); index >= 0 {
		base = trimmed[:index+len(marker)]
	}
	return base + " 2"
}
