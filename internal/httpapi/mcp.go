package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

const modernMCPVersion = "2026-07-28"

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Meta    json.RawMessage `json:"_meta,omitempty"`
}
type mcpTool struct {
	Name        string         `json:"name"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations map[string]any `json:"annotations,omitempty"`
}

func (s *Server) mcpProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	base := scheme + "://" + r.Host
	writeJSON(w, 200, map[string]any{"resource": base + "/mcp", "authorization_servers": []string{base}, "bearer_methods_supported": []string{"header"}, "scopes_supported": allowedAPIScopes})
}

func (s *Server) mcp(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	var request mcpRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if request.JSONRPC != "2.0" {
		s.mcpError(w, request.ID, -32600, "Invalid Request")
		return
	}
	protocol := r.Header.Get("MCP-Protocol-Version")
	if protocol == modernMCPVersion {
		if r.Header.Get("Mcp-Method") != request.Method {
			s.mcpError(w, request.ID, -32020, "Mcp-Method header mismatch")
			return
		}
		if request.Method == "tools/call" {
			var params struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(request.Params, &params)
			if r.Header.Get("Mcp-Name") != params.Name {
				s.mcpError(w, request.ID, -32020, "Mcp-Name header mismatch")
				return
			}
		}
	}
	switch request.Method {
	case "server/discover":
		s.mcpResult(w, request.ID, map[string]any{"protocolVersion": modernMCPVersion, "serverInfo": map[string]any{"name": "muni", "version": s.info.Version}, "capabilities": map[string]any{"tools": map[string]any{}, "extensions": map[string]any{}}})
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(request.Params, &params)
		version := params.ProtocolVersion
		if version == "" {
			version = "2025-06-18"
		}
		s.mcpResult(w, request.ID, map[string]any{"protocolVersion": version, "serverInfo": map[string]any{"name": "muni", "version": s.info.Version}, "capabilities": map[string]any{"tools": map[string]any{"listChanged": false}}})
	case "notifications/initialized":
		w.WriteHeader(202)
	case "ping":
		s.mcpResult(w, request.ID, map[string]any{})
	case "tools/list":
		tools := s.mcpTools(p.Scopes)
		s.mcpResult(w, request.ID, map[string]any{"tools": tools, "ttlMs": 300000, "cacheScope": "private"})
	case "tools/call":
		s.mcpCall(w, r, request, p)
	default:
		s.mcpError(w, request.ID, -32601, "Method not found")
	}
}

func (s *Server) mcpTools(scopes []string) []mcpTool {
	hasRead := contains(scopes, "mcp:read") || contains(scopes, "mcp:write") || len(scopes) == 0
	hasWrite := contains(scopes, "mcp:write") || len(scopes) == 0
	object := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	tools := make([]mcpTool, 0)
	if hasRead {
		tools = append(tools,
			mcpTool{Name: "list_workspaces", Title: "워크스페이스 목록", Description: "현재 사용자가 접근할 수 있는 muni 워크스페이스를 조회합니다.", InputSchema: object(map[string]any{}), Annotations: map[string]any{"readOnlyHint": true}},
			mcpTool{Name: "search_documents", Title: "문서 검색", Description: "ACL을 적용한 뒤 제목과 본문에서 문서를 검색합니다.", InputSchema: object(map[string]any{"query": map[string]any{"type": "string", "minLength": 1}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 50}}, "query"), Annotations: map[string]any{"readOnlyHint": true}},
			mcpTool{Name: "read_document", Title: "문서 읽기", Description: "권한을 확인하고 문서 제목, 구조화 본문, 현재 revision을 읽습니다.", InputSchema: object(map[string]any{"documentId": map[string]any{"type": "string", "format": "uuid"}}, "documentId"), Annotations: map[string]any{"readOnlyHint": true}},
			mcpTool{Name: "list_revisions", Title: "버전 목록", Description: "문서 revision 기록을 조회합니다.", InputSchema: object(map[string]any{"documentId": map[string]any{"type": "string", "format": "uuid"}}, "documentId"), Annotations: map[string]any{"readOnlyHint": true}},
		)
	}
	if hasWrite {
		tools = append(tools,
			mcpTool{Name: "create_document", Title: "문서 생성", Description: "워크스페이스에 새 구조화 문서를 생성합니다.", InputSchema: object(map[string]any{"workspaceId": map[string]any{"type": "string", "format": "uuid"}, "title": map[string]any{"type": "string"}, "content": map[string]any{"type": "object"}}, "workspaceId", "title")},
			mcpTool{Name: "update_document", Title: "문서 수정", Description: "EDITOR 이상 권한으로 제목 또는 Tiptap JSON 본문을 수정합니다.", InputSchema: object(map[string]any{"documentId": map[string]any{"type": "string", "format": "uuid"}, "expectedRevision": map[string]any{"type": "integer", "minimum": 1}, "title": map[string]any{"type": "string"}, "content": map[string]any{"type": "object"}}, "documentId", "expectedRevision")},
			mcpTool{Name: "add_comment", Title: "댓글 추가", Description: "COMMENTER 이상 권한으로 문서 댓글을 추가합니다.", InputSchema: object(map[string]any{"documentId": map[string]any{"type": "string", "format": "uuid"}, "body": map[string]any{"type": "string", "minLength": 1, "maxLength": 5000}}, "documentId", "body")},
			mcpTool{Name: "submit_for_approval", Title: "승인 요청", Description: "관리자가 워크플로를 활성화했을 때 문서를 팀장 검토로 제출합니다.", InputSchema: object(map[string]any{"documentId": map[string]any{"type": "string", "format": "uuid"}}, "documentId")},
		)
	}
	return tools
}

func (s *Server) mcpCall(w http.ResponseWriter, r *http.Request, request mcpRequest, p principal) {
	var params struct {
		Name      string                     `json:"name"`
		Arguments map[string]json.RawMessage `json:"arguments"`
	}
	if json.Unmarshal(request.Params, &params) != nil {
		s.mcpError(w, request.ID, -32602, "Invalid params")
		return
	}
	writeTools := []string{"create_document", "update_document", "add_comment", "submit_for_approval"}
	if contains(writeTools, params.Name) && p.APIKeyID != nil && !contains(p.Scopes, "mcp:write") {
		s.mcpToolError(w, request.ID, "API key requires mcp:write scope")
		return
	}
	result, err := s.executeMCPTool(r, params.Name, params.Arguments, p)
	if err != nil {
		s.mcpToolError(w, request.ID, err.Error())
		return
	}
	encoded, _ := json.Marshal(result)
	s.mcpResult(w, request.ID, map[string]any{"resultType": "complete", "content": []map[string]any{{"type": "text", "text": string(encoded)}}, "structuredContent": result, "isError": false})
	s.audit(r, &p.User.ID, "MCP_"+strings.ToUpper(params.Name), "MCP_TOOL", nil, map[string]any{"tool": params.Name})
}

func (s *Server) executeMCPTool(r *http.Request, name string, args map[string]json.RawMessage, p principal) (any, error) {
	switch name {
	case "list_workspaces":
		rows, err := s.db.Query(r.Context(), `SELECT w.id,w.name,w.slug,w.kind,wm.role FROM workspaces w JOIN workspace_members wm ON wm.workspace_id=w.id WHERE wm.user_id=$1 AND w.deleted_at IS NULL ORDER BY w.name`, p.User.ID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		items := make([]map[string]any, 0)
		for rows.Next() {
			var id uuid.UUID
			var n, slug, kind, role string
			if rows.Scan(&id, &n, &slug, &kind, &role) == nil {
				items = append(items, map[string]any{"id": id, "name": n, "slug": slug, "kind": kind, "role": role})
			}
		}
		return map[string]any{"workspaces": items}, nil
	case "search_documents":
		var q string
		var limit int
		_ = json.Unmarshal(args["query"], &q)
		_ = json.Unmarshal(args["limit"], &limit)
		q = strings.TrimSpace(q)
		if q == "" {
			return nil, fmt.Errorf("query is required")
		}
		if limit < 1 || limit > 50 {
			limit = 20
		}
		rows, err := s.db.Query(r.Context(), `SELECT d.id,d.title,left(d.content_text,500),d.updated_at FROM documents d WHERE d.deleted_at IS NULL
		AND (to_tsvector('simple',d.title||' '||d.content_text)@@websearch_to_tsquery('simple',$1) OR d.title ILIKE '%'||$1||'%' OR d.content_text ILIKE '%'||$1||'%')
		AND (d.owner_id=$2 OR d.visibility='ORGANIZATION' OR EXISTS(SELECT 1 FROM workspace_members wm WHERE wm.workspace_id=d.workspace_id AND wm.user_id=$2 AND d.visibility='WORKSPACE') OR EXISTS(SELECT 1 FROM document_permissions dp WHERE dp.document_id=d.id AND dp.subject_type='USER' AND dp.subject_id=$2) OR $3='ADMIN') ORDER BY d.updated_at DESC LIMIT $4`, q, p.User.ID, p.User.Role, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		items := make([]map[string]any, 0)
		for rows.Next() {
			var id uuid.UUID
			var title, snippet string
			var updated any
			if rows.Scan(&id, &title, &snippet, &updated) == nil {
				items = append(items, map[string]any{"id": id, "title": title, "snippet": snippet, "updatedAt": updated})
			}
		}
		return map[string]any{"documents": items}, nil
	case "read_document":
		id, err := mcpUUID(args, "documentId")
		if err != nil {
			return nil, err
		}
		role, err := s.documentRole(r.Context(), p.User, id, false)
		if err != nil {
			return nil, fmt.Errorf("document access denied")
		}
		var title string
		var content json.RawMessage
		var revision int
		if err := s.db.QueryRow(r.Context(), `SELECT title,content_json,revision_no FROM documents WHERE id=$1`, id).Scan(&title, &content, &revision); err != nil {
			return nil, err
		}
		return map[string]any{"id": id, "title": title, "content": content, "revision": revision, "permission": role}, nil
	case "list_revisions":
		id, err := mcpUUID(args, "documentId")
		if err != nil {
			return nil, err
		}
		if _, err = s.documentRole(r.Context(), p.User, id, false); err != nil {
			return nil, fmt.Errorf("document access denied")
		}
		rows, err := s.db.Query(r.Context(), `SELECT revision_no,reason,created_at FROM document_revisions WHERE document_id=$1 ORDER BY revision_no DESC LIMIT 100`, id)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		items := make([]map[string]any, 0)
		for rows.Next() {
			var revision int
			var reason string
			var created any
			if rows.Scan(&revision, &reason, &created) == nil {
				items = append(items, map[string]any{"revision": revision, "reason": reason, "createdAt": created})
			}
		}
		return map[string]any{"revisions": items}, nil
	case "create_document":
		workspaceID, err := mcpUUID(args, "workspaceId")
		if err != nil {
			return nil, err
		}
		var title string
		_ = json.Unmarshal(args["title"], &title)
		title = strings.TrimSpace(title)
		if title == "" {
			return nil, fmt.Errorf("title is required")
		}
		var memberRole, workspaceKind string
		if s.db.QueryRow(r.Context(), `SELECT wm.role,w.kind FROM workspace_members wm JOIN workspaces w ON w.id=wm.workspace_id WHERE wm.workspace_id=$1 AND wm.user_id=$2`, workspaceID, p.User.ID).Scan(&memberRole, &workspaceKind) != nil || memberRole == "VIEWER" {
			return nil, fmt.Errorf("workspace access denied")
		}
		content := args["content"]
		if len(content) == 0 {
			content = json.RawMessage(`{"type":"doc","content":[{"type":"paragraph"}]}`)
		}
		if !validDocumentJSON(content) {
			return nil, fmt.Errorf("invalid Tiptap document JSON")
		}
		id := uuid.New()
		text := extractDocumentText(content)
		visibility := "RESTRICTED"
		if workspaceKind != "PERSONAL" {
			visibility = "WORKSPACE"
		}
		_, err = s.db.Exec(r.Context(), `WITH inserted AS (INSERT INTO documents(id,workspace_id,owner_id,title,visibility,content_json,content_text,revision_no) VALUES($1,$2,$3,$4,$5,$6,$7,1) RETURNING id) INSERT INTO document_revisions(document_id,revision_no,content_json,content_text,author_id,reason) SELECT id,1,$6,$7,$3,'mcp:create' FROM inserted`, id, workspaceID, p.User.ID, truncate(title, 240), visibility, content, text)
		if err != nil {
			return nil, err
		}
		return map[string]any{"id": id, "title": title, "revision": 1}, nil
	case "update_document":
		id, err := mcpUUID(args, "documentId")
		if err != nil {
			return nil, err
		}
		role, err := s.documentRole(r.Context(), p.User, id, false)
		if err != nil || roleRank[role] < roleRank["EDITOR"] {
			return nil, fmt.Errorf("editor permission required")
		}
		var expected int
		_ = json.Unmarshal(args["expectedRevision"], &expected)
		var title string
		_ = json.Unmarshal(args["title"], &title)
		content := args["content"]
		var current int
		var workflowStatus string
		err = s.db.QueryRow(r.Context(), `SELECT revision_no,workflow_status FROM documents WHERE id=$1`, id).Scan(&current, &workflowStatus)
		if err != nil {
			return nil, err
		}
		if workflowStatus == "PENDING" {
			return nil, fmt.Errorf("document is pending approval and cannot be changed")
		}
		if current != expected {
			return nil, fmt.Errorf("revision conflict: current revision is %d", current)
		}
		if len(content) > 0 && !validDocumentJSON(content) {
			return nil, fmt.Errorf("invalid Tiptap document JSON")
		}
		var next int
		var text string
		if len(content) > 0 {
			text = extractDocumentText(content)
			err = s.db.QueryRow(r.Context(), `UPDATE documents SET title=CASE WHEN $2='' THEN title ELSE $2 END,content_json=$3,content_text=$4,revision_no=revision_no+1,updated_at=now() WHERE id=$1 AND revision_no=$5 RETURNING revision_no`, id, truncate(strings.TrimSpace(title), 240), content, text, expected).Scan(&next)
		} else {
			err = s.db.QueryRow(r.Context(), `UPDATE documents SET title=CASE WHEN $2='' THEN title ELSE $2 END,updated_at=now() WHERE id=$1 AND revision_no=$3 RETURNING revision_no`, id, truncate(strings.TrimSpace(title), 240), expected).Scan(&next)
		}
		if err != nil {
			return nil, err
		}
		if len(content) > 0 {
			_, err = s.db.Exec(r.Context(), `INSERT INTO document_revisions(document_id,revision_no,content_json,content_text,author_id,reason) VALUES($1,$2,$3,$4,$5,'mcp:update')`, id, next, content, text, p.User.ID)
		}
		return map[string]any{"id": id, "revision": next}, err
	case "add_comment":
		id, err := mcpUUID(args, "documentId")
		if err != nil {
			return nil, err
		}
		role, err := s.documentRole(r.Context(), p.User, id, false)
		if err != nil || roleRank[role] < roleRank["COMMENTER"] {
			return nil, fmt.Errorf("commenter permission required")
		}
		var body string
		_ = json.Unmarshal(args["body"], &body)
		body = strings.TrimSpace(body)
		if body == "" || len([]rune(body)) > 5000 {
			return nil, fmt.Errorf("body must be 1-5000 characters")
		}
		commentID := uuid.New()
		_, err = s.db.Exec(r.Context(), `INSERT INTO comments(id,document_id,author_id,body) VALUES($1,$2,$3,$4)`, commentID, id, p.User.ID, body)
		return map[string]any{"id": commentID}, err
	case "submit_for_approval":
		id, err := mcpUUID(args, "documentId")
		if err != nil {
			return nil, err
		}
		all, err := s.settings.GetAll(r.Context(), false)
		if err != nil || !all.Workflow.Enabled {
			return nil, fmt.Errorf("approval workflow is disabled")
		}
		role, err := s.documentRole(r.Context(), p.User, id, false)
		if err != nil || roleRank[role] < roleRank["EDITOR"] {
			return nil, fmt.Errorf("editor permission required")
		}
		requestID := uuid.New()
		result, err := s.db.Exec(r.Context(), `WITH updated AS (
			UPDATE documents SET workflow_status='PENDING',status='REVIEW',updated_at=now()
			WHERE id=$2 AND workflow_status<>'PENDING' RETURNING revision_no
		) INSERT INTO approval_requests(id,document_id,revision_no,requested_by,required_approvals)
		SELECT $1,$2,revision_no,$3,$4 FROM updated`, requestID, id, p.User.ID, all.Workflow.RequiredApprovals)
		if err != nil {
			return nil, err
		}
		if result.RowsAffected() == 0 {
			return nil, fmt.Errorf("document is already pending approval")
		}
		s.hub.CloseDocument(id)
		return map[string]any{"id": requestID, "status": "PENDING"}, nil
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func mcpUUID(args map[string]json.RawMessage, key string) (uuid.UUID, error) {
	var value string
	_ = json.Unmarshal(args[key], &value)
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%s must be a UUID", key)
	}
	return id, nil
}
func (s *Server) mcpResult(w http.ResponseWriter, id any, result any) {
	writeJSON(w, 200, map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}
func (s *Server) mcpError(w http.ResponseWriter, id any, code int, message string) {
	writeJSON(w, 200, map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message}})
}
func (s *Server) mcpToolError(w http.ResponseWriter, id any, message string) {
	s.mcpResult(w, id, map[string]any{"resultType": "complete", "content": []map[string]any{{"type": "text", "text": message}}, "isError": true})
}
