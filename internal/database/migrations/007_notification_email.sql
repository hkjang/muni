-- Notifications have always been written; nothing ever left the building with
-- them. These two columns are the outbox: when a notification was emailed, and
-- how many times sending has been tried so that one bad address cannot be
-- retried forever.
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS emailed_at timestamptz;
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS email_attempts integer NOT NULL DEFAULT 0;

-- The outbox query asks for the few rows still waiting, so it should not read
-- the whole table to find them.
CREATE INDEX IF NOT EXISTS notifications_outbox_idx
    ON notifications (created_at)
    WHERE emailed_at IS NULL;
