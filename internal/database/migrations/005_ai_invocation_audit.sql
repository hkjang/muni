-- Every AI call is recorded so an administrator can answer the questions an
-- operator actually gets asked: who used the model, what it cost, and what
-- failed. The columns the original table had were never written, so the record
-- could not answer any of them.
ALTER TABLE ai_actions ADD COLUMN IF NOT EXISTS duration_ms integer;
ALTER TABLE ai_actions ADD COLUMN IF NOT EXISTS error_code text;
ALTER TABLE ai_actions ADD COLUMN IF NOT EXISTS error_message text;
ALTER TABLE ai_actions ADD COLUMN IF NOT EXISTS prompt_chars bigint;
ALTER TABLE ai_actions ADD COLUMN IF NOT EXISTS completion_chars bigint;
ALTER TABLE ai_actions ADD COLUMN IF NOT EXISTS tool_calls integer NOT NULL DEFAULT 0;

-- The admin screen reads by time, and filters by person, status and model.
CREATE INDEX IF NOT EXISTS ai_actions_created_idx ON ai_actions (created_at DESC);
CREATE INDEX IF NOT EXISTS ai_actions_user_created_idx ON ai_actions (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS ai_actions_status_created_idx ON ai_actions (status, created_at DESC);
