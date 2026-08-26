package settings

import "testing"

func TestRetentionKeepsAFloorOnVersions(t *testing.T) {
	// A policy that could leave a document with no history is not a retention
	// policy, it is data loss with a schedule.
	policy := Retention{RevisionKeep: 1}.Normalize()
	if policy.RevisionKeep != MinRevisionKeep {
		t.Fatalf("revisionKeep = %d, want at least %d", policy.RevisionKeep, MinRevisionKeep)
	}
	if zero := (Retention{}).Normalize(); zero.RevisionKeep != MinRevisionKeep {
		t.Fatalf("an unset policy should still keep versions: %d", zero.RevisionKeep)
	}
}

func TestRetentionTreatsNonsenseAsKeepForever(t *testing.T) {
	policy := Retention{TrashDays: -5, AuditDays: 99999, AIAuditDays: -1}.Normalize()
	if policy.TrashDays != 0 || policy.AuditDays != 0 || policy.AIAuditDays != 0 {
		t.Fatalf("out of range values should mean keep forever: %+v", policy)
	}
}

func TestRetentionLeavesAWorkablePolicyAlone(t *testing.T) {
	policy := Retention{TrashDays: 30, RevisionDays: 365, RevisionKeep: 20, AuditDays: 730}.Normalize()
	if policy.TrashDays != 30 || policy.RevisionDays != 365 || policy.RevisionKeep != 20 || policy.AuditDays != 730 {
		t.Fatalf("policy was altered: %+v", policy)
	}
}

func TestValidateRefusesAPolicyThatWouldEmptyHistory(t *testing.T) {
	all := workableSettings()
	all.Retention.RevisionKeep = 2
	if err := Validate(all); err == nil {
		t.Fatal("expected a policy keeping two versions to be refused")
	}
}

func TestValidateRefusesAnAbsurdPeriod(t *testing.T) {
	all := workableSettings()
	all.Retention.AuditDays = 100000
	if err := Validate(all); err == nil {
		t.Fatal("expected an out of range period to be refused")
	}
}

func TestValidateAcceptsNoPolicyAtAll(t *testing.T) {
	if err := Validate(workableSettings()); err != nil {
		t.Fatalf("an installation with no retention policy must stay valid: %v", err)
	}
}

// workableSettings is the smallest configuration Validate accepts, so each
// test can change one thing and see only that thing refused.
func workableSettings() All {
	return All{
		General:  General{ServiceName: "muni", PageSize: 30},
		OIDC:     OIDC{DefaultRole: "USER", Scopes: []string{"openid"}},
		AI:       AI{MaxTokens: 32768, TimeoutSeconds: 600},
		Workflow: Workflow{RequiredApprovals: 1},
		Security: Security{SessionHours: 12, APIKeyMaxDays: 365, MaxUploadMB: 50},
	}
}
