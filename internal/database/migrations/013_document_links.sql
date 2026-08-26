-- Public share links.
--
-- The visibility value 'LINK' has been accepted since the first migration and
-- an administrator setting appeared to control it, but nothing granted access
-- on it and no route served a document by link. Setting it did nothing at all.
-- This is the mechanism that was missing.
--
-- A link is its own row rather than a document_permissions entry with
-- subject_type='PUBLIC_LINK': it needs a token, a use count, and an expiry of
-- its own, and a document may reasonably have more than one (one for the
-- client, one for the auditor, revoked separately).
CREATE TABLE IF NOT EXISTS document_links (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    -- The token is never stored. The prefix finds the row; the hash proves the
    -- rest of it, compared in constant time. Someone who reads this table
    -- cannot reconstruct a working link from it.
    token_prefix text NOT NULL UNIQUE,
    token_hash bytea NOT NULL,
    label text NOT NULL DEFAULT '',
    -- VIEWER only. Letting an anonymous holder of a URL edit or comment would
    -- need an identity to attribute the change to, and there is none.
    role text NOT NULL DEFAULT 'VIEWER' CHECK (role IN ('VIEWER')),
    -- Optional second factor for the link. Also hashed.
    password_hash text,
    expires_at timestamptz,
    -- A link that may be opened a fixed number of times, for the case where it
    -- is sent to one person and should not outlive that.
    max_views integer CHECK (max_views IS NULL OR max_views > 0),
    view_count bigint NOT NULL DEFAULT 0,
    last_viewed_at timestamptz,
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);

-- Every public request looks a link up by prefix, and the owner's screen lists
-- the links on one document.
CREATE INDEX IF NOT EXISTS document_links_document_idx
    ON document_links (document_id, created_at DESC);
