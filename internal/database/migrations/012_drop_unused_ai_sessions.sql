-- ai_sessions described AI conversations grouped into threads. muni has never
-- had those: the agent panel asks one question and gets one answer, with no
-- history on either side. Nothing has ever inserted a row here, so
-- ai_actions.session_id has been NULL on every AI call ever recorded.
--
-- A table nobody writes to costs nothing to run and quite a lot to read: the
-- next person to open the schema sees a foreign key and concludes that AI
-- usage is grouped by conversation, then writes a query that returns nothing.
--
-- The drop is conditional on the table being empty. It provably is — there is
-- no code path that could have filled it — but a migration that could destroy
-- rows should have to prove it is not doing so, and an installation that
-- somehow has data keeps it rather than losing it to an assumption.
DO $$
DECLARE
    row_count bigint;
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_tables WHERE tablename = 'ai_sessions') THEN
        RETURN;
    END IF;
    EXECUTE 'SELECT count(*) FROM ai_sessions' INTO row_count;
    IF row_count > 0 THEN
        RAISE NOTICE 'ai_sessions has % rows and was left in place', row_count;
        RETURN;
    END IF;
    ALTER TABLE ai_actions DROP COLUMN IF EXISTS session_id;
    DROP TABLE ai_sessions;
END $$;
