package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/hkjang/muni/internal/settings"
)

// RetentionResult is what one pass removed.
type RetentionResult struct {
	Documents int64 `json:"documents"`
	Revisions int64 `json:"revisions"`
	Audit     int64 `json:"audit"`
	AIAudit   int64 `json:"aiAudit"`
	Sessions  int64 `json:"sessions"`
}

// Empty reports whether the pass had nothing to do, so a quiet system does not
// write a log line every day saying so.
func (r RetentionResult) Empty() bool {
	return r.Documents == 0 && r.Revisions == 0 && r.Audit == 0 &&
		r.AIAudit == 0 && r.Sessions == 0
}

// retentionInterval is how often the policy is applied. Daily is often enough
// for a rule measured in days, and rare enough that the deletes never land in
// the middle of a working morning more than once.
const retentionInterval = 24 * time.Hour

// StartRetention runs the cleanup in the background for the life of the
// process.
//
// Nothing was ever removed before this: trashed documents, every version of
// every document and both audit logs grew without bound. On an installation
// that has been running for a year that is a disk problem, and it leaves an
// operator unable to answer how long anything is kept — which is a question
// they are asked.
func (s *Server) StartRetention(ctx context.Context) {
	go func() {
		// A first pass shortly after start, so a policy set yesterday takes
		// effect without waiting a whole day, but late enough that it is not
		// competing with everything else a boot does.
		timer := time.NewTimer(5 * time.Minute)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			if result, err := s.applyRetention(ctx); err != nil {
				s.logger.Warn("retention pass failed", "error", err)
			} else if !result.Empty() {
				s.logger.Info("retention pass removed rows",
					"documents", result.Documents, "revisions", result.Revisions,
					"audit", result.Audit, "aiAudit", result.AIAudit, "sessions", result.Sessions)
			}
			timer.Reset(retentionInterval)
		}
	}()
}

// applyRetention removes what the policy says is past its time.
//
// Each rule is independent and a failure in one does not stop the others: a
// cleanup that gives up halfway leaves the operator with a policy that half
// works and no sign of which half.
func (s *Server) applyRetention(ctx context.Context) (RetentionResult, error) {
	var result RetentionResult
	all, err := s.settings.GetAll(ctx, false)
	if err != nil {
		return result, err
	}
	policy := all.Retention.Normalize()

	// Expired sessions are dead weight whatever the policy says: they can no
	// longer authenticate anyone.
	if tag, err := s.db.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`); err == nil {
		result.Sessions = tag.RowsAffected()
	}

	if policy.TrashDays > 0 {
		// The cascade takes the versions, comments, suggestions, attachments
		// and collaboration state with the document.
		if tag, err := s.db.Exec(ctx,
			`DELETE FROM documents WHERE deleted_at IS NOT NULL
			 AND deleted_at < now() - make_interval(days => $1)`, policy.TrashDays); err == nil {
			result.Documents = tag.RowsAffected()
		} else {
			s.logger.Warn("trash retention failed", "error", err)
		}
	}

	if policy.RevisionDays > 0 {
		if tag, err := s.db.Exec(ctx, revisionCleanup, policy.RevisionDays, policy.RevisionKeep); err == nil {
			result.Revisions = tag.RowsAffected()
		} else {
			s.logger.Warn("revision retention failed", "error", err)
		}
	}

	if policy.AuditDays > 0 {
		if tag, err := s.db.Exec(ctx,
			`DELETE FROM activity_logs WHERE created_at < now() - make_interval(days => $1)`,
			policy.AuditDays); err == nil {
			result.Audit = tag.RowsAffected()
		} else {
			s.logger.Warn("audit retention failed", "error", err)
		}
	}

	if policy.AIAuditDays > 0 {
		if tag, err := s.db.Exec(ctx,
			`DELETE FROM ai_actions WHERE created_at < now() - make_interval(days => $1)`,
			policy.AIAuditDays); err == nil {
			result.AIAudit = tag.RowsAffected()
		} else {
			s.logger.Warn("ai audit retention failed", "error", err)
		}
	}

	return result, nil
}

// revisionCleanup drops old versions while keeping the ones that matter.
//
// Three things always survive, however old: the newest $2 versions of each
// document, any version an author named, and the version the document is
// currently at. A history that can be emptied by the clock is not a history.
const revisionCleanup = `
WITH ranked AS (
	SELECT r.id, r.name, r.created_at, r.document_id, r.revision_no,
		row_number() OVER (PARTITION BY r.document_id ORDER BY r.revision_no DESC) AS recency,
		d.revision_no AS current_revision
	FROM document_revisions r JOIN documents d ON d.id = r.document_id
)
DELETE FROM document_revisions
WHERE id IN (
	SELECT id FROM ranked
	WHERE created_at < now() - make_interval(days => $1)
		AND recency > $2
		AND coalesce(btrim(name), '') = ''
		AND revision_no <> current_revision
)`

// retentionPreview reports what the current policy would remove, without
// removing it. An administrator setting a policy for the first time should be
// able to see the size of what they are about to delete.
func (s *Server) retentionPreview(ctx context.Context, policy settings.Retention) RetentionResult {
	policy = policy.Normalize()
	var result RetentionResult
	if policy.TrashDays > 0 {
		_ = s.db.QueryRow(ctx,
			`SELECT count(*) FROM documents WHERE deleted_at IS NOT NULL
			 AND deleted_at < now() - make_interval(days => $1)`, policy.TrashDays).Scan(&result.Documents)
	}
	if policy.RevisionDays > 0 {
		_ = s.db.QueryRow(ctx, revisionPreview, policy.RevisionDays, policy.RevisionKeep).Scan(&result.Revisions)
	}
	if policy.AuditDays > 0 {
		_ = s.db.QueryRow(ctx,
			`SELECT count(*) FROM activity_logs WHERE created_at < now() - make_interval(days => $1)`,
			policy.AuditDays).Scan(&result.Audit)
	}
	if policy.AIAuditDays > 0 {
		_ = s.db.QueryRow(ctx,
			`SELECT count(*) FROM ai_actions WHERE created_at < now() - make_interval(days => $1)`,
			policy.AIAuditDays).Scan(&result.AIAudit)
	}
	_ = s.db.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE expires_at < now()`).Scan(&result.Sessions)
	return result
}

// revisionPreview counts what revisionCleanup would take, using the same rule.
const revisionPreview = `
WITH ranked AS (
	SELECT r.id, r.name, r.created_at, r.revision_no,
		row_number() OVER (PARTITION BY r.document_id ORDER BY r.revision_no DESC) AS recency,
		d.revision_no AS current_revision
	FROM document_revisions r JOIN documents d ON d.id = r.document_id
)
SELECT count(*) FROM ranked
WHERE created_at < now() - make_interval(days => $1)
	AND recency > $2
	AND coalesce(btrim(name), '') = ''
	AND revision_no <> current_revision`

// previewRetention answers the admin screen: how much the policy as it stands
// would remove the next time it runs.
func (s *Server) previewRetention(w http.ResponseWriter, r *http.Request) {
	all, err := s.settings.GetAll(r.Context(), false)
	if err != nil {
		writeError(w, 500, "DATABASE_ERROR", "보존 정책을 읽지 못했습니다.")
		return
	}
	writeData(w, 200, map[string]any{
		"policy":  all.Retention.Normalize(),
		"pending": s.retentionPreview(r.Context(), all.Retention),
	})
}

// runRetention applies the policy now.
//
// The pass is daily, and an administrator who has just set a policy should not
// have to wait a day to see it take effect — or to find out that it removes
// more than they meant.
func (s *Server) runRetention(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	result, err := s.applyRetention(r.Context())
	if err != nil {
		writeError(w, 500, "RETENTION_FAILED", "보존 정책을 적용하지 못했습니다: "+err.Error())
		return
	}
	s.audit(r, &p.User.ID, "RUN_RETENTION", "SETTINGS", nil, map[string]any{
		"documents": result.Documents, "revisions": result.Revisions,
		"audit": result.Audit, "aiAudit": result.AIAudit, "sessions": result.Sessions,
	})
	writeData(w, 200, result)
}
