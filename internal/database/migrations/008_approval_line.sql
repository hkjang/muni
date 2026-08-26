-- 결재선. A Korean report is approved in order — 팀장, then 부서장, then
-- 본부장 — and only the person whose turn it is can act. muni could only
-- express "any N of the managers", which is a different thing and not the one
-- an organisation with a 결재선 is asking for.
--
-- The existing mode is kept and stays the default, so an installation that was
-- happy with it is unaffected.
ALTER TABLE approval_requests
    ADD COLUMN IF NOT EXISTS mode text NOT NULL DEFAULT 'ANY';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'approval_requests_mode_check'
    ) THEN
        ALTER TABLE approval_requests
            ADD CONSTRAINT approval_requests_mode_check CHECK (mode IN ('ANY', 'SEQUENTIAL'));
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS approval_steps (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id uuid NOT NULL REFERENCES approval_requests(id) ON DELETE CASCADE,
    -- Where in the line this approver sits, counting from one.
    position integer NOT NULL,
    approver_id uuid NOT NULL REFERENCES users(id),
    status text NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED', 'SKIPPED')),
    -- 전결: approving here finishes the request, and the steps after it are
    -- marked skipped rather than left waiting for someone who will never be
    -- asked.
    is_final boolean NOT NULL DEFAULT false,
    comment text NOT NULL DEFAULT '',
    decided_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (request_id, position)
);

-- "Whose turn is it" is asked on every decision and on every list.
CREATE INDEX IF NOT EXISTS approval_steps_turn_idx
    ON approval_steps (request_id, position)
    WHERE status = 'PENDING';

-- "What is waiting for me" is the approvals screen.
CREATE INDEX IF NOT EXISTS approval_steps_approver_idx
    ON approval_steps (approver_id)
    WHERE status = 'PENDING';
