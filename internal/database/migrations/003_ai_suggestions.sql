-- Suggestions were anchored to document positions, which move the moment
-- anyone edits above them, and every suggestion was attributed to a person.
--
-- block_id anchors a suggestion to the block it is about, so it still points at
-- the right text after the document changes around it. origin separates a
-- suggestion the AI proposed from one a colleague wrote, and note carries the
-- reason the AI gave.
ALTER TABLE suggestions
    ADD COLUMN block_id text,
    ADD COLUMN origin text NOT NULL DEFAULT 'USER' CHECK (origin IN ('USER','AI')),
    ADD COLUMN note text;

CREATE INDEX suggestions_document_status_idx ON suggestions(document_id, status);
