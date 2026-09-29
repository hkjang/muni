package httpapi

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// contextNoticeFragment is the middle of the sentence truncateRunes appends
// when it shortens a value for an AI prompt. The tests below never want to see
// it, and matching a fragment rather than the whole sentence keeps them honest
// if the wording is ever reworded for the model's benefit.
const contextNoticeFragment = "문서 컨텍스트가 길어"

// A stored name is data, not a prompt. These walk the routes that write one —
// the attachment upload and the document import — with a value past the 240
// rune ceiling those routes impose, and read back what the database actually
// holds. The ceiling itself is not in question; what it leaves behind is.
//
// The upload route is the plainest of them: a multipart part carries whatever
// file name the browser had, and nothing between the form and the INSERT looks
// at its length, so this is the shortest path from a person's file name to an
// attachments row.
func TestALongAttachmentFilenameIsStoredWithoutAContextNotice(t *testing.T) {
	srv := newServerUnderTest(t)
	document := markdownDocumentOwnedByAdmin(t, srv, "첨부 이름 길이 확인")

	// Four names around the ceiling. Only the first is over it; the other
	// three are there to pin down that shortening the long one does not start
	// rewriting the ordinary ones, which is the way a cut like this usually
	// goes wrong.
	for _, testCase := range []struct {
		name   string
		upload string
		want   string
	}{
		{"241 runes is cut to the first 240", strings.Repeat("긴", 241), strings.Repeat("긴", 240)},
		{"exactly 240 runes is untouched", strings.Repeat("가", 240), strings.Repeat("가", 240)},
		{"240 runes ending in an extension is untouched", strings.Repeat("가", 236) + ".txt", strings.Repeat("가", 236) + ".txt"},
		{"an ordinary name is untouched", "회의록.txt", "회의록.txt"},
		{"a single rune is untouched", "가", "가"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			attachment := uploadAttachment(t, srv, document, testCase.upload, []byte("내용"))

			var stored string
			if err := srv.db.QueryRow(t.Context(), `SELECT name FROM attachments WHERE id=$1`, attachment).Scan(&stored); err != nil {
				t.Fatal(err)
			}
			if stored != testCase.want {
				t.Errorf("stored attachment name = %q, want %q", stored, testCase.want)
			}
			if strings.Contains(stored, contextNoticeFragment) {
				t.Errorf("the stored name carries the AI context notice: %q", stored)
			}
			if strings.ContainsAny(stored, "\r\n") {
				t.Errorf("the stored name carries a line break: %q", stored)
			}

			// The name does not stay in the database. It goes out again in the
			// download's Content-Disposition, so the round trip is read with
			// the production parser — the same mime.ParseMediaType another
			// muni uses on a handoff — and has to give back what was stored.
			resp, err := srv.admin.Get(srv.URL + "/api/v1/attachments/" + attachment.String())
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("download = %d %s", resp.StatusCode, body)
			}
			disposition := resp.Header.Get("Content-Disposition")
			if got := filenameFrom(t, disposition); got != stored {
				t.Errorf("filename read back = %q, want the stored name %q (from %q)", got, stored, disposition)
			}
		})
	}
}

// The document title takes the same ceiling by the same helper, and it is the
// more visible of the two: the title is what the document list shows, what the
// search index is built over, and what the workspace archive writes into its
// 목록.md. This drives the import route the way the import dialog does, with
// the title typed into the form rather than read out of the file.
func TestALongImportedTitleIsStoredWithoutAContextNotice(t *testing.T) {
	srv := newServerUnderTest(t)
	title := strings.Repeat("제", 241)
	want := strings.Repeat("제", 240)

	imported := importFile(t, srv, adminWorkspace(t, srv), "보고서.md", title, []byte("# 본문 제목\n\n내용입니다.\n"))
	documentID := uuid.MustParse(imported["id"].(string))

	var stored string
	if err := srv.db.QueryRow(t.Context(), `SELECT title FROM documents WHERE id=$1`, documentID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != want {
		t.Errorf("stored title = %q, want %q", stored, want)
	}
	if strings.Contains(stored, contextNoticeFragment) {
		t.Errorf("the stored title carries the AI context notice: %q", stored)
	}
	if strings.ContainsAny(stored, "\r\n") {
		t.Errorf("the stored title carries a line break: %q", stored)
	}
}

// A title at or under the ceiling is not a cut at all, and has to come back
// from the import byte for byte — including the case the ceiling is measured
// in, where 240 runes of Korean are 720 bytes.
func TestAnImportedTitleWithinTheCeilingIsStoredUnchanged(t *testing.T) {
	srv := newServerUnderTest(t)
	workspace := adminWorkspace(t, srv)

	for _, testCase := range []struct {
		name  string
		title string
	}{
		{"exactly 240 runes", strings.Repeat("제", 240)},
		{"a single rune", "제"},
		{"an ordinary title with an extension in it", "2026년 계획 v2.final"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			imported := importFile(t, srv, workspace, "보고서.md", testCase.title, []byte("내용입니다.\n"))
			documentID := uuid.MustParse(imported["id"].(string))

			var stored string
			if err := srv.db.QueryRow(t.Context(), `SELECT title FROM documents WHERE id=$1`, documentID).Scan(&stored); err != nil {
				t.Fatal(err)
			}
			if stored != testCase.title {
				t.Errorf("stored title = %q, want it unchanged at %q", stored, testCase.title)
			}
		})
	}
}
