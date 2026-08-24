-- Yjs updates were append-only, so a long-lived document accumulated every
-- keystroke batch forever and every client sent the whole document state back
-- on connect. Loading a document meant shipping the entire history.
--
-- A snapshot is a single merged Yjs state that stands in for every update up to
-- base_seq. Only the updates after it have to be stored and replayed.
CREATE TABLE collab_snapshots (
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    generation integer NOT NULL,
    base_seq bigint NOT NULL,
    state_data bytea NOT NULL CHECK (octet_length(state_data) <= 16777216),
    author_id uuid REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (document_id, generation)
);
