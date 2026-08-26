-- Search reads with ILIKE '%…%' because Postgres' text search tokenises on
-- whitespace, and Korean does not put a space between 회의 and 회의록 — so
-- "회의" has to match inside "회의록" or the search finds nothing a reader
-- expects. That works today and reads the whole table to do it, which stops
-- being acceptable somewhere in the tens of thousands of documents.
--
-- Trigram indexes make exactly that pattern indexable.
--
-- The extension is contrib and ships with every official Postgres image, but
-- creating it needs a privilege a hardened installation may not grant the
-- application's role. A failure here must not stop the service from starting:
-- without the indexes search behaves exactly as it did before, just without
-- the speedup.
DO $$
BEGIN
    CREATE EXTENSION IF NOT EXISTS pg_trgm;
EXCEPTION
    WHEN insufficient_privilege OR undefined_file THEN
        RAISE NOTICE 'pg_trgm is unavailable; search stays unindexed';
END $$;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_trgm') THEN
        CREATE INDEX IF NOT EXISTS documents_title_trgm_idx
            ON documents USING gin (title gin_trgm_ops);
        CREATE INDEX IF NOT EXISTS documents_text_trgm_idx
            ON documents USING gin (content_text gin_trgm_ops);
        CREATE INDEX IF NOT EXISTS users_display_name_trgm_idx
            ON users USING gin (display_name gin_trgm_ops);
        CREATE INDEX IF NOT EXISTS tags_name_trgm_idx
            ON tags USING gin ((name::text) gin_trgm_ops);
    END IF;
END $$;
