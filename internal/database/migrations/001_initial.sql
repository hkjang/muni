CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    username citext NOT NULL UNIQUE,
    email citext NOT NULL UNIQUE,
    display_name text NOT NULL,
    password_hash text,
    role text NOT NULL DEFAULT 'USER' CHECK (role IN ('ADMIN','USER')),
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','SUSPENDED')),
    oidc_subject text UNIQUE,
    avatar_url text,
    locale text NOT NULL DEFAULT 'ko-KR',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    last_login_at timestamptz
);

CREATE TABLE sessions (
    token_hash bytea PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    ip inet,
    user_agent text
);
CREATE INDEX sessions_user_idx ON sessions(user_id);
CREATE INDEX sessions_expiry_idx ON sessions(expires_at);

CREATE TABLE oidc_states (
    state_hash bytea PRIMARY KEY,
    verifier text NOT NULL,
    nonce text NOT NULL,
    return_to text NOT NULL DEFAULT '/',
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE app_settings (
    key text PRIMARY KEY,
    category text NOT NULL,
    value jsonb,
    encrypted_value bytea,
    is_secret boolean NOT NULL DEFAULT false,
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((is_secret AND encrypted_value IS NOT NULL) OR (NOT is_secret AND value IS NOT NULL))
);

INSERT INTO app_settings(key,category,value) VALUES
('general.service_name','general','"muni"'),
('general.allow_local_login','general','true'),
('general.default_locale','general','"ko-KR"'),
('general.page_size','general','30'),
('oidc.enabled','oidc','false'),
('oidc.issuer_url','oidc','""'),
('oidc.client_id','oidc','""'),
('oidc.redirect_url','oidc','""'),
('oidc.scopes','oidc','["openid","profile","email"]'),
('oidc.auto_provision','oidc','true'),
('oidc.default_role','oidc','"USER"'),
('ai.enabled','ai','false'),
('ai.base_url','ai','""'),
('ai.model','ai','""'),
('ai.max_tokens','ai','32768'),
('ai.timeout_seconds','ai','600'),
('ai.system_prompt','ai','"당신은 muni 문서 도우미입니다. 사용자가 접근할 수 있는 문서 정보만 사용하세요."'),
('workflow.enabled','workflow','false'),
('workflow.required_approvals','workflow','1'),
('workflow.allow_self_approval','workflow','false'),
('security.session_hours','security','12'),
('security.api_key_max_days','security','365'),
('security.allow_public_links','security','false'),
('security.max_upload_mb','security','50'),
('security.audit_reads','security','true'),
('export.enable_pdf','export','true'),
('export.enable_docx','export','true')
ON CONFLICT DO NOTHING;

CREATE TABLE workspaces (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    slug citext NOT NULL UNIQUE,
    description text NOT NULL DEFAULT '',
    kind text NOT NULL DEFAULT 'TEAM' CHECK (kind IN ('PERSONAL','TEAM','DEPARTMENT','ORGANIZATION')),
    owner_id uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE TABLE workspace_members (
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('OWNER','MANAGER','MEMBER','VIEWER')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(workspace_id,user_id)
);

CREATE TABLE folders (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    parent_id uuid REFERENCES folders(id) ON DELETE CASCADE,
    name text NOT NULL,
    owner_id uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE INDEX folders_workspace_idx ON folders(workspace_id,parent_id) WHERE deleted_at IS NULL;

CREATE TABLE documents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    folder_id uuid REFERENCES folders(id) ON DELETE SET NULL,
    owner_id uuid NOT NULL REFERENCES users(id),
    title text NOT NULL DEFAULT '제목 없는 문서',
    status text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','REVIEW','PUBLISHED','ARCHIVED')),
    visibility text NOT NULL DEFAULT 'RESTRICTED' CHECK (visibility IN ('RESTRICTED','WORKSPACE','ORGANIZATION','LINK')),
    workflow_status text NOT NULL DEFAULT 'NONE' CHECK (workflow_status IN ('NONE','DRAFT','PENDING','APPROVED','REJECTED')),
    content_json jsonb NOT NULL DEFAULT '{"type":"doc","content":[{"type":"paragraph"}]}'::jsonb,
    content_text text NOT NULL DEFAULT '',
    revision_no integer NOT NULL DEFAULT 0,
    crdt_generation integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE INDEX documents_workspace_idx ON documents(workspace_id,updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX documents_owner_idx ON documents(owner_id,updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX documents_search_idx ON documents USING gin(to_tsvector('simple', coalesce(title,'') || ' ' || coalesce(content_text,'')));

CREATE TABLE document_revisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    revision_no integer NOT NULL,
    content_json jsonb NOT NULL,
    content_text text NOT NULL DEFAULT '',
    author_id uuid NOT NULL REFERENCES users(id),
    reason text NOT NULL DEFAULT 'autosave',
    name text,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(document_id,revision_no)
);
CREATE INDEX revisions_document_idx ON document_revisions(document_id,revision_no DESC);

CREATE TABLE document_permissions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    subject_type text NOT NULL CHECK (subject_type IN ('USER','WORKSPACE','ORGANIZATION','PUBLIC_LINK')),
    subject_id uuid,
    role text NOT NULL CHECK (role IN ('OWNER','EDITOR','COMMENTER','VIEWER')),
    password_hash text,
    expires_at timestamptz,
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE NULLS NOT DISTINCT(document_id,subject_type,subject_id)
);
CREATE INDEX document_permissions_subject_idx ON document_permissions(subject_type,subject_id);

CREATE TABLE comments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    parent_id uuid REFERENCES comments(id) ON DELETE CASCADE,
    author_id uuid NOT NULL REFERENCES users(id),
    anchor jsonb,
    body text NOT NULL,
    resolved_at timestamptz,
    resolved_by uuid REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE INDEX comments_document_idx ON comments(document_id,created_at) WHERE deleted_at IS NULL;

CREATE TABLE suggestions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    author_id uuid NOT NULL REFERENCES users(id),
    range_data jsonb NOT NULL,
    previous_value jsonb,
    new_value jsonb NOT NULL,
    status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','ACCEPTED','REJECTED')),
    decided_by uuid REFERENCES users(id),
    decided_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE collab_updates (
    seq bigserial PRIMARY KEY,
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    generation integer NOT NULL,
    author_id uuid NOT NULL REFERENCES users(id),
    update_data bytea NOT NULL CHECK (octet_length(update_data) <= 1048576),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX collab_updates_document_idx ON collab_updates(document_id,generation,seq);

CREATE TABLE favorites (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(user_id,document_id)
);

CREATE TABLE tags (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name citext NOT NULL,
    color text NOT NULL DEFAULT '#5B5BD6',
    UNIQUE(workspace_id,name)
);
CREATE TABLE document_tags (
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    tag_id uuid NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY(document_id,tag_id)
);

CREATE TABLE approval_requests (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    revision_no integer NOT NULL,
    requested_by uuid NOT NULL REFERENCES users(id),
    status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','APPROVED','REJECTED','CANCELLED')),
    required_approvals integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    decided_at timestamptz
);
CREATE TABLE approval_decisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id uuid NOT NULL REFERENCES approval_requests(id) ON DELETE CASCADE,
    reviewer_id uuid NOT NULL REFERENCES users(id),
    decision text NOT NULL CHECK (decision IN ('APPROVED','REJECTED')),
    comment text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(request_id,reviewer_id)
);

CREATE TABLE user_keys (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name text NOT NULL,
    fingerprint text NOT NULL,
    wrapped_key bytea NOT NULL,
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','RETIRED','REVOKED')),
    version integer NOT NULL,
    rotated_from uuid REFERENCES user_keys(id),
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    retired_at timestamptz,
    UNIQUE(user_id,version)
);
CREATE UNIQUE INDEX user_active_key_idx ON user_keys(user_id) WHERE status='ACTIVE';

CREATE TABLE key_role_policies (
    role text PRIMARY KEY,
    permissions jsonb NOT NULL,
    updated_by uuid REFERENCES users(id),
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO key_role_policies(role,permissions) VALUES
('ADMIN','["key:read:any","key:rotate:any","key:revoke:any","policy:manage"]'),
('USER','["key:read:own","key:rotate:own","key:revoke:own"]')
ON CONFLICT DO NOTHING;

CREATE TABLE api_keys (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name text NOT NULL,
    prefix text NOT NULL UNIQUE,
    secret_hash bytea NOT NULL,
    scopes text[] NOT NULL DEFAULT '{}',
    expires_at timestamptz,
    last_used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);
CREATE INDEX api_keys_user_idx ON api_keys(user_id) WHERE revoked_at IS NULL;

CREATE TABLE attachments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    uploader_id uuid NOT NULL REFERENCES users(id),
    name text NOT NULL,
    media_type text NOT NULL,
    size_bytes bigint NOT NULL,
    sha256 text NOT NULL,
    data bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE templates (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid REFERENCES workspaces(id) ON DELETE CASCADE,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    content_json jsonb NOT NULL,
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE notifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type text NOT NULL,
    title text NOT NULL,
    body text NOT NULL DEFAULT '',
    resource_type text,
    resource_id uuid,
    read_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX notifications_user_idx ON notifications(user_id,created_at DESC);

CREATE TABLE ai_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    document_id uuid REFERENCES documents(id) ON DELETE SET NULL,
    title text NOT NULL DEFAULT '새 AI 대화',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE ai_actions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id uuid REFERENCES ai_sessions(id) ON DELETE SET NULL,
    user_id uuid NOT NULL REFERENCES users(id),
    document_id uuid REFERENCES documents(id) ON DELETE SET NULL,
    action text NOT NULL,
    model text NOT NULL,
    prompt_tokens bigint,
    completion_tokens bigint,
    status text NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE activity_logs (
    id bigserial PRIMARY KEY,
    actor_id uuid REFERENCES users(id) ON DELETE SET NULL,
    action text NOT NULL,
    resource_type text NOT NULL,
    resource_id uuid,
    ip inet,
    user_agent text,
    before_data jsonb,
    after_data jsonb,
    metadata jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX activity_logs_created_idx ON activity_logs(created_at DESC);
CREATE INDEX activity_logs_resource_idx ON activity_logs(resource_type,resource_id,created_at DESC);
