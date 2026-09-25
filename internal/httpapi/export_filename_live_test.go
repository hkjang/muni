package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"mime"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// A title long enough to be cut reaches the file name through five callers of
// safeFilename, and a unit test only sees the helper. These two read the bytes
// the routes actually send: the download header a browser saves by, and the
// entry names an unpacker writes to disk.

// documentTitledOverTheAPI creates a document the way the editor does — over
// the API, so the 240-rune title limit the endpoint enforces is the one in play
// — and takes it back out afterwards, because the administrator's workspace
// outlives the test.
func documentTitledOverTheAPI(t *testing.T, srv *serverUnderTest, workspaceID uuid.UUID, title string) uuid.UUID {
	t.Helper()
	status, data := postJSON(t, srv.admin, srv.URL+"/api/v1/documents", map[string]any{
		"workspaceId": workspaceID.String(),
		"title":       title,
	})
	if status != http.StatusOK {
		t.Fatalf("create document = %d %v", status, data)
	}
	id, err := uuid.Parse(data["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = srv.db.Exec(context.Background(), `DELETE FROM documents WHERE id=$1`, id)
	})
	return id
}

// overLongTitle is 120 runes — past the 100 the file name allows, well under
// the 240 the API allows, and the kind of thing a report title really is.
const overLongTitle = "2026년 3분기 정보화사업 추진 현황 및 차년도 예산 편성 방향에 관한 부서 합동 검토 보고서 초안 — 관계 부서 의견 조회용 배포본 제이차 개정 대외비 문서로 외부 반출을 금합니다 사내 한정 자료이며 재배포 금지"

func TestALongTitleDoesNotPutAContextNoticeInTheDownloadName(t *testing.T) {
	srv := newServerUnderTest(t)
	if got := len([]rune(overLongTitle)); got != 120 {
		t.Fatalf("the fixture title is %d runes, not the 120 this test is about", got)
	}
	document := documentTitledOverTheAPI(t, srv, adminWorkspace(t, srv), overLongTitle)

	resp, err := srv.admin.Get(srv.URL + "/api/v1/documents/" + document.String() + "/export/md")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("export = %d", resp.StatusCode)
	}

	disposition := resp.Header.Get("Content-Disposition")

	// The name the browser saves by is the filename* parameter, so the check is
	// made through a header parser rather than against the production escaper,
	// which would only agree with itself.
	if _, params, err := mime.ParseMediaType(disposition); err != nil {
		t.Errorf("mime.ParseMediaType(%q) = %v", disposition, err)
	} else if want := string([]rune(overLongTitle)[:100]) + ".md"; params["filename"] != want {
		t.Errorf("filename read back = %q,\nwant %q (from %q)", params["filename"], want, disposition)
	}
	if strings.Contains(disposition, "생략됨") || strings.Contains(disposition, "%EC%83%9D%EB%9E%B5%EB%90%A8") {
		t.Errorf("the header carries the AI context notice: %q", disposition)
	}
	// net/http rewrites a newline in a header value as a space, so the notice
	// arrived as an unescaped space and a bracket inside the parameter rather
	// than as an injected header. Either way the parameter stops being one.
	if strings.ContainsAny(disposition, "\r\n[]") {
		t.Errorf("the header carries a line break or a bracket: %q", disposition)
	}
}

func TestALongTitleDoesNotPutAContextNoticeInTheArchiveEntryName(t *testing.T) {
	srv := newServerUnderTest(t)
	workspaceID := adminWorkspace(t, srv)
	folder := folderNamed(t, srv, workspaceID, "긴 제목 확인", nil)
	documentInFolder(t, srv, workspaceID, &folder, overLongTitle, false)

	resp, err := srv.admin.Get(srv.URL + "/api/v1/workspaces/" + workspaceID.String() + "/export.zip?format=md")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("export = %d: %s", resp.StatusCode, raw)
	}
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}

	var manifest string
	entry := ""
	for _, file := range archive.File {
		if file.Name == "목록.md" {
			body, err := file.Open()
			if err != nil {
				t.Fatal(err)
			}
			text, _ := io.ReadAll(body)
			body.Close()
			manifest = string(text)
			continue
		}
		if strings.HasPrefix(file.Name, "긴 제목 확인/") {
			entry = file.Name
		}
		if strings.Contains(file.Name, "생략됨") {
			t.Errorf("entry %q carries the AI context notice", file.Name)
		}
		if strings.ContainsAny(file.Name, "\r\n") {
			t.Errorf("entry %q carries a line break", file.Name)
		}
	}

	want := "긴 제목 확인/" + string([]rune(overLongTitle)[:100]) + ".md"
	if entry != want {
		t.Errorf("entry = %q, want %q", entry, want)
	}
	// 목록.md is the index of the archive: the name it prints has to be the
	// name the reader will find, or the index points at nothing.
	if entry != "" && !strings.Contains(manifest, "- "+entry+" —") {
		t.Errorf("목록.md does not list %q; it says:\n%s", entry, manifest)
	}
}
