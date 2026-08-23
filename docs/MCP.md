# muni MCP

Endpoint: `POST https://<host>/mcp`

Create a personal key with `mcp:read` and, only when required, `mcp:write`. Send it in every request:

```http
Authorization: Bearer muni_<prefix>_<secret>
Content-Type: application/json
MCP-Protocol-Version: 2026-07-28
Mcp-Method: tools/list
```

Modern discovery:

```json
{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{},"_meta":{"io.modelcontextprotocol/clientInfo":{"name":"internal-agent","version":"1.0"}}}
```

Tool call example:

```http
Mcp-Method: tools/call
Mcp-Name: search_documents
```

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/call",
  "params": {
    "name": "search_documents",
    "arguments": { "query": "2027 사업 계획", "limit": 10 }
  }
}
```

Provided tools are deterministic and filtered by key scope:

- Read: `list_workspaces`, `search_documents`, `read_document`, `list_revisions`
- Write: `create_document`, `update_document`, `add_comment`, `submit_for_approval`

Legacy clients may call `initialize` with protocol `2025-06-18` or `2025-11-25`; muni returns a compatible tools capability. Tool execution errors are returned with `isError: true` so the calling model can correct its request.
