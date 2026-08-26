-- An administrator can now create accounts, which means muni hands out
-- passwords it chose. A password somebody else picked has to be replaced on
-- first use, or the temporary one becomes the permanent one — and the
-- administrator who typed it keeps knowing it.
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_reset_required boolean NOT NULL DEFAULT false;

-- Who created the account, for the administrator asking why it exists. Kept
-- when the creator's own account is removed: the answer "someone who is gone"
-- is still worth more than no answer.
ALTER TABLE users ADD COLUMN IF NOT EXISTS created_by uuid REFERENCES users(id) ON DELETE SET NULL;

-- The user list sorts by this and filters on it ("nobody since March"), and
-- until now it read every row to do it.
CREATE INDEX IF NOT EXISTS idx_users_last_login ON users (last_login_at DESC NULLS LAST);
CREATE INDEX IF NOT EXISTS idx_users_created_at ON users (created_at DESC);
