package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/database"
	"github.com/jackc/pgx/v5"
)

func (s *Server) submitApproval(w http.ResponseWriter, r *http.Request) {
	documentID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	all, err := s.settings.GetAll(r.Context(), false)
	if err != nil || !all.Workflow.Enabled {
		writeError(w, 409, "WORKFLOW_DISABLED", "관리자가 검토·승인 프로세스를 활성화하지 않았습니다.")
		return
	}
	role, err := s.documentRole(r.Context(), p.User, documentID, false)
	if err != nil || !requireDocumentRole(w, role, "EDITOR") {
		return
	}
	var requestID uuid.UUID
	err = database.WithTx(r.Context(), s.db, func(tx pgx.Tx) error {
		var revision int
		var status string
		if err := tx.QueryRow(r.Context(), `SELECT revision_no,workflow_status FROM documents WHERE id=$1 FOR UPDATE`, documentID).Scan(&revision, &status); err != nil {
			return err
		}
		if status == "PENDING" {
			return workflowConflict("이미 승인 대기 중인 문서입니다.")
		}
		requestID = uuid.New()
		if _, err := tx.Exec(r.Context(), `INSERT INTO approval_requests(id,document_id,revision_no,requested_by,required_approvals) VALUES($1,$2,$3,$4,$5)`, requestID, documentID, revision, p.User.ID, all.Workflow.RequiredApprovals); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `UPDATE documents SET workflow_status='PENDING',status='REVIEW',updated_at=now() WHERE id=$1`, documentID); err != nil {
			return err
		}
		_, err := tx.Exec(r.Context(), `INSERT INTO notifications(user_id,type,title,body,resource_type,resource_id) SELECT DISTINCT wm.user_id,'APPROVAL_REQUEST','문서 검토 요청','검토 및 승인할 문서가 있습니다.','DOCUMENT',$1::uuid FROM documents d JOIN workspace_members wm ON wm.workspace_id=d.workspace_id WHERE d.id=$1::uuid AND wm.role IN ('OWNER','MANAGER') AND wm.user_id<>$2::uuid`, documentID, p.User.ID)
		return err
	})
	if err != nil {
		writeError(w, 409, "WORKFLOW_SUBMIT_FAILED", err.Error())
		return
	}
	s.hub.CloseDocument(documentID)
	s.audit(r, &p.User.ID, "SUBMIT_APPROVAL", "DOCUMENT", &documentID, map[string]any{"requestId": requestID})
	writeData(w, 201, map[string]any{"id": requestID, "status": "PENDING", "requiredApprovals": all.Workflow.RequiredApprovals})
}

type workflowConflict string

func (e workflowConflict) Error() string { return string(e) }

func (s *Server) decideApproval(w http.ResponseWriter, r *http.Request) {
	requestID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	all, err := s.settings.GetAll(r.Context(), false)
	if err != nil || !all.Workflow.Enabled {
		writeError(w, 409, "WORKFLOW_DISABLED", "검토·승인 프로세스가 비활성화되어 있습니다.")
		return
	}
	var input struct {
		Decision string `json:"decision"`
		Comment  string `json:"comment"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Decision = strings.ToUpper(input.Decision)
	if input.Decision != "APPROVED" && input.Decision != "REJECTED" {
		writeError(w, 400, "INVALID_DECISION", "APPROVED 또는 REJECTED가 필요합니다.")
		return
	}
	var documentID, requester uuid.UUID
	var required int
	if s.db.QueryRow(r.Context(), `SELECT document_id,requested_by,required_approvals FROM approval_requests WHERE id=$1 AND status='PENDING'`, requestID).Scan(&documentID, &requester, &required) != nil {
		writeError(w, 404, "APPROVAL_NOT_FOUND", "대기 중인 승인 요청을 찾을 수 없습니다.")
		return
	}
	if !s.canReview(r.Context(), p.User, documentID) {
		writeError(w, 403, "REVIEW_PERMISSION_DENIED", "이 문서를 검토할 권한이 없습니다.")
		return
	}
	if !all.Workflow.AllowSelfApproval && requester == p.User.ID {
		writeError(w, 403, "SELF_APPROVAL_DISABLED", "자신이 요청한 문서는 승인할 수 없습니다.")
		return
	}
	err = database.WithTx(r.Context(), s.db, func(tx pgx.Tx) error {
		var lockedStatus string
		if err := tx.QueryRow(r.Context(), `SELECT status FROM approval_requests WHERE id=$1 FOR UPDATE`, requestID).Scan(&lockedStatus); err != nil {
			return err
		}
		if lockedStatus != "PENDING" {
			return workflowConflict("승인 요청이 이미 처리되었습니다.")
		}
		if _, err := tx.Exec(r.Context(), `INSERT INTO approval_decisions(request_id,reviewer_id,decision,comment) VALUES($1,$2,$3,$4)`, requestID, p.User.ID, input.Decision, truncate(input.Comment, 2000)); err != nil {
			return err
		}
		finalStatus := "PENDING"
		if input.Decision == "REJECTED" {
			finalStatus = "REJECTED"
		} else {
			var count int
			if err := tx.QueryRow(r.Context(), `SELECT count(*) FROM approval_decisions WHERE request_id=$1 AND decision='APPROVED'`, requestID).Scan(&count); err != nil {
				return err
			}
			if count >= required {
				finalStatus = "APPROVED"
			}
		}
		if finalStatus != "PENDING" {
			if _, err := tx.Exec(r.Context(), `UPDATE approval_requests SET status=$2,decided_at=now() WHERE id=$1`, requestID, finalStatus); err != nil {
				return err
			}
			docStatus := "REJECTED"
			publication := "DRAFT"
			if finalStatus == "APPROVED" {
				docStatus = "APPROVED"
				publication = "PUBLISHED"
			}
			if _, err := tx.Exec(r.Context(), `UPDATE documents SET workflow_status=$2,status=$3,updated_at=now() WHERE id=$1`, documentID, docStatus, publication); err != nil {
				return err
			}
		}
		_, err := tx.Exec(r.Context(), `INSERT INTO notifications(user_id,type,title,body,resource_type,resource_id) VALUES($1,'APPROVAL_DECISION',$2,$3,'DOCUMENT',$4)`, requester, "문서 검토 결과", input.Decision+": "+truncate(input.Comment, 180), documentID)
		return err
	})
	if err != nil {
		writeError(w, 409, "APPROVAL_DECISION_FAILED", "이미 처리했거나 승인 상태가 변경되었습니다.")
		return
	}
	s.hub.CloseDocument(documentID)
	s.audit(r, &p.User.ID, "DECIDE_APPROVAL", "DOCUMENT", &documentID, map[string]any{"requestId": requestID, "decision": input.Decision})
	w.WriteHeader(204)
}

func (s *Server) listApprovals(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	status := strings.ToUpper(r.URL.Query().Get("status"))
	if status == "" {
		status = "PENDING"
	}
	rows, err := s.db.Query(r.Context(), `SELECT ar.id,ar.document_id,d.title,ar.revision_no,ar.requested_by,u.display_name,ar.status,ar.required_approvals,ar.created_at,ar.decided_at,(SELECT count(*) FROM approval_decisions ad WHERE ad.request_id=ar.id AND ad.decision='APPROVED') FROM approval_requests ar JOIN documents d ON d.id=ar.document_id JOIN users u ON u.id=ar.requested_by WHERE ar.status=$1 AND ($2='ADMIN' OR EXISTS(SELECT 1 FROM workspace_members wm WHERE wm.workspace_id=d.workspace_id AND wm.user_id=$3 AND wm.role IN ('OWNER','MANAGER'))) ORDER BY ar.created_at DESC LIMIT 100`, status, p.User.Role, p.User.ID)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "승인 요청을 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, docID, requester uuid.UUID
		var title, name, requestStatus string
		var revision, required, approved int
		var created time.Time
		var decided *time.Time
		if rows.Scan(&id, &docID, &title, &revision, &requester, &name, &requestStatus, &required, &created, &decided, &approved) == nil {
			items = append(items, map[string]any{"id": id, "documentId": docID, "documentTitle": title, "revision": revision, "requester": map[string]any{"id": requester, "displayName": name}, "status": requestStatus, "requiredApprovals": required, "approvedCount": approved, "createdAt": created, "decidedAt": decided})
		}
	}
	writeData(w, 200, items)
}
