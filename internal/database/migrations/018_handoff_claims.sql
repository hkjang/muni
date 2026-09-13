-- 다른 서비스로 문서를 넘길 때 발급하는 표(claim).
--
-- 표는 한 번만 쓰이고 5분 안에 만료된다. 본문은 표를 만든 순간의 문서를 그대로
-- 담아 두어, 받아 가는 쪽이 보는 것이 보낸 사람이 본 것과 같다. 받아 가는 것이
-- 곧 삭제라서 두 번째 요청은 아무것도 찾지 못한다. 표 자체는 저장하지 않고
-- 해시만 둔다 — 데이터베이스가 새어도 표는 새지 않는다.
CREATE TABLE IF NOT EXISTS handoff_claims (
    claim_hash bytea PRIMARY KEY,
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    format text NOT NULL,
    filename text NOT NULL,
    content_type text NOT NULL,
    body bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS handoff_claims_expires_at_idx ON handoff_claims(expires_at);
