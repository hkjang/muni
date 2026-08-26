-- A Korean report is expected to number its sections. Doing it by hand means
-- renumbering everything below whenever a section is inserted, so the scheme
-- is recorded per document and the numbers are worked out from the headings
-- every time the document is drawn or exported.
ALTER TABLE documents ADD COLUMN IF NOT EXISTS heading_numbering text NOT NULL DEFAULT 'none';
