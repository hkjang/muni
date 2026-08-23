package httpapi

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var mentionPattern = regexp.MustCompile(`@([a-zA-Z0-9_.-]{3,48})`)

func (s *Server) listComments(w http.ResponseWriter, r *http.Request) {
	documentID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	if _, err := s.documentRole(r.Context(), p.User, documentID, false); err != nil {
		writeError(w, 403, "DOCUMENT_PERMISSION_DENIED", "댓글을 볼 권한이 없습니다.")
		return
	}
	rows, err := s.db.Query(r.Context(), `SELECT c.id,c.parent_id,c.author_id,u.display_name,c.anchor,c.body,c.resolved_at,c.resolved_by,c.created_at,c.updated_at FROM comments c JOIN users u ON u.id=c.author_id WHERE c.document_id=$1 AND c.deleted_at IS NULL ORDER BY c.created_at`, documentID)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "댓글을 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, authorID uuid.UUID
		var parentID, resolvedBy *uuid.UUID
		var author, body string
		var anchor json.RawMessage
		var resolvedAt *time.Time
		var created, updated time.Time
		if rows.Scan(&id, &parentID, &authorID, &author, &anchor, &body, &resolvedAt, &resolvedBy, &created, &updated) == nil {
			items = append(items, map[string]any{"id": id, "parentId": parentID, "author": map[string]any{"id": authorID, "displayName": author}, "anchor": anchor, "body": body, "resolvedAt": resolvedAt, "resolvedBy": resolvedBy, "createdAt": created, "updatedAt": updated})
		}
	}
	writeData(w, 200, items)
}

func (s *Server) createComment(w http.ResponseWriter, r *http.Request) {
	documentID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	role, err := s.documentRole(r.Context(), p.User, documentID, false)
	if err != nil || !requireDocumentRole(w, role, "COMMENTER") {
		return
	}
	var input struct {
		ParentID *uuid.UUID      `json:"parentId"`
		Anchor   json.RawMessage `json:"anchor"`
		Body     string          `json:"body"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Body = strings.TrimSpace(input.Body)
	if input.Body == "" || len([]rune(input.Body)) > 5000 {
		writeError(w, 400, "INVALID_COMMENT", "댓글은 1~5,000자여야 합니다.")
		return
	}
	if len(input.Anchor) == 0 {
		input.Anchor = json.RawMessage("null")
	}
	id := uuid.New()
	result, err := s.db.Exec(r.Context(), `INSERT INTO comments(id,document_id,parent_id,author_id,anchor,body) SELECT $1,$2,$3,$4,$5,$6 WHERE $3::uuid IS NULL OR EXISTS(SELECT 1 FROM comments WHERE id=$3 AND document_id=$2 AND deleted_at IS NULL)`, id, documentID, input.ParentID, p.User.ID, input.Anchor, input.Body)
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, 400, "COMMENT_CREATE_FAILED", "댓글을 등록하지 못했습니다.")
		return
	}
	for _, match := range mentionPattern.FindAllStringSubmatch(input.Body, -1) {
		if len(match) < 2 {
			continue
		}
		_, _ = s.db.Exec(r.Context(), `INSERT INTO notifications(user_id,type,title,body,resource_type,resource_id) SELECT id,'MENTION','문서 댓글에서 회원님을 멘션했습니다.',$3,'DOCUMENT',$1 FROM users WHERE username=$2 AND id<>$4`, documentID, match[1], truncate(input.Body, 240), p.User.ID)
	}
	s.audit(r, &p.User.ID, "CREATE_COMMENT", "DOCUMENT", &documentID, map[string]any{"commentId": id})
	writeData(w, 201, map[string]any{"id": id})
}

func (s *Server) resolveComment(w http.ResponseWriter, r *http.Request) {
	s.setCommentResolved(w, r, true)
}
func (s *Server) reopenComment(w http.ResponseWriter, r *http.Request) {
	s.setCommentResolved(w, r, false)
}
func (s *Server) setCommentResolved(w http.ResponseWriter, r *http.Request, resolved bool) {
	commentID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	var documentID, authorID uuid.UUID
	if s.db.QueryRow(r.Context(), `SELECT document_id,author_id FROM comments WHERE id=$1 AND deleted_at IS NULL`, commentID).Scan(&documentID, &authorID) != nil {
		writeError(w, 404, "COMMENT_NOT_FOUND", "댓글을 찾을 수 없습니다.")
		return
	}
	role, err := s.documentRole(r.Context(), p.User, documentID, false)
	if err != nil || (authorID != p.User.ID && roleRank[role] < roleRank["EDITOR"]) {
		writeError(w, 403, "COMMENT_PERMISSION_DENIED", "댓글 상태를 바꿀 권한이 없습니다.")
		return
	}
	if resolved {
		_, err = s.db.Exec(r.Context(), `UPDATE comments SET resolved_at=now(),resolved_by=$2,updated_at=now() WHERE id=$1`, commentID, p.User.ID)
	} else {
		_, err = s.db.Exec(r.Context(), `UPDATE comments SET resolved_at=NULL,resolved_by=NULL,updated_at=now() WHERE id=$1`, commentID)
	}
	if err != nil {
		writeError(w, 500, "COMMENT_UPDATE_FAILED", "댓글 상태를 변경하지 못했습니다.")
		return
	}
	s.audit(r, &p.User.ID, map[bool]string{true: "RESOLVE_COMMENT", false: "REOPEN_COMMENT"}[resolved], "DOCUMENT", &documentID, map[string]any{"commentId": commentID})
	w.WriteHeader(204)
}

func (s *Server) listSuggestions(w http.ResponseWriter, r *http.Request) {
	documentID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	if _, err := s.documentRole(r.Context(), p.User, documentID, false); err != nil {
		writeError(w, 403, "DOCUMENT_PERMISSION_DENIED", "제안을 볼 권한이 없습니다.")
		return
	}
	rows, err := s.db.Query(r.Context(), `SELECT s.id,s.author_id,u.display_name,s.range_data,s.previous_value,s.new_value,s.status,s.decided_by,s.decided_at,s.created_at FROM suggestions s JOIN users u ON u.id=s.author_id WHERE s.document_id=$1 ORDER BY s.created_at DESC`, documentID)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "제안을 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, authorID uuid.UUID
		var author, status string
		var rangeData, previous, newValue json.RawMessage
		var decidedBy *uuid.UUID
		var decidedAt *time.Time
		var created time.Time
		if rows.Scan(&id, &authorID, &author, &rangeData, &previous, &newValue, &status, &decidedBy, &decidedAt, &created) == nil {
			items = append(items, map[string]any{"id": id, "author": map[string]any{"id": authorID, "displayName": author}, "range": rangeData, "previousValue": previous, "newValue": newValue, "status": status, "decidedBy": decidedBy, "decidedAt": decidedAt, "createdAt": created})
		}
	}
	writeData(w, 200, items)
}
func (s *Server) createSuggestion(w http.ResponseWriter, r *http.Request) {
	documentID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	role, err := s.documentRole(r.Context(), p.User, documentID, false)
	if err != nil || !requireDocumentRole(w, role, "COMMENTER") {
		return
	}
	var input struct {
		Range         json.RawMessage `json:"range"`
		PreviousValue json.RawMessage `json:"previousValue"`
		NewValue      json.RawMessage `json:"newValue"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if len(input.Range) == 0 || len(input.NewValue) == 0 {
		writeError(w, 400, "INVALID_SUGGESTION", "변경 범위와 제안 값이 필요합니다.")
		return
	}
	id := uuid.New()
	_, err = s.db.Exec(r.Context(), `INSERT INTO suggestions(id,document_id,author_id,range_data,previous_value,new_value) VALUES($1,$2,$3,$4,$5,$6)`, id, documentID, p.User.ID, input.Range, nullJSON(input.PreviousValue), input.NewValue)
	if err != nil {
		writeError(w, 400, "SUGGESTION_CREATE_FAILED", "제안을 등록하지 못했습니다.")
		return
	}
	s.audit(r, &p.User.ID, "CREATE_SUGGESTION", "DOCUMENT", &documentID, map[string]any{"suggestionId": id})
	writeData(w, 201, map[string]any{"id": id})
}
func (s *Server) decideSuggestion(w http.ResponseWriter, r *http.Request) {
	suggestionID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	var input struct {
		Decision string `json:"decision"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Decision = strings.ToUpper(input.Decision)
	if input.Decision != "ACCEPTED" && input.Decision != "REJECTED" {
		writeError(w, 400, "INVALID_DECISION", "ACCEPTED 또는 REJECTED가 필요합니다.")
		return
	}
	var documentID uuid.UUID
	if s.db.QueryRow(r.Context(), `SELECT document_id FROM suggestions WHERE id=$1 AND status='PENDING'`, suggestionID).Scan(&documentID) != nil {
		writeError(w, 404, "SUGGESTION_NOT_FOUND", "대기 중인 제안을 찾을 수 없습니다.")
		return
	}
	role, err := s.documentRole(r.Context(), p.User, documentID, false)
	if err != nil || !requireDocumentRole(w, role, "EDITOR") {
		return
	}
	_, err = s.db.Exec(r.Context(), `UPDATE suggestions SET status=$2,decided_by=$3,decided_at=now() WHERE id=$1 AND status='PENDING'`, suggestionID, input.Decision, p.User.ID)
	if err != nil {
		writeError(w, 500, "SUGGESTION_UPDATE_FAILED", "제안 상태를 변경하지 못했습니다.")
		return
	}
	s.audit(r, &p.User.ID, "DECIDE_SUGGESTION", "DOCUMENT", &documentID, map[string]any{"suggestionId": suggestionID, "decision": input.Decision})
	w.WriteHeader(204)
}
func nullJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return raw
}
