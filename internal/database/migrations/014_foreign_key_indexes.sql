-- Indexes for the foreign keys muni actually queries by.
--
-- Thirty-one foreign keys had no index behind them. Most of those do not
-- matter — nothing looks up an app_settings row by who last changed it. These
-- are the ones that showed up on a measured plan, on a workspace of 300
-- people, 60 workspaces, 20,000 documents and 6,000 attachments.
--
-- Two kinds of query were paying for it. The first is ordinary reads: the
-- workspace list runs on every page load and was scanning workspace_members to
-- find three rows out of fourteen hundred. The second is cascading deletes:
-- removing 100 documents took 37ms because every child table had to be scanned
-- in full to find the rows belonging to them, and the retention policy deletes
-- in bulk.
--
-- Built without CONCURRENTLY because the migration runner wraps each file in a
-- transaction. On a large existing database this holds a write lock for the
-- duration of the build — seconds, during a restart that is already downtime.

-- Every page load: "which workspaces am I in".
-- The primary key is (workspace_id, user_id), which cannot answer a query that
-- knows only the user.
CREATE INDEX IF NOT EXISTS workspace_members_user_idx
    ON workspace_members (user_id);

-- The attachments tab, and the cascade when a document is deleted.
CREATE INDEX IF NOT EXISTS attachments_document_idx
    ON attachments (document_id);

-- favorites is keyed (user_id, document_id), so deleting a document had to
-- scan the whole table to find who had starred it.
CREATE INDEX IF NOT EXISTS favorites_document_idx
    ON favorites (document_id);

-- Filtering search by tag, and the cascade on delete.
CREATE INDEX IF NOT EXISTS document_tags_tag_idx
    ON document_tags (tag_id);

-- "Is this document waiting for approval", shown on every document row, and
-- the cascade on delete.
CREATE INDEX IF NOT EXISTS approval_requests_document_idx
    ON approval_requests (document_id);

-- Offboarding counts what a departing person still has waiting.
CREATE INDEX IF NOT EXISTS approval_requests_requester_idx
    ON approval_requests (requested_by, status);

-- The AI audit filtered to one document.
CREATE INDEX IF NOT EXISTS ai_actions_document_idx
    ON ai_actions (document_id);

-- The audit log filtered to one person — the "what did they do" question.
CREATE INDEX IF NOT EXISTS activity_logs_actor_idx
    ON activity_logs (actor_id, id DESC);

-- The template list, read every time somebody starts a document.
CREATE INDEX IF NOT EXISTS templates_workspace_idx
    ON templates (workspace_id);

-- Comment threads: finding the replies to a comment.
CREATE INDEX IF NOT EXISTS comments_parent_idx
    ON comments (parent_id) WHERE parent_id IS NOT NULL;

-- Walking a folder's ancestors, which is what stops a folder being moved
-- inside itself, and listing a folder's children.
CREATE INDEX IF NOT EXISTS folders_parent_idx
    ON folders (parent_id) WHERE parent_id IS NOT NULL;

-- Listing the documents in a folder.
CREATE INDEX IF NOT EXISTS documents_folder_idx
    ON documents (folder_id) WHERE folder_id IS NOT NULL AND deleted_at IS NULL;

-- documents_owner_idx already exists but is partial on deleted_at IS NULL, so
-- counting what a departing person has *in the trash* could not use it and
-- scanned every document in the installation.
CREATE INDEX IF NOT EXISTS documents_owner_trashed_idx
    ON documents (owner_id) WHERE deleted_at IS NOT NULL;

-- Offboarding and the admin workspace list ask what somebody owns.
CREATE INDEX IF NOT EXISTS workspaces_owner_idx
    ON workspaces (owner_id) WHERE deleted_at IS NULL;
