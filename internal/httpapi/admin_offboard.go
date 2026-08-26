package httpapi

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/database"
	"github.com/jackc/pgx/v5"
)

// Somebody leaves. What they own does not.
//
// muni could transfer a document, and separately transfer a workspace, and
// separately end sessions — one at a time, from three different screens, with
// no list of what was left. So the honest answer to "did we finish offboarding
// 김부장?" was that nobody knew.
//
// The sharpest part is approvals. A 결재선 is ordered, and only the person
// whose turn it is may act, so a departed approver leaves every document
// waiting at their step stuck forever. Nothing surfaced that, and suspending
// the account made it worse rather than better.
//
// muni does not delete the account. Almost every table that mentions a user
// does so without ON DELETE, which is the schema being right: an approval that
// was decided, a document that was edited, an audit line — those record what a
// person did, and removing the person would either destroy that or leave it
// lying about who acted. Offboarding hands over what they hold and suspends
// them; the record of what they did stays.

type belongings struct {
	Documents        int `json:"documents"`
	TrashedDocuments int `json:"trashedDocuments"`
	SharedWorkspaces int `json:"sharedWorkspaces"`
	Memberships      int `json:"memberships"`
	ActiveAPIKeys    int `json:"activeApiKeys"`
	OpenSessions     int `json:"openSessions"`
	// BlockingApprovals is the count of approval steps waiting on this person.
	// Every one of them is a document nobody else can move forward.
	BlockingApprovals int `json:"blockingApprovals"`
	PendingRequests   int `json:"pendingRequests"`
}

func (s *Server) userBelongings(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var target struct {
		DisplayName string
		Email       string
		Status      string
	}
	if err := s.db.QueryRow(r.Context(),
		`SELECT display_name,email,status FROM users WHERE id=$1`, id).
		Scan(&target.DisplayName, &target.Email, &target.Status); err != nil {
		writeError(w, 404, "USER_NOT_FOUND", "사용자를 찾을 수 없습니다.")
		return
	}

	held, err := s.countBelongings(r, id)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "보유 항목을 세지 못했습니다.")
		return
	}

	// The documents themselves, so the administrator can see what they are
	// handing over rather than a number. Capped: the point is recognition, and
	// a thousand rows is not that.
	rows, err := s.db.Query(r.Context(), `
		SELECT d.id, d.title, w.name, d.deleted_at IS NOT NULL, d.updated_at
		FROM documents d JOIN workspaces w ON w.id = d.workspace_id
		WHERE d.owner_id = $1
		ORDER BY d.deleted_at IS NOT NULL, d.updated_at DESC
		LIMIT 100`, id)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "문서를 불러오지 못했습니다.")
		return
	}
	defer rows.Close()
	documents := make([]map[string]any, 0)
	for rows.Next() {
		var docID uuid.UUID
		var title, workspace string
		var trashed bool
		var updatedAt any
		if rows.Scan(&docID, &title, &workspace, &trashed, &updatedAt) == nil {
			documents = append(documents, map[string]any{
				"id": docID, "title": title, "workspace": workspace,
				"trashed": trashed, "updatedAt": updatedAt,
			})
		}
	}

	writeData(w, 200, map[string]any{
		"user": map[string]any{
			"id": id, "displayName": target.DisplayName,
			"email": target.Email, "status": target.Status,
		},
		"counts":    held,
		"documents": documents,
		"truncated": held.Documents+held.TrashedDocuments > len(documents),
	})
}

func (s *Server) countBelongings(r *http.Request, id uuid.UUID) (belongings, error) {
	var held belongings
	err := s.db.QueryRow(r.Context(), `
		SELECT
			(SELECT count(*) FROM documents WHERE owner_id=$1 AND deleted_at IS NULL),
			(SELECT count(*) FROM documents WHERE owner_id=$1 AND deleted_at IS NOT NULL),
			(SELECT count(*) FROM workspaces WHERE owner_id=$1 AND kind<>'PERSONAL' AND deleted_at IS NULL),
			(SELECT count(*) FROM workspace_members WHERE user_id=$1),
			(SELECT count(*) FROM api_keys WHERE user_id=$1 AND revoked_at IS NULL),
			(SELECT count(*) FROM sessions WHERE user_id=$1 AND expires_at>now()),
			(SELECT count(*) FROM approval_steps st JOIN approval_requests rq ON rq.id=st.request_id
				WHERE st.approver_id=$1 AND st.status='PENDING' AND rq.status='PENDING'),
			(SELECT count(*) FROM approval_requests WHERE requested_by=$1 AND status='PENDING')`, id).
		Scan(&held.Documents, &held.TrashedDocuments, &held.SharedWorkspaces, &held.Memberships,
			&held.ActiveAPIKeys, &held.OpenSessions, &held.BlockingApprovals, &held.PendingRequests)
	return held, err
}

var (
	errSameUser      = errors.New("cannot hand over to the same person")
	errRecipientGone = errors.New("recipient is not an active account")
)

// offboardUser hands everything over in one transaction and suspends the
// account. Half an offboarding is worse than none: the documents moved but the
// sessions did not, and nobody can tell which half happened.
func (s *Server) offboardUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	var input struct {
		TransferTo        uuid.UUID `json:"transferTo"`
		IncludeTrashed    bool      `json:"includeTrashed"`
		ReassignApprovals bool      `json:"reassignApprovals"`
		RevokeAPIKeys     bool      `json:"revokeApiKeys"`
		EndSessions       bool      `json:"endSessions"`
		Suspend           bool      `json:"suspend"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.TransferTo == uuid.Nil {
		writeError(w, 400, "RECIPIENT_REQUIRED", "인계받을 사람을 선택해 주세요.")
		return
	}
	if input.TransferTo == id {
		writeError(w, 400, "SAME_USER", "같은 사람에게 넘길 수는 없습니다.")
		return
	}
	if id == p.User.ID {
		// Handing over your own account and suspending it mid-request would
		// end the session doing the work.
		writeError(w, 409, "SELF_OFFBOARD", "자기 계정은 정리할 수 없습니다.")
		return
	}

	moved := map[string]int64{}
	err := database.WithTx(r.Context(), s.db, func(tx pgx.Tx) error {
		var status string
		if err := tx.QueryRow(r.Context(), `SELECT status FROM users WHERE id=$1`, input.TransferTo).Scan(&status); err != nil {
			return err
		}
		if status != "ACTIVE" {
			return errRecipientGone
		}

		// Documents. The recipient becomes a member of every workspace the
		// documents live in: owning a document you cannot find is not owning
		// it. Their own personal workspace is skipped — it is somebody's name.
		tag, err := tx.Exec(r.Context(), `
			INSERT INTO workspace_members(workspace_id,user_id,role)
			SELECT DISTINCT d.workspace_id, $2::uuid, 'MEMBER' FROM documents d
			WHERE d.owner_id=$1 AND ($3::boolean OR d.deleted_at IS NULL)
			ON CONFLICT (workspace_id,user_id) DO NOTHING`, id, input.TransferTo, input.IncludeTrashed)
		if err != nil {
			return err
		}
		moved["workspacesJoined"] = tag.RowsAffected()

		tag, err = tx.Exec(r.Context(),
			`UPDATE documents SET owner_id=$2, updated_at=now()
			 WHERE owner_id=$1 AND ($3 OR deleted_at IS NULL)`, id, input.TransferTo, input.IncludeTrashed)
		if err != nil {
			return err
		}
		moved["documents"] = tag.RowsAffected()

		// Shared workspaces they owned. The personal one keeps its owner: it
		// is named after them and giving it away reads as impersonation. The
		// membership is granted from the same statement, so it can only cover
		// the workspaces this transfer actually moved.
		tag, err = tx.Exec(r.Context(), `
			WITH handed_over AS (
				UPDATE workspaces SET owner_id=$2, updated_at=now()
				WHERE owner_id=$1 AND kind<>'PERSONAL' AND deleted_at IS NULL
				RETURNING id
			)
			INSERT INTO workspace_members(workspace_id,user_id,role)
			SELECT id, $2::uuid, 'OWNER' FROM handed_over
			ON CONFLICT (workspace_id,user_id) DO UPDATE SET role='OWNER'`, id, input.TransferTo)
		if err != nil {
			return err
		}
		moved["workspaces"] = tag.RowsAffected()

		if input.ReassignApprovals {
			// Where the recipient is already an approver on the same request,
			// moving the step would put one person in the line twice — they
			// would have to approve the same document at two positions. Those
			// steps are skipped instead, which is what 전결 already means here.
			tag, err = tx.Exec(r.Context(), `
				UPDATE approval_steps st SET status='SKIPPED', decided_at=now(),
					comment = CASE WHEN st.comment='' THEN '퇴사자 정리로 건너뜀' ELSE st.comment END
				FROM approval_requests rq
				WHERE rq.id=st.request_id AND st.approver_id=$1 AND st.status='PENDING' AND rq.status='PENDING'
					AND EXISTS (SELECT 1 FROM approval_steps other
						WHERE other.request_id=st.request_id AND other.approver_id=$2)`, id, input.TransferTo)
			if err != nil {
				return err
			}
			moved["approvalsSkipped"] = tag.RowsAffected()

			tag, err = tx.Exec(r.Context(), `
				UPDATE approval_steps st SET approver_id=$2
				FROM approval_requests rq
				WHERE rq.id=st.request_id AND st.approver_id=$1 AND st.status='PENDING' AND rq.status='PENDING'`, id, input.TransferTo)
			if err != nil {
				return err
			}
			moved["approvalsReassigned"] = tag.RowsAffected()

			// Skipping the last waiting step leaves a request that is pending
			// with nobody pending on it — the deadlock this whole endpoint
			// exists to clear, recreated by clearing it. Those are cancelled
			// and the document goes back to a draft its author can resubmit.
			//
			// Not approved. An approval line that lost its approvers has not
			// approved anything, and muni deciding one on its own is the one
			// outcome that must never come out of an administrative action.
			tag, err = tx.Exec(r.Context(), `
				WITH emptied AS (
					UPDATE approval_requests rq SET status='CANCELLED', decided_at=now()
					WHERE rq.status='PENDING' AND rq.mode='SEQUENTIAL'
						AND NOT EXISTS (SELECT 1 FROM approval_steps st
							WHERE st.request_id=rq.id AND st.status='PENDING')
					RETURNING rq.document_id
				)
				UPDATE documents SET workflow_status='DRAFT', status='DRAFT', updated_at=now()
				WHERE id IN (SELECT document_id FROM emptied)`)
			if err != nil {
				return err
			}
			moved["approvalsCancelled"] = tag.RowsAffected()
		}

		if input.RevokeAPIKeys {
			tag, err = tx.Exec(r.Context(),
				`UPDATE api_keys SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, id)
			if err != nil {
				return err
			}
			moved["apiKeysRevoked"] = tag.RowsAffected()
		}
		if input.EndSessions {
			tag, err = tx.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, id)
			if err != nil {
				return err
			}
			moved["sessionsEnded"] = tag.RowsAffected()
		}
		if input.Suspend {
			if _, err := tx.Exec(r.Context(),
				`UPDATE users SET status='SUSPENDED', updated_at=now() WHERE id=$1`, id); err != nil {
				return err
			}
			moved["suspended"] = 1
		}
		return nil
	})
	if errors.Is(err, errRecipientGone) {
		writeError(w, 409, "RECIPIENT_INACTIVE", "활성 상태인 사용자에게만 넘길 수 있습니다.")
		return
	}
	if err != nil {
		s.logger.Error("offboarding failed", "error", err, "user", id)
		writeError(w, 500, "DATABASE_ERROR", "정리하지 못했습니다. 아무것도 바뀌지 않았습니다.")
		return
	}

	s.audit(r, &p.User.ID, "OFFBOARD_USER", "USER", &id, map[string]any{
		"transferTo": input.TransferTo, "moved": moved,
	})
	remaining, _ := s.countBelongings(r, id)
	writeData(w, 200, map[string]any{"moved": moved, "remaining": remaining})
}
