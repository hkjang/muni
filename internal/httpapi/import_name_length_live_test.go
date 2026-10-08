package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/hwpx"
	"github.com/hkjang/muni/internal/richdoc"
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

// HWPX carries long furniture through the real writer and reader. DOCX would
// hide this regression because its reader already cuts furniture to 200 runes.
func pageFurnitureCases() []struct {
	name, header, footer, wantHeader, wantFooter string
	landscape                                    bool
} {
	return []struct {
		name, header, footer, wantHeader, wantFooter string
		landscape                                    bool
	}{
		{"201 runes", strings.Repeat("머", 200) + "끝", strings.Repeat("꼬", 200) + "끝", strings.Repeat("머", 200), strings.Repeat("꼬", 200), true},
		{"empty", "", "", "", "", false},
		{"short", "회의록 대외비", "기획 부서", "회의록 대외비", "기획 부서", true},
		{"exactly 200 runes", strings.Repeat("가", 200), strings.Repeat("나", 200), strings.Repeat("가", 200), strings.Repeat("나", 200), false},
		{"space at the cut", strings.Repeat("머", 199) + " 끝", strings.Repeat("꼬", 199) + " 끝", strings.Repeat("머", 199), strings.Repeat("꼬", 199), true},
	}
}

func hwpxWithPageFurniture(t *testing.T, header, footer string, landscape bool) []byte {
	t.Helper()
	file, err := hwpx.Build(richdoc.Doc(richdoc.Paragraph(richdoc.Text("가져온 본문입니다."))), hwpx.Options{
		Title: "파일 안 제목", Header: header, Footer: footer, Landscape: landscape,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, meta, err := hwpx.Parse(file)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Header != header || meta.Footer != footer || meta.Landscape != landscape {
		t.Fatalf("HWPX writer/reader changed furniture before HTTP: %+v", meta)
	}
	return file
}

func checkPageFurniture(t *testing.T, where, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %q, want %q", where, got, want)
	}
	if strings.Contains(got, contextNoticeFragment) || strings.ContainsAny(got, "\r\n") {
		t.Errorf("%s carries an AI context notice or line break: %q", where, got)
	}
}

func cleanupFurnitureDocument(t *testing.T, srv *serverUnderTest, id string) {
	t.Helper()
	t.Cleanup(func() {
		if _, err := srv.db.Exec(context.Background(), `DELETE FROM documents WHERE id=$1`, uuid.MustParse(id)); err != nil {
			t.Errorf("clean up imported document: %v", err)
		}
	})
}

func TestImportedPageFurnitureIsStoredWithoutAContextNotice(t *testing.T) {
	srv := newServerUnderTest(t)
	workspace := adminWorkspace(t, srv)
	for _, tc := range pageFurnitureCases() {
		t.Run(tc.name, func(t *testing.T) {
			file := hwpxWithPageFurniture(t, tc.header, tc.footer, tc.landscape)
			imported := importFile(t, srv, workspace, "보고서.hwpx", "가져온 제목", file)
			id := imported["id"].(string)
			cleanupFurnitureDocument(t, srv, id)

			var header, footer, title, body, orientation string
			if err := srv.db.QueryRow(t.Context(), `SELECT page_header,page_footer,title,content_text,page_orientation FROM documents WHERE id=$1`, uuid.MustParse(id)).Scan(&header, &footer, &title, &body, &orientation); err != nil {
				t.Fatal(err)
			}
			checkPageFurniture(t, "stored header", header, tc.wantHeader)
			checkPageFurniture(t, "stored footer", footer, tc.wantFooter)
			checkPageFurniture(t, "response pageHeader", imported["pageHeader"].(string), tc.wantHeader)
			checkPageFurniture(t, "response pageFooter", imported["pageFooter"].(string), tc.wantFooter)
			wantOrientation := "PORTRAIT"
			if tc.landscape {
				wantOrientation = "LANDSCAPE"
			}
			if title != "가져온 제목" || imported["title"] != title || orientation != wantOrientation || imported["pageOrientation"] != wantOrientation {
				t.Errorf("title/orientation changed: stored %q/%q, response %v/%v", title, orientation, imported["title"], imported["pageOrientation"])
			}
			content, err := json.Marshal(imported["content"])
			if err != nil {
				t.Fatal(err)
			}
			if body != "가져온 본문입니다." || extractDocumentText(content) != body {
				t.Errorf("imported body changed: stored %q, response %s", body, content)
			}
		})
	}
}

func TestInsertedPageFurnitureIsReturnedWithoutAContextNotice(t *testing.T) {
	srv := newServerUnderTest(t)
	created := importFile(t, srv, adminWorkspace(t, srv), "기존.md", "기존 제목", []byte("기존 본문입니다."))
	id := created["id"].(string)
	cleanupFurnitureDocument(t, srv, id)
	if _, err := srv.db.Exec(t.Context(), `UPDATE documents SET page_header='기존 머리말',page_footer='기존 꼬리말' WHERE id=$1`, uuid.MustParse(id)); err != nil {
		t.Fatal(err)
	}
	// Snapshot only this document, including both body representations and its
	// revision. The editor must decide when to save the returned import data.
	snapshot := func(t *testing.T) string {
		t.Helper()
		var state string
		if err := srv.db.QueryRow(t.Context(), `SELECT jsonb_build_array(title,page_header,page_footer,content_json,content_text,revision_no,page_orientation)::text FROM documents WHERE id=$1`, uuid.MustParse(id)).Scan(&state); err != nil {
			t.Fatal(err)
		}
		return state
	}
	before := snapshot(t)
	for _, tc := range pageFurnitureCases() {
		t.Run(tc.name, func(t *testing.T) {
			file := hwpxWithPageFurniture(t, tc.header, tc.footer, tc.landscape)
			status, data := importIntoDocument(t, srv, id, "보고서.hwpx", file)
			if status != http.StatusOK {
				t.Fatalf("import into document = %d %v", status, data)
			}
			checkPageFurniture(t, "response header", data["header"].(string), tc.wantHeader)
			checkPageFurniture(t, "response footer", data["footer"].(string), tc.wantFooter)
			if data["title"] != "보고서" || data["landscape"] != tc.landscape || data["format"] != "hwpx" {
				t.Errorf("title/orientation/format changed: %v", data)
			}
			content, err := json.Marshal(data["content"])
			if err != nil {
				t.Fatal(err)
			}
			if extractDocumentText(content) != "가져온 본문입니다." {
				t.Errorf("returned body changed: %s", content)
			}
			if after := snapshot(t); after != before {
				t.Errorf("insertion changed the target document: before %s, after %s", before, after)
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
