package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/richdoc"
	"github.com/hkjang/muni/internal/settings"
)

const (
	maxPatchBlocks = 120
	maxPatchEdits  = 20
	maxPatchChars  = 4000
)

// aiPatchInput is a request to have the model propose changes to a document.
type aiPatchInput struct {
	Instruction string   `json:"instruction"`
	BlockIDs    []string `json:"blockIds"`
}

// proposedEdit is one block rewrite the model suggested.
type proposedEdit struct {
	BlockID string `json:"blockId"`
	NewText string `json:"newText"`
	Reason  string `json:"reason"`
}

// proposeDocumentPatch asks the model to rewrite whole blocks and records the
// result as pending suggestions.
//
// Nothing is applied. The AI's output lands in the same review queue a
// colleague's suggestion does, anchored to the block it is about rather than to
// a document position that moves when anyone types above it.
func (s *Server) proposeDocumentPatch(w http.ResponseWriter, r *http.Request) {
	documentID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	role, err := s.documentRole(r.Context(), p.User, documentID, false)
	if !documentAllowed(w, role, err, "COMMENTER") {
		return
	}
	var input aiPatchInput
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Instruction = strings.TrimSpace(input.Instruction)
	if input.Instruction == "" {
		writeError(w, 400, "INSTRUCTION_REQUIRED", "어떻게 고칠지 알려 주세요.")
		return
	}

	all, err := s.settings.GetAll(r.Context(), true)
	if err != nil || !all.AI.Enabled {
		writeError(w, 409, "AI_DISABLED", "관리자가 AI 기능을 설정하지 않았습니다.")
		return
	}
	config := normalizeAI(all.AI)
	if config.BaseURL == "" || config.Model == "" {
		writeError(w, 409, "AI_NOT_CONFIGURED", "관리자 설정에서 AI API 주소와 모델 이름을 입력해 주세요.")
		return
	}

	var content json.RawMessage
	var title string
	if err := s.db.QueryRow(r.Context(), `SELECT title,content_json FROM documents WHERE id=$1`, documentID).
		Scan(&title, &content); err != nil {
		writeError(w, 404, "DOCUMENT_NOT_FOUND", "문서를 찾을 수 없습니다.")
		return
	}
	document, err := richdoc.Parse(content)
	if err != nil {
		writeError(w, 500, "DOCUMENT_UNREADABLE", "문서 내용을 읽지 못했습니다.")
		return
	}

	blocks := patchableBlocks(document, input.BlockIDs)
	if len(blocks) == 0 {
		writeError(w, 400, "NO_PATCHABLE_BLOCKS", "고칠 수 있는 블록이 없습니다. 문서를 한 번 열어 저장한 뒤 다시 시도해 주세요.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(config.TimeoutSeconds)*time.Second)
	defer cancel()
	invocation := startAIInvocation(p.User, "patch", config.Model, &documentID, len([]rune(input.Instruction)))
	invocation.note("blocks", len(blocks))
	edits, usage, err := s.requestPatch(ctx, config, title, input.Instruction, blocks)
	invocation.usage = usage
	if err != nil {
		var upstream *aiUpstreamError
		code := "AI_UPSTREAM_UNAVAILABLE"
		if errors.As(err, &upstream) {
			code = "AI_UPSTREAM_ERROR"
		}
		s.logger.Warn("ai patch failed", "model", config.Model, "error", err.Error())
		invocation.fail(code, err)
		s.record(r.Context(), invocation)
		writeError(w, 502, code, err.Error())
		return
	}

	known := map[string]string{}
	for _, block := range blocks {
		known[block.id] = block.text
	}
	created := make([]map[string]any, 0, len(edits))
	for _, edit := range edits {
		previous, exists := known[edit.BlockID]
		// A block the model invented, or a rewrite that changes nothing, is
		// not worth putting in front of a reviewer.
		if !exists || strings.TrimSpace(edit.NewText) == "" || edit.NewText == previous {
			continue
		}
		if len([]rune(edit.NewText)) > maxPatchChars {
			continue
		}
		id := uuid.New()
		rangeData, _ := json.Marshal(map[string]any{"blockId": edit.BlockID})
		newValue, _ := json.Marshal(edit.NewText)
		previousValue, _ := json.Marshal(previous)
		if _, err := s.db.Exec(r.Context(),
			`INSERT INTO suggestions(id,document_id,author_id,range_data,previous_value,new_value,block_id,origin,note)
			 VALUES($1,$2,$3,$4,$5,$6,$7,'AI',$8)`,
			id, documentID, p.User.ID, rangeData, previousValue, newValue, edit.BlockID, nullString(truncate(edit.Reason, 400))); err != nil {
			s.logger.Warn("ai suggestion was not stored", "document_id", documentID, "error", err)
			continue
		}
		created = append(created, map[string]any{
			"id": id, "blockId": edit.BlockID, "previousValue": previous,
			"newValue": edit.NewText, "note": edit.Reason,
		})
		if len(created) >= maxPatchEdits {
			break
		}
	}

	invocation.note("suggestions", len(created))
	s.record(r.Context(), invocation)
	s.audit(r, &p.User.ID, "AI_PROPOSE_PATCH", "DOCUMENT", &documentID,
		map[string]any{"model": config.Model, "blocks": len(blocks), "suggestions": len(created)})
	writeData(w, 201, map[string]any{"suggestions": created, "count": len(created)})
}

type patchBlock struct {
	id   string
	kind string
	text string
}

// patchableBlocks lists the blocks the model may rewrite. Only blocks with a
// stable id qualify: without one there is nothing to anchor a proposal to.
func patchableBlocks(document *richdoc.Node, only []string) []patchBlock {
	wanted := map[string]bool{}
	for _, id := range only {
		wanted[id] = true
	}
	blocks := make([]patchBlock, 0, 32)
	var walk func(*richdoc.Node)
	walk = func(node *richdoc.Node) {
		if node == nil || len(blocks) >= maxPatchBlocks {
			return
		}
		switch node.Type {
		case "paragraph", "heading", "codeBlock":
			id := node.AttrString(richdoc.BlockIDAttr)
			text := node.PlainText()
			if id != "" && strings.TrimSpace(text) != "" && (len(wanted) == 0 || wanted[id]) {
				blocks = append(blocks, patchBlock{id: id, kind: node.Type, text: text})
			}
			return
		}
		for _, child := range node.Content {
			walk(child)
		}
	}
	walk(document)
	return blocks
}

func (s *Server) requestPatch(ctx context.Context, config settings.AI, title, instruction string, blocks []patchBlock) ([]proposedEdit, aiUsage, error) {
	var document strings.Builder
	for _, block := range blocks {
		fmt.Fprintf(&document, "[%s] (%s) %s\n", block.id, block.kind, truncateRunes(block.text, 1200))
	}

	prompt := "문서 제목: " + title + "\n\n" +
		"아래는 문서의 블록 목록입니다. 각 줄은 [블록ID] (종류) 내용 형식입니다.\n\n" +
		document.String() + "\n" +
		"요청: " + instruction + "\n\n" +
		"고쳐야 할 블록만 골라 JSON으로만 답하세요. 형식은 다음과 같습니다.\n" +
		`{"edits":[{"blockId":"...","newText":"고친 전체 내용","reason":"왜 고쳤는지 한 문장"}]}` + "\n" +
		"newText는 그 블록을 통째로 대체할 내용이며 서식 없는 평문입니다. " +
		"목록에 없는 블록ID를 만들지 말고, 고칠 필요가 없으면 edits를 빈 배열로 두세요. " +
		"설명이나 코드 블록 표시 없이 JSON만 출력하세요."

	response, _, err := s.call(ctx, aiRequest{
		config:    config,
		messages:  []aiMessage{{Role: "user", Content: prompt}},
		maxTokens: config.MaxTokens,
	})
	if err != nil {
		return nil, aiUsage{}, err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 8<<20))

	var parsed completionResponse
	if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Choices) == 0 {
		return nil, aiUsage{}, fmt.Errorf("AI 응답 형식을 이해하지 못했습니다: %s", truncate(string(body), 300))
	}
	edits, parseErr := parsePatchEdits(contentText(parsed.Choices[0].Message.Content))
	return edits, readUsage(parsed.Usage), parseErr
}

// parsePatchEdits reads the model's answer. Models wrap JSON in prose or a code
// fence often enough that insisting on a clean document would fail for reasons
// that have nothing to do with the edit.
func parsePatchEdits(answer string) ([]proposedEdit, error) {
	answer = strings.TrimSpace(answer)
	if fence := strings.Index(answer, "```"); fence >= 0 {
		rest := answer[fence+3:]
		if newline := strings.IndexByte(rest, '\n'); newline >= 0 {
			rest = rest[newline+1:]
		}
		if end := strings.Index(rest, "```"); end >= 0 {
			rest = rest[:end]
		}
		answer = strings.TrimSpace(rest)
	}
	start := strings.IndexByte(answer, '{')
	end := strings.LastIndexByte(answer, '}')
	if start < 0 || end <= start {
		return nil, errors.New("AI가 제안 형식(JSON)으로 답하지 않았습니다")
	}
	var envelope struct {
		Edits []proposedEdit `json:"edits"`
	}
	if err := json.Unmarshal([]byte(answer[start:end+1]), &envelope); err != nil {
		return nil, errors.New("AI 제안을 읽지 못했습니다")
	}
	return envelope.Edits, nil
}
