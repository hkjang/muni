package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Documents imported before the importers learned to give a picture a line of
// its own are still in the database with the image inside a paragraph. The
// editor does not lose the picture when it meets that shape — it refuses the
// whole document. These check the repair on the two doors a ProseMirror
// document goes out of.

const paragraphWithAPicture = `{"type":"doc","content":[{"type":"paragraph","content":[` +
	`{"type":"text","text":"사진 앞"},` +
	`{"type":"image","attrs":{"src":"/api/v1/attachments/00000000-0000-0000-0000-000000000001"}}]}]}`

func storeBrokenContent(t *testing.T, srv *serverUnderTest, documentID uuid.UUID) {
	t.Helper()
	if _, err := srv.db.Exec(context.Background(),
		`UPDATE documents SET content_json=$2 WHERE id=$1`, documentID, paragraphWithAPicture); err != nil {
		t.Fatal(err)
	}
}

// blocksOf reports the block types of a document as the client received it.
func blocksOf(t *testing.T, raw string) []string {
	t.Helper()
	var document struct {
		Content []struct {
			Type string `json:"type"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(raw), &document); err != nil {
		t.Fatalf("본문을 읽지 못했습니다: %v\n%s", err, raw)
	}
	types := make([]string, 0, len(document.Content))
	for _, block := range document.Content {
		types = append(types, block.Type)
	}
	return types
}

func TestTheEditorGetsAStoredPictureAsABlock(t *testing.T) {
	srv := newServerUnderTest(t)
	documentID := ownedDocument(t, srv, "예전에 가져온 문서")
	storeBrokenContent(t, srv, documentID)

	resp, err := srv.admin.Get(srv.URL + "/api/v1/documents/" + documentID.String())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("문서 열기 = %d %s", resp.StatusCode, raw)
	}
	var out struct {
		Data struct {
			Content json.RawMessage `json:"content"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	content := out.Data.Content
	blocks := blocksOf(t, string(content))
	if len(blocks) != 2 || blocks[0] != "paragraph" || blocks[1] != "image" {
		t.Fatalf("블록 = %v, 문단과 이미지를 기대했습니다: %s", blocks, content)
	}
	if !strings.Contains(string(content), "사진 앞") {
		t.Errorf("주변 문장이 사라졌습니다: %s", content)
	}
}

func TestASharedLinkGetsAStoredPictureAsABlock(t *testing.T) {
	srv := newServerUnderTest(t)
	enableLinks(t, srv, true)
	documentID := ownedDocument(t, srv, "링크로 공유한 예전 문서")
	storeBrokenContent(t, srv, documentID)
	token, _ := makeLink(t, srv, documentID, map[string]any{})

	status, data := openLink(t, srv, token, "")
	if status != 200 {
		t.Fatalf("링크 열기 = %d %v", status, data)
	}
	content, _ := data["content"].(string)
	blocks := blocksOf(t, content)
	if len(blocks) != 2 || blocks[1] != "image" {
		t.Fatalf("블록 = %v: %s", blocks, content)
	}
}
