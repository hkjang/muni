# muni architecture

## Runtime topology

```text
Browser (React + Tiptap + Yjs + IndexedDB)
  ├─ REST / SSE ───────────────┐
  └─ WebSocket CRDT/awareness ─┤
                               ▼
                    muni Go modular monolith
                      ├─ auth / OIDC
                      ├─ document / revision / ACL
                      ├─ comments / suggestions / workflow
                      ├─ collaboration relay
                      ├─ AI gateway
                      ├─ REST / MCP / export
                      └─ settings / key management / audit
                               │
                               ▼
                         PostgreSQL 15+
```

React assets, Korean web fonts, Go API, DOCX writer and PDF rendering dependencies are all present in `muni:v<version>`. Runtime egress is needed only when an administrator intentionally configures an internal OIDC issuer or AI gateway.

## Configuration boundary

Startup needs four immutable bootstrap values: database DSN, bootstrap identity/password and encryption master key. All mutable policy is stored in `app_settings`; secret values use AES-256-GCM envelopes and never return to the admin UI. An empty secret field during an update preserves the current value.

## Authorization order

Every document-aware path follows this order:

```text
authenticate session/API key
  → check API/MCP scope
  → resolve document OWNER/EDITOR/COMMENTER/VIEWER
  → execute DB search/read/write
  → optionally build AI context
  → append audit event
```

Service `ADMIN` can administer all resources; such access is still audited. Search SQL applies ownership, explicit ACL and visibility predicates before returning snippets. AI document context is fetched only after the same permission check.

## Collaboration and revisions

Yjs updates use WebSocket binary channel `0`; awareness/cursor updates use channel `1`. CRDT updates are append-only in `collab_updates`, while the debounced editor snapshot is stored as ProseMirror JSON and plain text in `documents`. Snapshot saves create monotonic revisions with optimistic conflict checking. Restore increments `crdt_generation`, clears old update logs and forces clients to rebuild from the restored snapshot.

IndexedDB stores the local Y.Doc. After reconnect, the client applies server updates and sends one combined state update, which makes offline changes idempotently mergeable.

## Key hierarchy

```text
ENCRYPTION_KEY (external 256-bit master)
  ├─ app setting secrets (OIDC client secret, AI API key)
  └─ per-user data keys
       ├─ ACTIVE vN
       ├─ RETIRED vN-1 (decrypt only)
       └─ REVOKED history
```

Only one user key can be ACTIVE. Rotation locks the current row, retires it and creates the next version atomically. `key_role_policies` makes read/rotate/revoke authority mutable by administrators.

## Approval feature flag

When `workflow.enabled=false`, submit/decision APIs return `WORKFLOW_DISABLED`, approval navigation is hidden and documents remain outside approval states. When enabled, an editor submits a fixed revision, workspace OWNER/MANAGER (or service ADMIN) decides it, and the configured approval count controls publication. A pending submission freezes REST, MCP and WebSocket writes; every state transition closes existing collaboration sockets so clients must re-authorize against the new workflow state.
