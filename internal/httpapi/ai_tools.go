package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/richdoc"
)

// A tool the model may call. Every tool reads; none of them writes. An agent
// that can quietly change a document is a much larger promise than an agent
// that can look things up, and the change should be a proposal a person accepts
// rather than something that already happened.
type aiTool struct {
	Name        string
	Description string
	Parameters  map[string]any
	Run         func(ctx context.Context, s *Server, user User, args map[string]any) (any, error)
}

const (
	maxToolRounds     = 6
	maxToolCallsTotal = 12
	maxToolTextRunes  = 8000
)

func aiTools() []aiTool {
	return []aiTool{
		{
			Name: "search_documents",
			Description: "이 사용자가 접근할 수 있는 문서를 제목과 본문으로 검색합니다. " +
				"문서 식별자와 제목, 일치한 부분의 발췌를 돌려줍니다.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "검색어"},
					"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 20, "description": "최대 결과 수"},
				},
				"required": []string{"query"},
			},
			Run: runSearchDocuments,
		},
		{
			Name:        "read_document",
			Description: "문서 하나의 제목과 본문을 읽습니다. 긴 문서는 앞부분만 돌려줍니다.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"documentId": map[string]any{"type": "string", "description": "문서 UUID"},
				},
				"required": []string{"documentId"},
			},
			Run: runReadDocument,
		},
		{
			Name: "get_document_outline",
			Description: "문서의 제목 구조를 단계와 블록 식별자와 함께 돌려줍니다. " +
				"어느 부분을 인용할지 고를 때 사용하세요.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"documentId": map[string]any{"type": "string", "description": "문서 UUID"},
				},
				"required": []string{"documentId"},
			},
			Run: runDocumentOutline,
		},
		{
			Name: "compare_revisions",
			Description: "문서의 두 버전을 블록 단위로 비교해 무엇이 추가·삭제·변경·이동되었는지 돌려줍니다. " +
				"버전 번호를 모르면 list_revisions를 먼저 부르세요.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"documentId": map[string]any{"type": "string", "description": "문서 UUID"},
					"from":       map[string]any{"type": "integer", "minimum": 1, "description": "이전 버전 번호"},
					"to":         map[string]any{"type": "integer", "minimum": 1, "description": "이후 버전 번호"},
				},
				"required": []string{"documentId", "from", "to"},
			},
			Run: runCompareRevisions,
		},
		{
			Name:        "list_revisions",
			Description: "문서의 버전 목록을 최신순으로 돌려줍니다.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"documentId": map[string]any{"type": "string", "description": "문서 UUID"},
				},
				"required": []string{"documentId"},
			},
			Run: runListRevisions,
		},
	}
}

// toolDefinitions renders the tools in the shape OpenAI-compatible providers
// expect.
func toolDefinitions(tools []aiTool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  tool.Parameters,
			},
		})
	}
	return out
}

var errToolNotFound = errors.New("알 수 없는 도구입니다")

// runTool dispatches one call. The name and the arguments come from the model,
// so a malformed call must become an error the model can read rather than a
// panic that takes the request down with it.
func runTool(ctx context.Context, s *Server, user User, name string, rawArgs string) (result any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			s.logger.Error("ai tool panicked", "tool", name, "panic", fmt.Sprint(recovered))
			result, err = nil, errors.New("도구 실행 중 오류가 발생했습니다")
		}
	}()
	args := map[string]any{}
	if strings.TrimSpace(rawArgs) != "" {
		if unmarshalErr := json.Unmarshal([]byte(rawArgs), &args); unmarshalErr != nil {
			return nil, fmt.Errorf("도구 인자를 읽지 못했습니다: %w", unmarshalErr)
		}
	}
	for _, tool := range aiTools() {
		if tool.Name == name {
			return tool.Run(ctx, s, user, args)
		}
	}
	return nil, errToolNotFound
}

func stringArg(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}

func intArg(args map[string]any, key string, fallback int) int {
	switch value := args[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case string:
		var parsed int
		if _, err := fmt.Sscanf(value, "%d", &parsed); err == nil {
			return parsed
		}
	}
	return fallback
}

// documentArg resolves a document the tool was pointed at, refusing anything
// the person running the agent cannot read.
func (s *Server) documentArg(ctx context.Context, user User, args map[string]any) (uuid.UUID, error) {
	id, err := uuid.Parse(stringArg(args, "documentId"))
	if err != nil {
		return uuid.Nil, errors.New("documentId가 UUID 형식이 아닙니다")
	}
	if _, err := s.documentRole(ctx, user, id, false); err != nil {
		return uuid.Nil, errors.New("이 문서를 읽을 권한이 없습니다")
	}
	return id, nil
}

func runSearchDocuments(ctx context.Context, s *Server, user User, args map[string]any) (any, error) {
	query := stringArg(args, "query")
	if query == "" {
		return nil, errors.New("query가 필요합니다")
	}
	limit := intArg(args, "limit", 8)
	if limit < 1 || limit > 20 {
		limit = 8
	}
	items, err := s.searchVisibleDocuments(ctx, user, query, limit)
	if err != nil {
		return nil, errors.New("검색에 실패했습니다")
	}
	results := make([]map[string]any, 0, len(items))
	for _, item := range items {
		results = append(results, map[string]any{
			"documentId": item["id"],
			"title":      item["title"],
			"snippet":    item["snippet"],
			"updatedAt":  item["updatedAt"],
			"owner":      item["ownerName"],
		})
	}
	return map[string]any{"results": results, "count": len(results)}, nil
}

func runReadDocument(ctx context.Context, s *Server, user User, args map[string]any) (any, error) {
	id, err := s.documentArg(ctx, user, args)
	if err != nil {
		return nil, err
	}
	var title, text string
	var revision int
	if err := s.db.QueryRow(ctx, `SELECT title,content_text,revision_no FROM documents WHERE id=$1`, id).
		Scan(&title, &text, &revision); err != nil {
		return nil, errors.New("문서를 찾을 수 없습니다")
	}
	truncated := len([]rune(text)) > maxToolTextRunes
	return map[string]any{
		"documentId": id,
		"title":      title,
		"revision":   revision,
		"text":       truncateRunes(text, maxToolTextRunes),
		"truncated":  truncated,
	}, nil
}

func runDocumentOutline(ctx context.Context, s *Server, user User, args map[string]any) (any, error) {
	id, err := s.documentArg(ctx, user, args)
	if err != nil {
		return nil, err
	}
	var title string
	var content json.RawMessage
	if err := s.db.QueryRow(ctx, `SELECT title,content_json FROM documents WHERE id=$1`, id).
		Scan(&title, &content); err != nil {
		return nil, errors.New("문서를 찾을 수 없습니다")
	}
	document, err := richdoc.Parse(content)
	if err != nil {
		return nil, errors.New("문서 내용을 읽지 못했습니다")
	}
	sections := make([]map[string]any, 0, 16)
	var walk func(*richdoc.Node)
	walk = func(node *richdoc.Node) {
		if node == nil {
			return
		}
		if node.Type == "heading" {
			sections = append(sections, map[string]any{
				"level":   node.AttrInt("level", 1),
				"text":    node.PlainText(),
				"blockId": node.AttrString(richdoc.BlockIDAttr),
			})
		}
		for _, child := range node.Content {
			walk(child)
		}
	}
	walk(document)
	return map[string]any{"documentId": id, "title": title, "sections": sections}, nil
}

func runListRevisions(ctx context.Context, s *Server, user User, args map[string]any) (any, error) {
	id, err := s.documentArg(ctx, user, args)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT dr.revision_no,dr.reason,dr.created_at,u.display_name
		FROM document_revisions dr JOIN users u ON u.id=dr.author_id
		WHERE dr.document_id=$1 ORDER BY dr.revision_no DESC LIMIT 30`, id)
	if err != nil {
		return nil, errors.New("버전 기록을 불러오지 못했습니다")
	}
	defer rows.Close()
	items := make([]map[string]any, 0, 30)
	for rows.Next() {
		var revision int
		var reason *string
		var created any
		var author string
		if rows.Scan(&revision, &reason, &created, &author) == nil {
			items = append(items, map[string]any{
				"revision": revision, "reason": reason, "createdAt": created, "author": author,
			})
		}
	}
	return map[string]any{"documentId": id, "revisions": items}, nil
}

func runCompareRevisions(ctx context.Context, s *Server, user User, args map[string]any) (any, error) {
	id, err := s.documentArg(ctx, user, args)
	if err != nil {
		return nil, err
	}
	from := intArg(args, "from", 0)
	to := intArg(args, "to", 0)
	if from < 1 || to < 1 {
		return nil, errors.New("from과 to는 1 이상의 버전 번호여야 합니다")
	}
	before, err := s.loadRevision(ctx, id, from)
	if err != nil {
		return nil, fmt.Errorf("버전 %d을 찾을 수 없습니다", from)
	}
	after, err := s.loadRevision(ctx, id, to)
	if err != nil {
		return nil, fmt.Errorf("버전 %d을 찾을 수 없습니다", to)
	}
	beforeDocument, err := richdoc.Parse(before.content)
	if err != nil {
		return nil, errors.New("이전 버전을 읽지 못했습니다")
	}
	afterDocument, err := richdoc.Parse(after.content)
	if err != nil {
		return nil, errors.New("이후 버전을 읽지 못했습니다")
	}
	result := richdoc.Diff(beforeDocument, afterDocument)

	// The unchanged blocks are the bulk of a document and say nothing about the
	// edit; sending them would crowd out the answer.
	changes := make([]map[string]any, 0, len(result.Blocks))
	for _, block := range result.Blocks {
		if block.Status == richdoc.BlockUnchanged {
			continue
		}
		changes = append(changes, map[string]any{
			"status": block.Status, "type": block.Type, "blockId": block.BlockID,
			"before": truncateRunes(block.Before, 600), "after": truncateRunes(block.After, 600),
		})
	}
	return map[string]any{
		"documentId": id, "from": from, "to": to,
		"summary": result.Summary, "changes": changes,
	}, nil
}
