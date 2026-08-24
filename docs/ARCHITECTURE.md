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

React assets, Korean web fonts, Go API, the OOXML reader/writer, the pure-Go PDF text extractor and the headless Chromium used for PDF rendering are all present in `muni:v<version>`.

The document agent may call tools before it answers, and every tool resolves
the caller's own document access first: search runs the same ACL-filtered query
the search endpoint uses, and anything that names a document checks
`documentRole` before reading it. Tool arguments come from the model, so a call
that fails — a malformed id, a document the caller cannot see, a panic — is
turned into an error the model is told about rather than one that ends the
request. Rounds and total calls are capped, and running out of rounds asks once
more without tools so the reader still gets an answer. Tools only read: a change
to a document stays a proposal a person accepts.

Yjs updates were append-only, so a document's history — and the payload every
client downloaded on open — grew without bound, and each client pushed a full
document state back on connect. `collab_snapshots` now holds one merged state
per document generation that stands in for every update up to `base_seq`; only
the tail after it is stored and replayed.

The merge itself happens on a client, because merging Yjs binary updates needs a
Yjs implementation and the server has none. When the tail crosses a threshold
the server marks the sync message `compact`, and one writer replies on the
snapshot channel with `Y.encodeStateAsUpdate` of the document it just rebuilt.
That state covers at least everything the server sent, so deleting the updates
up to `base_seq` cannot lose content even when another client wrote concurrently
— the newer updates keep a higher `seq`, and Yjs treats a re-applied update as a
no-op. On connect a client now sends only what the server is missing, computed
from the state vector of the updates it received, instead of the whole document.

Every anchorable block — paragraph, heading, quote, code block, rule, image,
list item, task item and table — carries a `blockId` attribute. Comments,
citations, AI patches, revision diffs and deep links need an anchor that
survives editing, and a document position does not: inserting a paragraph
shifts every offset below it. The editor stamps ids in an `appendTransaction`
so that splitting or pasting a block, both of which copy the source node's
attributes, cannot leave two blocks claiming the same identity; the first block
in document order keeps the id and later copies are re-stamped. Documents that
never pass through the editor — imports, API writes — are stamped server side by
`richdoc.AssignBlockIDs`, and the HTML exporter writes the id as
`data-block-id` so the anchor survives an export and re-import. Runtime egress is needed only when an administrator intentionally configures an internal OIDC issuer or AI gateway.

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
