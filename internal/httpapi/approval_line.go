package httpapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// approvalStep is one place in the line.
type approvalStep struct {
	approver uuid.UUID
	// final marks 전결: approving here settles the request and the steps after
	// it are skipped rather than left waiting for someone who will never be
	// asked.
	final bool
}

// maxApprovalSteps bounds a line. A document that needs more than this is not
// being approved, it is being circulated.
const maxApprovalSteps = 10

// buildApprovalLine checks an ordered list of approvers and turns it into
// steps.
//
// Everything that can be wrong with a line is wrong before the request exists:
// a person who cannot read the document, the same person twice, the requester
// approving their own work when the policy forbids it. Finding out at step
// three is finding out too late.
func (s *Server) buildApprovalLine(
	ctx context.Context,
	documentID uuid.UUID,
	requester User,
	approvers []uuid.UUID,
	finalAt int,
	allowSelfApproval bool,
) ([]approvalStep, error) {
	if len(approvers) == 0 {
		return nil, nil
	}
	if len(approvers) > maxApprovalSteps {
		return nil, fmt.Errorf("결재선은 %d단계까지입니다", maxApprovalSteps)
	}
	if finalAt < 0 || finalAt > len(approvers) {
		return nil, errors.New("전결 위치가 결재선 범위를 벗어납니다")
	}

	// Everything that can be judged from the list itself is judged first, so
	// "you listed the same person twice" does not cost a round trip per
	// approver to find out.
	seen := map[uuid.UUID]bool{}
	for _, approver := range approvers {
		if approver == uuid.Nil {
			return nil, errors.New("결재자를 선택해 주세요")
		}
		if seen[approver] {
			return nil, errors.New("같은 사람을 결재선에 두 번 넣을 수 없습니다")
		}
		seen[approver] = true
		if approver == requester.ID && !allowSelfApproval {
			return nil, errors.New("본인은 결재선에 넣을 수 없습니다")
		}
	}

	line := make([]approvalStep, 0, len(approvers))
	for index, approver := range approvers {
		var status string
		if err := s.db.QueryRow(ctx, `SELECT status FROM users WHERE id=$1`, approver).Scan(&status); err != nil {
			return nil, errors.New("결재자를 찾을 수 없습니다")
		}
		if status != "ACTIVE" {
			return nil, errors.New("정지된 계정은 결재선에 넣을 수 없습니다")
		}
		// An approver who cannot open the document cannot approve it, and
		// finding that out at their turn stops the whole line.
		user := User{ID: approver, Role: "USER"}
		role, err := s.documentRole(ctx, user, documentID, false)
		if err != nil || roleRank[role] < roleRank["VIEWER"] {
			return nil, errors.New("문서를 볼 수 없는 사람이 결재선에 있습니다")
		}

		// Without an explicit 전결 the last step is the one that settles it.
		final := index+1 == len(approvers)
		if finalAt > 0 {
			final = index+1 == finalAt
		}
		line = append(line, approvalStep{approver: approver, final: final})
	}

	// A line where nobody can finish it would wait forever.
	hasFinal := false
	for _, step := range line {
		if step.final {
			hasFinal = true
		}
	}
	if !hasFinal {
		line[len(line)-1].final = true
	}
	return line, nil
}
