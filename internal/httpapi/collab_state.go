package httpapi

import (
	"context"

	"github.com/google/uuid"
)

// Compaction thresholds. Yjs updates are append-only, so without compaction a
// document's history — and the payload every client downloads on open — grows
// without bound. These are deliberately generous: compacting costs one full
// document state round trip, so it should be rare compared to editing.
const (
	compactUpdateCount = 400
	compactUpdateBytes = 4 << 20
	// maxSnapshotBytes matches the CHECK on collab_snapshots.
	maxSnapshotBytes = 16 << 20
	// maxUpdateBytes matches the CHECK on collab_updates.
	maxUpdateBytes = 1 << 20
)

// collabState is everything a joining client needs to rebuild the document:
// one merged snapshot plus the updates recorded after it.
type collabState struct {
	snapshot []byte
	baseSeq  int64
	updates  [][]byte
	maxSeq   int64
	bytes    int
}

// shouldCompact reports whether the tail after the snapshot has grown enough to
// be worth merging back into one.
func shouldCompact(updateCount, updateBytes int) bool {
	return updateCount >= compactUpdateCount || updateBytes >= compactUpdateBytes
}

func (s *Server) loadCollabState(ctx context.Context, documentID uuid.UUID, generation int) (collabState, error) {
	state := collabState{}
	// A missing snapshot is normal for a document that has never been
	// compacted, so the row is optional.
	var snapshot []byte
	var baseSeq int64
	if err := s.db.QueryRow(ctx,
		`SELECT state_data,base_seq FROM collab_snapshots WHERE document_id=$1 AND generation=$2`,
		documentID, generation).Scan(&snapshot, &baseSeq); err == nil {
		state.snapshot = snapshot
		state.baseSeq = baseSeq
		state.maxSeq = baseSeq
	}

	rows, err := s.db.Query(ctx,
		`SELECT seq,update_data FROM collab_updates WHERE document_id=$1 AND generation=$2 AND seq>$3 ORDER BY seq`,
		documentID, generation, state.baseSeq)
	if err != nil {
		return state, err
	}
	defer rows.Close()
	for rows.Next() {
		var seq int64
		var update []byte
		if rows.Scan(&seq, &update) != nil {
			continue
		}
		state.updates = append(state.updates, update)
		state.bytes += len(update)
		if seq > state.maxSeq {
			state.maxSeq = seq
		}
	}
	return state, rows.Err()
}

// storeCollabSnapshot replaces the tail up to baseSeq with a single merged
// state. The snapshot always covers at least those updates — the client built
// it after applying everything the server sent — so dropping them is safe even
// when a concurrent snapshot already moved the marker further along.
func (s *Server) storeCollabSnapshot(ctx context.Context, documentID uuid.UUID, generation int, baseSeq int64, author uuid.UUID, state []byte) error {
	if _, err := s.db.Exec(ctx,
		`INSERT INTO collab_snapshots(document_id,generation,base_seq,state_data,author_id,updated_at)
		 VALUES($1,$2,$3,$4,$5,now())
		 ON CONFLICT (document_id,generation) DO UPDATE
		   SET base_seq=EXCLUDED.base_seq, state_data=EXCLUDED.state_data,
		       author_id=EXCLUDED.author_id, updated_at=now()
		 WHERE collab_snapshots.base_seq < EXCLUDED.base_seq`,
		documentID, generation, baseSeq, state, author); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx,
		`DELETE FROM collab_updates WHERE document_id=$1 AND generation=$2 AND seq<=$3`,
		documentID, generation, baseSeq)
	return err
}
