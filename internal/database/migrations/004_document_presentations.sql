-- muni keeps only the link. The presentation itself, its slides and the built
-- file all live in Ptium; storing a copy here would mean two systems disagreeing
-- about the same deck the moment someone edits it in Ptium.
CREATE TABLE document_presentations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    document_revision integer NOT NULL,
    provider text NOT NULL DEFAULT 'ptium',
    presentation_id text NOT NULL,
    title text NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    slide_count integer NOT NULL DEFAULT 0,
    template_id text,
    options jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    last_synced_at timestamptz,
    UNIQUE (provider, presentation_id)
);
CREATE INDEX document_presentations_document_idx ON document_presentations(document_id, created_at DESC);
