package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/integration/ptium"
	"github.com/hkjang/muni/internal/richdoc"
)

// planPresentationSync reports what a document change means for a deck without
// touching it. Rebuilding a whole deck discards everything a person did in the
// presentation editor, so the plan comes first and costs nothing.
func (s *Server) planPresentationSync(w http.ResponseWriter, r *http.Request) {
	link, config, ok := s.presentationForRequest(w, r, "VIEWER")
	if !ok {
		return
	}
	plan, _, err := s.buildSyncPlan(r.Context(), link, config)
	if err != nil {
		writeError(w, ptium.HTTPStatus(err), "PTIUM_REQUEST_FAILED", err.Error())
		return
	}
	writeData(w, 200, plan)
}

// applyPresentationSync redrafts only the slides whose source material moved.
//
// Ptium's revise call returns a proposal and saves nothing, so muni splices the
// new slide text into the deck source it already holds and writes the whole
// thing back. Slides nobody's section touched keep their exact text, which is
// what preserves work done in the presentation editor.
func (s *Server) applyPresentationSync(w http.ResponseWriter, r *http.Request) {
	link, config, ok := s.presentationForRequest(w, r, "EDITOR")
	if !ok {
		return
	}
	plan, deck, err := s.buildSyncPlan(r.Context(), link, config)
	if err != nil {
		writeError(w, ptium.HTTPStatus(err), "PTIUM_REQUEST_FAILED", err.Error())
		return
	}
	if !plan.Changed() {
		writeData(w, 200, map[string]any{"plan": plan, "revised": 0, "applied": false})
		return
	}

	client := ptium.NewClient(config)
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(config.TimeoutSeconds)*time.Second)
	defer cancel()

	// The version is read now so a concurrent edit in Ptium is rejected rather
	// than overwritten.
	presentation, err := client.Get(ctx, link.PresentationID)
	if err != nil {
		writeError(w, ptium.HTTPStatus(err), "PTIUM_REQUEST_FAILED", err.Error())
		return
	}

	revised := 0
	failures := make([]map[string]any, 0)
	for _, impact := range plan.Impacts {
		if impact.Action != ptium.SlideRevise || impact.Position == 0 {
			continue
		}
		source, err := client.ReviseSlide(ctx, link.PresentationID, impact.Position, impact.Instruction)
		if err != nil {
			// One slide that could not be redrafted should not cost the others.
			s.logger.Warn("slide revision failed", "presentation_id", link.PresentationID,
				"slide", impact.Position, "error", err.Error())
			failures = append(failures, map[string]any{"position": impact.Position, "error": err.Error()})
			continue
		}
		if deck.Replace(impact.Position, source) {
			revised++
		}
	}
	if revised == 0 {
		writeError(w, 502, "PTIUM_SYNC_FAILED", "슬라이드를 다시 작성하지 못했습니다.")
		return
	}
	if err := client.ApplySource(ctx, link.PresentationID, deck.Source(), presentation.Version, false); err != nil {
		writeError(w, ptium.HTTPStatus(err), "PTIUM_SYNC_FAILED", err.Error())
		return
	}

	p, _ := principalFrom(r.Context())
	if _, err := s.db.Exec(r.Context(),
		`UPDATE document_presentations SET document_revision=$2,updated_at=now(),last_synced_at=now() WHERE id=$1`,
		link.ID, plan.ToRevision); err != nil {
		s.logger.Warn("presentation link was not updated after sync", "id", link.ID, "error", err)
	}
	s.audit(r, &p.User.ID, "SYNC_PRESENTATION", "DOCUMENT", &link.DocumentID, map[string]any{
		"presentationId": link.PresentationID, "from": plan.FromRevision,
		"to": plan.ToRevision, "revised": revised,
	})
	writeData(w, 200, map[string]any{
		"plan": plan, "revised": revised, "applied": true, "failures": failures,
	})
}

// buildSyncPlan compares the revision the deck was built from with the current
// one and works out which slides that reaches.
func (s *Server) buildSyncPlan(ctx context.Context, link presentationLink, config ptium.Config) (ptium.SyncPlan, ptium.Deck, error) {
	var title string
	var currentRevision int
	var currentContent json.RawMessage
	if err := s.db.QueryRow(ctx, `SELECT title,revision_no,content_json FROM documents WHERE id=$1`, link.DocumentID).
		Scan(&title, &currentRevision, &currentContent); err != nil {
		return ptium.SyncPlan{}, ptium.Deck{}, err
	}
	previous, err := s.revisionContent(ctx, link.DocumentID, link.DocumentRevision)
	if err != nil {
		// Without the revision the deck was built from there is nothing to
		// compare against; asking for a full rebuild is the honest answer.
		return ptium.SyncPlan{}, ptium.Deck{}, err
	}

	beforeDocument, err := richdoc.Parse(previous)
	if err != nil {
		return ptium.SyncPlan{}, ptium.Deck{}, err
	}
	afterDocument, err := richdoc.Parse(currentContent)
	if err != nil {
		return ptium.SyncPlan{}, ptium.Deck{}, err
	}

	beforeBrief := ptium.BuildBrief(beforeDocument, ptium.BriefSource{
		Type: "muni", DocumentID: link.DocumentID.String(), Revision: link.DocumentRevision, Title: title,
	}, ptium.Options{})
	afterBrief := ptium.BuildBrief(afterDocument, ptium.BriefSource{
		Type: "muni", DocumentID: link.DocumentID.String(), Revision: currentRevision, Title: title,
	}, ptium.Options{})

	client := ptium.NewClient(config)
	sourceCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	source, err := client.Source(sourceCtx, link.PresentationID)
	if err != nil {
		return ptium.SyncPlan{}, ptium.Deck{}, err
	}
	deck := ptium.SplitSlides(source)
	plan := ptium.PlanSync(richdoc.Diff(beforeDocument, afterDocument), deck, beforeBrief, afterBrief)
	return plan, deck, nil
}

func (s *Server) revisionContent(ctx context.Context, documentID uuid.UUID, revision int) (json.RawMessage, error) {
	var content json.RawMessage
	err := s.db.QueryRow(ctx,
		`SELECT content_json FROM document_revisions WHERE document_id=$1 AND revision_no=$2`,
		documentID, revision).Scan(&content)
	return content, err
}
