package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

type attachmentRow struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	SizeBytes int64     `json:"sizeBytes"`
	InUse     bool      `json:"inUse"`
	Uploaded  string    `json:"uploadedBy"`
}

func listAttachmentsOf(t *testing.T, srv *serverUnderTest, documentID uuid.UUID) []attachmentRow {
	t.Helper()
	resp, err := srv.admin.Get(srv.URL + "/api/v1/documents/" + documentID.String() + "/attachments")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("list = %d %s", resp.StatusCode, raw)
	}
	var out struct {
		Data []attachmentRow `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out.Data
}

func uploadAttachment(t *testing.T, srv *serverUnderTest, documentID uuid.UUID, name string, content []byte) uuid.UUID {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", name)
	_, _ = part.Write(content)
	writer.Close()
	req, _ := http.NewRequest("POST",
		srv.URL+"/api/v1/documents/"+documentID.String()+"/attachments", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := srv.admin.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		t.Fatalf("upload = %d %s", resp.StatusCode, raw)
	}
	var out struct {
		Data struct {
			ID uuid.UUID `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out.Data.ID
}

// The point of listing attachments is not "what is attached" but "what is
// still being used". A file dragged in and then deleted from the text keeps
// its bytes with nothing pointing at them, and those are the ones worth
// clearing — so getting this flag wrong makes the whole panel useless in
// either direction: it either hides the waste or tells someone to delete a
// picture that is still on the page.
func TestAnAttachmentKnowsWhetherTheDocumentStillUsesIt(t *testing.T) {
	srv := newServerUnderTest(t)
	ctx := context.Background()
	documentID := ownedDocument(t, srv, "첨부 있는 문서")

	used := uploadAttachment(t, srv, documentID, "쓰는그림.png", []byte("PNG-1"))
	orphan := uploadAttachment(t, srv, documentID, "버려진그림.png", []byte("PNG-2"))

	// Put a reference to the first one into the document, the way the editor
	// does — by the URL that carries its id.
	content := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"본문"}]},` +
		`{"type":"image","attrs":{"src":"/api/v1/attachments/` + used.String() + `"}}]}`
	if _, err := srv.db.Exec(ctx,
		`UPDATE documents SET content_json = $2::jsonb WHERE id = $1`, documentID, content); err != nil {
		t.Fatal(err)
	}

	rows := listAttachmentsOf(t, srv, documentID)
	if len(rows) != 2 {
		t.Fatalf("got %d attachments", len(rows))
	}
	for _, row := range rows {
		switch row.ID {
		case used:
			if !row.InUse {
				t.Error("a file the document still points at is in use")
			}
		case orphan:
			if row.InUse {
				t.Error("a file nothing points at is not in use")
			}
		default:
			t.Errorf("unexpected attachment %v", row.ID)
		}
		if row.Uploaded == "" {
			t.Error("who uploaded it is part of what makes the list readable")
		}
	}
}

func TestDeletingAnAttachmentActuallyRemovesIt(t *testing.T) {
	srv := newServerUnderTest(t)
	documentID := ownedDocument(t, srv, "삭제할 첨부")
	id := uploadAttachment(t, srv, documentID, "지울파일.txt", []byte("bytes"))

	req, _ := http.NewRequest("DELETE", srv.URL+"/api/v1/attachments/"+id.String(), nil)
	resp, err := srv.admin.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("delete = %d", resp.StatusCode)
	}
	if rows := listAttachmentsOf(t, srv, documentID); len(rows) != 0 {
		t.Fatalf("still listed: %+v", rows)
	}
	// And the bytes are gone, not just hidden.
	var remaining int
	_ = srv.db.QueryRow(context.Background(),
		`SELECT count(*) FROM attachments WHERE id=$1`, id).Scan(&remaining)
	if remaining != 0 {
		t.Fatal("the row survived the delete")
	}
}
