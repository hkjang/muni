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
	// An approval line is an ordered list of people. Without one the request
	// keeps the older behaviour: any N of the workspace's managers.
	var input struct {
		Approvers []uuid.UUID `json:"approvers"`
		// FinalAt marks the position that may settle the request on its own
		// (전결); zero means only the last step can.
		FinalAt int `json:"finalAt"`
	}
	if r.ContentLength > 0 && !decodeJSON(w, r, &input) {
		return
	}
	line, lineErr := s.buildApprovalLine(r.Context(), documentID, p.User, input.Approvers, input.FinalAt, all.Workflow.AllowSelfApproval)
	if lineErr != nil {
		writeError(w, 400, "INVALID_APPROVAL_LINE", lineErr.Error())
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
		mode := "ANY"
		required := all.Workflow.RequiredApprovals
		if len(line) > 0 {
			mode = "SEQUENTIAL"
			// In a line every step has to say yes, so the count is the line.
			required = len(line)
		}
		if _, err := tx.Exec(r.Context(), `INSERT INTO approval_requests(id,document_id,revision_no,requested_by,required_approvals,mode) VALUES($1,$2,$3,$4,$5,$6)`, requestID, documentID, revision, p.User.ID, required, mode); err != nil {
			return err
		}
		for index, step := range line {
			if _, err := tx.Exec(r.Context(),
				`INSERT INTO approval_steps(request_id,position,approver_id,is_final) VALUES($1,$2,$3,$4)`,
				requestID, index+1, step.approver, step.final); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(r.Context(), `UPDATE documents SET workflow_status='PENDING',status='REVIEW',updated_at=now() WHERE id=$1`, documentID); err != nil {
			return err
		}
		if len(line) > 0 {
			// Only the first approver is asked. Telling everyone in the line at
			// once is how a queue turns into a scramble.
			_, err := tx.Exec(r.Context(),
				`INSERT INTO notifications(user_id,type,title,body,resource_type,resource_id)
				 VALUES($1,'APPROVAL_REQUEST','문서 결재 요청','결재할 차례입니다.','DOCUMENT',$2)`,
				line[0].approver, documentID)
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
	mode := "ANY"
	if len(line) > 0 {
		mode = "SEQUENTIAL"
	}
	writeData(w, 201, map[string]any{
		"id": requestID, "status": "PENDING", "mode": mode,
		"requiredApprovals": all.Workflow.RequiredApprovals, "steps": len(line),
	})
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
	var mode string
	if s.db.QueryRow(r.Context(), `SELECT document_id,requested_by,required_approvals,mode FROM approval_requests WHERE id=$1 AND status='PENDING'`, requestID).Scan(&documentID, &requester, &required, &mode) != nil {
		writeError(w, 404, "APPROVAL_NOT_FOUND", "대기 중인 승인 요청을 찾을 수 없습니다.")
		return
	}
	// In a line the right to decide comes from the line, not from a role: a
	// manager who is not this step's approver is not the one being asked.
	var currentStep uuid.UUID
	var stepIsFinal bool
	if mode == "SEQUENTIAL" {
		var approver uuid.UUID
		err := s.db.QueryRow(r.Context(), `
			SELECT id, approver_id, is_final FROM approval_steps
			WHERE request_id=$1 AND status='PENDING'
			ORDER BY position LIMIT 1`, requestID).Scan(&currentStep, &approver, &stepIsFinal)
		if err != nil {
			writeError(w, 409, "APPROVAL_LINE_EMPTY", "결재할 단계가 없습니다.")
			return
		}
		if approver != p.User.ID {
			writeError(w, 403, "NOT_YOUR_TURN", "지금은 결재할 차례가 아닙니다.")
			return
		}
	} else if !s.canReview(r.Context(), p.User, documentID) {
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
		var nextApprover *uuid.UUID

		if mode == "SEQUENTIAL" {
			if _, err := tx.Exec(r.Context(),
				`UPDATE approval_steps SET status=$2, comment=$3, decided_at=now() WHERE id=$1`,
				currentStep, input.Decision, truncate(input.Comment, 2000)); err != nil {
				return err
			}
			if input.Decision == "REJECTED" {
				finalStatus = "REJECTED"
				// A rejection ends the line; nobody after this is asked.
				if _, err := tx.Exec(r.Context(),
					`UPDATE approval_steps SET status='SKIPPED', decided_at=now()
					 WHERE request_id=$1 AND status='PENDING'`, requestID); err != nil {
					return err
				}
			} else if stepIsFinal {
				finalStatus = "APPROVED"
				if _, err := tx.Exec(r.Context(),
					`UPDATE approval_steps SET status='SKIPPED', decided_at=now()
					 WHERE request_id=$1 AND status='PENDING'`, requestID); err != nil {
					return err
				}
			} else {
				var next uuid.UUID
				err := tx.QueryRow(r.Context(),
					`SELECT approver_id FROM approval_steps WHERE request_id=$1 AND status='PENDING'
					 ORDER BY position LIMIT 1`, requestID).Scan(&next)
				if err == nil {
					nextApprover = &next
				} else {
					// No step left and none of them was final: settle it here
					// rather than leave the document waiting on nobody.
					finalStatus = "APPROVED"
				}
			}
		} else if input.Decision == "REJECTED" {
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
		if nextApprover != nil {
			// The turn has moved; the person it moved to is the one to tell.
			if _, err := tx.Exec(r.Context(),
				`INSERT INTO notifications(user_id,type,title,body,resource_type,resource_id)
				 VALUES($1,'APPROVAL_REQUEST','문서 결재 요청','결재할 차례입니다.','DOCUMENT',$2)`,
				*nextApprover, documentID); err != nil {
				return err
			}
		}
		if finalStatus == "PENDING" && nextApprover != nil {
			// The requester hears when it is settled, not at every step.
			return nil
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
	// Three kinds of person see a request: an administrator, a manager of the
	// workspace (which is who the older mode asks), and anyone standing in the
	// line — a line's approver is often not a manager of anything.
	rows, err := s.db.Query(r.Context(), `
		SELECT ar.id, ar.document_id, d.title, ar.revision_no, ar.requested_by, u.display_name,
			ar.status, ar.required_approvals, ar.mode, ar.created_at, ar.decided_at,
			(SELECT count(*) FROM approval_decisions ad WHERE ad.request_id=ar.id AND ad.decision='APPROVED'),
			(SELECT s2.approver_id FROM approval_steps s2 WHERE s2.request_id=ar.id AND s2.status='PENDING' ORDER BY s2.position LIMIT 1),
			(SELECT cu.display_name FROM approval_steps s3 JOIN users cu ON cu.id=s3.approver_id
			 WHERE s3.request_id=ar.id AND s3.status='PENDING' ORDER BY s3.position LIMIT 1),
			(SELECT count(*) FROM approval_steps s4 WHERE s4.request_id=ar.id),
			(SELECT count(*) FROM approval_steps s5 WHERE s5.request_id=ar.id AND s5.status='APPROVED')
		FROM approval_requests ar
		JOIN documents d ON d.id=ar.document_id
		JOIN users u ON u.id=ar.requested_by
		WHERE ar.status=$1
			AND ($2='ADMIN'
				OR EXISTS(SELECT 1 FROM workspace_members wm WHERE wm.workspace_id=d.workspace_id AND wm.user_id=$3 AND wm.role IN ('OWNER','MANAGER'))
				OR EXISTS(SELECT 1 FROM approval_steps st WHERE st.request_id=ar.id AND st.approver_id=$3)
				OR ar.requested_by=$3)
		ORDER BY ar.created_at DESC LIMIT 100`, status, p.User.Role, p.User.ID)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "승인 요청을 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	ids := make([]uuid.UUID, 0, 32)
	for rows.Next() {
		var id, docID, requester uuid.UUID
		var title, name, requestStatus, mode string
		var revision, required, approved int
		var created time.Time
		var decided *time.Time
		var currentID *uuid.UUID
		var currentName *string
		var steps, stepsApproved int
		if rows.Scan(&id, &docID, &title, &revision, &requester, &name, &requestStatus,
			&required, &mode, &created, &decided, &approved,
			&currentID, &currentName, &steps, &stepsApproved) == nil {
			item := map[string]any{
				"id": id, "documentId": docID, "documentTitle": title, "revision": revision,
				"requester": map[string]any{"id": requester, "displayName": name},
				"status":    requestStatus, "mode": mode,
				"requiredApprovals": required, "approvedCount": approved,
				"createdAt": created, "decidedAt": decided,
				"steps": []map[string]any{},
				// Whose turn it is, so the list can say so without the reader
				// opening each request to find out.
				"myTurn": currentID != nil && *currentID == p.User.ID,
			}
			if currentID != nil {
				item["currentApprover"] = map[string]any{"id": currentID, "displayName": currentName}
			}
			if mode == "SEQUENTIAL" {
				item["stepCount"] = steps
				item["stepsApproved"] = stepsApproved
			}
			items = append(items, item)
			ids = append(ids, id)
		}
	}
	s.attachApprovalSteps(r, items, ids)
	writeData(w, 200, items)
}

// attachApprovalSteps fills in each request's line in one query rather than
// one per request.
func (s *Server) attachApprovalSteps(r *http.Request, items []map[string]any, ids []uuid.UUID) {
	if len(ids) == 0 {
		return
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT st.request_id, st.position, st.approver_id, u.display_name, st.status,
			st.is_final, st.comment, st.decided_at
		FROM approval_steps st JOIN users u ON u.id = st.approver_id
		WHERE st.request_id = ANY($1) ORDER BY st.request_id, st.position`, ids)
	if err != nil {
		return
	}
	defer rows.Close()
	byRequest := map[uuid.UUID][]map[string]any{}
	for rows.Next() {
		var requestID, approver uuid.UUID
		var position int
		var name, stepStatus, comment string
		var isFinal bool
		var decided *time.Time
		if rows.Scan(&requestID, &position, &approver, &name, &stepStatus, &isFinal, &comment, &decided) == nil {
			byRequest[requestID] = append(byRequest[requestID], map[string]any{
				"position": position,
				"approver": map[string]any{"id": approver, "displayName": name},
				"status":   stepStatus, "isFinal": isFinal,
				"comment": comment, "decidedAt": decided,
			})
		}
	}
	for _, item := range items {
		if id, ok := item["id"].(uuid.UUID); ok {
			if steps, ok := byRequest[id]; ok {
				item["steps"] = steps
			}
		}
	}
}
