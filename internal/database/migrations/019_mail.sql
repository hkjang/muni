-- 메일 설정의 키를 사내 표준(MAIL-STANDARD)의 이름으로 옮긴다.
--
-- 앱마다 이름이 다르면 운영자가 스무 번 다르게 배우므로, 다른 사내 서비스와 같은
-- 이름을 쓴다. 값은 그대로이고 이름만 바뀐다. 이미 새 이름의 행이 있으면(같은
-- 마이그레이션을 두 번 돌리는 경우) 그것을 지키고 옛 행을 버린다.
--
-- 비밀번호(smtp.password)는 옮기지 않는다. 봉인할 때 키 이름을 함께 묶어 두어서
-- 이름을 바꾸면 열리지 않는다. 코드가 새 이름을 먼저 보고 없으면 옛 이름을 읽으며,
-- 관리자가 비밀번호를 새로 저장하면 그때 새 이름으로 적힌다.
UPDATE app_settings SET key = renamed.new_key, category = 'mail'
FROM (VALUES
    ('smtp.enabled', 'mail.enabled'),
    ('smtp.host', 'mail.smtp_host'),
    ('smtp.port', 'mail.smtp_port'),
    ('smtp.username', 'mail.username'),
    ('smtp.security', 'mail.security'),
    ('smtp.from', 'mail.from_address'),
    ('smtp.from_name', 'mail.from_name'),
    ('smtp.skip_verify', 'mail.skip_tls_verify'),
    ('smtp.base_url', 'mail.base_url')
) AS renamed(old_key, new_key)
WHERE app_settings.key = renamed.old_key
  AND NOT EXISTS (SELECT 1 FROM app_settings existing WHERE existing.key = renamed.new_key);
DELETE FROM app_settings WHERE key IN ('smtp.enabled', 'smtp.host', 'smtp.port', 'smtp.username',
    'smtp.security', 'smtp.from', 'smtp.from_name', 'smtp.skip_verify', 'smtp.base_url');

-- 건물 밖으로 나간 메일의 기록. 시도마다 한 행이라 "안 왔다"는 문의에 언제 누구에게
-- 무엇을 보내려 했고 되었는지 안 되었는지로 답할 수 있다. 본문은 담지 않는다 —
-- 제목과 받는 사람이면 충분하고, 본문까지 담으면 이 표가 그 자체로 유출 경로가 된다.
CREATE TABLE IF NOT EXISTS mail_deliveries (
    id uuid PRIMARY KEY,
    event text NOT NULL,
    recipient text NOT NULL,
    subject text NOT NULL,
    user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    notifications integer NOT NULL DEFAULT 1,
    status text NOT NULL CHECK (status IN ('sent', 'failed')),
    error_message text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS mail_deliveries_created_at_idx ON mail_deliveries (created_at DESC);

-- 만료가 다가온 API 키를 한 번만 알리기 위해 같은 키의 알림을 찾는다.
CREATE INDEX IF NOT EXISTS notifications_resource_idx ON notifications (resource_type, resource_id, type);
