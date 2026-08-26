package httpapi

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

// The line builder is the part that can be checked without a database: which
// lines are refused, and where 전결 lands.

func TestNoApproversMeansTheOlderBehaviour(t *testing.T) {
	server := &Server{}
	line, err := server.buildApprovalLine(t.Context(), uuid.New(), User{ID: uuid.New()}, nil, 0, false)
	if err != nil || line != nil {
		t.Fatalf("an empty line should mean no line at all: %v %v", line, err)
	}
}

func TestALineIsRefusedWhenItIsTooLong(t *testing.T) {
	server := &Server{}
	approvers := make([]uuid.UUID, maxApprovalSteps+1)
	for index := range approvers {
		approvers[index] = uuid.New()
	}
	_, err := server.buildApprovalLine(t.Context(), uuid.New(), User{ID: uuid.New()}, approvers, 0, false)
	if err == nil || !strings.Contains(err.Error(), "결재선") {
		t.Fatalf("an over-long line should be refused: %v", err)
	}
}

func TestFinalPositionMustBeInTheLine(t *testing.T) {
	server := &Server{}
	approvers := []uuid.UUID{uuid.New(), uuid.New()}
	if _, err := server.buildApprovalLine(t.Context(), uuid.New(), User{ID: uuid.New()}, approvers, 5, false); err == nil {
		t.Fatal("a 전결 position past the end of the line should be refused")
	}
	if _, err := server.buildApprovalLine(t.Context(), uuid.New(), User{ID: uuid.New()}, approvers, -1, false); err == nil {
		t.Fatal("a negative 전결 position should be refused")
	}
}

func TestBlankApproverIsRefused(t *testing.T) {
	server := &Server{}
	_, err := server.buildApprovalLine(t.Context(), uuid.New(), User{ID: uuid.New()},
		[]uuid.UUID{uuid.Nil}, 0, false)
	if err == nil {
		t.Fatal("an unset approver should be refused")
	}
}

func TestTheSamePersonCannotStandTwiceInALine(t *testing.T) {
	server := &Server{}
	same := uuid.New()
	_, err := server.buildApprovalLine(t.Context(), uuid.New(), User{ID: uuid.New()},
		[]uuid.UUID{same, same}, 0, false)
	if err == nil || !strings.Contains(err.Error(), "두 번") {
		t.Fatalf("a repeated approver should be refused: %v", err)
	}
}

func TestTheRequesterCannotApproveTheirOwnWorkWhenThePolicySaysSo(t *testing.T) {
	server := &Server{}
	requester := User{ID: uuid.New()}
	_, err := server.buildApprovalLine(t.Context(), uuid.New(), requester,
		[]uuid.UUID{requester.ID}, 0, false)
	if err == nil || !strings.Contains(err.Error(), "본인") {
		t.Fatalf("self-approval should be refused when the policy forbids it: %v", err)
	}
}
