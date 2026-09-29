package httpapi

import (
	"context"
	"io"
	"mime"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// The unit tests next door build the header the way the routes build it. These
// read the bytes the routes really send, through the real server, because the
// escaper is only half of a Content-Disposition — the other half is the format
// string around it, and there is a different one in each route.
//
// awkwardTitle is not exotic: a Korean report title with a bracketed marker, a
// percentage and the punctuation a person types. Every character of it outside
// attr-char has to come back through a parser unchanged, and the semicolon is
// the one that used to end the parameter early and truncate the name with no
// error to show for it.
const awkwardTitle = `[대외비] 2026년 계획(초안); 달성률 100% "검토용"`

// filenameFrom reads the name a client would save by. internal/handoff's
// filenameOf does this to a header muni sent, so this is not a test-only view
// of the value — it is the production reader.
func filenameFrom(t *testing.T, disposition string) string {
	t.Helper()
	_, params, err := mime.ParseMediaType(disposition)
	if err != nil {
		t.Errorf("mime.ParseMediaType(%q) = %v", disposition, err)
		return ""
	}
	return params["filename"]
}

func TestADownloadNamesTheFileSoAParserReadsTheTitleBack(t *testing.T) {
	srv := newServerUnderTest(t)
	document := documentTitledOverTheAPI(t, srv, adminWorkspace(t, srv), awkwardTitle)

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
	if want := awkwardTitle + ".md"; filenameFrom(t, disposition) != want {
		t.Errorf("filename read back = %q, want %q (from %q)", filenameFrom(t, disposition), want, disposition)
	}
	// The ASCII fallback stays what it was, for whatever cannot read the other
	// parameter at all.
	if !strings.Contains(disposition, `filename="document.md"`) {
		t.Errorf("the plain filename fallback is gone: %q", disposition)
	}
}

// An attachment's name is the uploader's own file name, which goes into the
// same parameter by the same route shape — and unlike a document title it is
// never cut, so whatever the browser sent is what has to survive.
func TestAnAttachmentDownloadNamesTheFileSoAParserReadsItBack(t *testing.T) {
	srv := newServerUnderTest(t)
	document := markdownDocumentOwnedByAdmin(t, srv, "첨부 이름 확인")
	name := "회의 자료(최종); 100%.txt"
	attachment := uploadAttachment(t, srv, document, name, []byte("내용"))

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
	if got := filenameFrom(t, disposition); got != name {
		t.Errorf("filename read back = %q, want %q (from %q)", got, name, disposition)
	}
}

// The other half of an attachment's Content-Disposition is the ASCII fallback,
// and that half is all a client gets when it cannot read `filename*` at all —
// an old browser, a plain HTTP client, one of the closed-network tools this is
// deployed next to. Four of the five download routes put an extension in theirs
// (`document.md`, `presentation.pdf`, `workspace-20260929.zip`, `document.xlsx`);
// this one said `attachment` flat, so a spreadsheet saved by such a client
// arrived as an extensionless file that nothing on the machine would open.
//
// The extension cannot simply be pasted in, which is why this is a table rather
// than one case. An attachment's name is the uploader's own file name cut to
// length and nothing else — no character is replaced on the way in — so it may
// hold the quote that ends a quoted-string early, or runes that have no business
// in an ASCII fallback. For those the fallback has to stay the bare word it was,
// and the header as a whole has to keep parsing, because losing `filename*`
// would cost more than the missing extension ever did.
func TestAnAttachmentDownloadPutsTheExtensionInTheAsciiFallback(t *testing.T) {
	srv := newServerUnderTest(t)
	document := markdownDocumentOwnedByAdmin(t, srv, "fallback 확장자 확인")

	for _, testCase := range []struct {
		name     string
		upload   string
		fallback string
	}{
		{"an ordinary spreadsheet keeps its extension", "회의록.xlsx", `filename="attachment.xlsx"`},
		{"a name with no extension stays the bare word", "회의록", `filename="attachment"`},
		{"a quote inside the extension is refused", `보고서.p"g`, `filename="attachment"`},
		{"a non-ASCII extension is refused", "자료.한글확장자", `filename="attachment"`},
		{"an implausibly long extension is refused", "자료.abcdefghijkl", `filename="attachment"`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			attachment := uploadAttachment(t, srv, document, testCase.upload, []byte("내용"))

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
			if !strings.Contains(disposition, testCase.fallback) {
				t.Errorf("the ASCII fallback in %q does not contain %q", disposition, testCase.fallback)
			}
			// Whatever the fallback ends up saying, the parameter that carries
			// the real name is untouched and the header still parses — read back
			// with the production parser, as everything else here is.
			if got := filenameFrom(t, disposition); got != testCase.upload {
				t.Errorf("filename read back = %q, want %q (from %q)", got, testCase.upload, disposition)
			}
		})
	}
}

// An attachment does not have to be uploaded by hand to get a name. A picture
// carried inside an imported file is stored as one too, and its name is the
// description the file gave it — a value from outside, of any length. This
// walks the whole way a description travels: the import route, the attachments
// row it is written to, and the header the download hands a client, read back
// with the same parser as everything else here.
func TestAnImportedImageNamesItselfWithoutAContextNotice(t *testing.T) {
	srv := newServerUnderTest(t)
	pixel := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	description := strings.Repeat("긴", 201)
	want := strings.Repeat("긴", 200) + ".png"

	imported := importFile(t, srv, adminWorkspace(t, srv), "그림.html", "",
		[]byte(`<html><body><p><img src="`+pixel+`" alt="`+description+`"></p></body></html>`))
	documentID := uuid.MustParse(imported["id"].(string))

	var attachmentID uuid.UUID
	var stored string
	if err := srv.db.QueryRow(t.Context(), `SELECT id,name FROM attachments WHERE document_id=$1`, documentID).Scan(&attachmentID, &stored); err != nil {
		t.Fatal(err)
	}
	if stored != want {
		t.Errorf("stored attachment name = %q, want %q", stored, want)
	}

	resp, err := srv.admin.Get(srv.URL + "/api/v1/attachments/" + attachmentID.String())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download = %d %s", resp.StatusCode, body)
	}
	disposition := resp.Header.Get("Content-Disposition")
	if got := filenameFrom(t, disposition); got != want {
		t.Errorf("filename read back = %q, want %q (from %q)", got, want, disposition)
	}
}

// The workspace archive names itself after the workspace, and a workspace name
// is checked for length and nothing else — so the quote in a name like
// `연구 "특별"` went straight into the quoted-string and ended it early, and a
// Korean name was raw UTF-8 where only ASCII belongs. This is the same route a
// person clicks, and the name is read back with the same parser as the rest.
func TestAWorkspaceArchiveNamesItselfSoAParserReadsTheNameBack(t *testing.T) {
	srv := newServerUnderTest(t)
	workspaceID := workspaceNamed(t, srv, awkwardTitle, "quote-zip")

	resp, err := srv.admin.Get(srv.URL + "/api/v1/workspaces/" + workspaceID.String() + "/export.zip?format=md")
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
	day := time.Now().Format("20060102")
	if want := safeFilename(awkwardTitle) + "-" + day + ".zip"; filenameFrom(t, disposition) != want {
		t.Errorf("filename read back = %q, want %q (from %q)", filenameFrom(t, disposition), want, disposition)
	}
	// The fallback carries only what ASCII can: the day the archive was made.
	if want := `filename="workspace-` + day + `.zip"`; !strings.Contains(disposition, want) {
		t.Errorf("the plain filename fallback is %q, want it to contain %q", disposition, want)
	}
}

// workspaceNamed makes a workspace over the API, the way the admin screen does.
func workspaceNamed(t *testing.T, srv *serverUnderTest, name, slug string) uuid.UUID {
	t.Helper()
	status, data := postJSON(t, srv.admin, srv.URL+"/api/v1/workspaces", map[string]any{"name": name, "slug": slug})
	if status != http.StatusCreated {
		t.Fatalf("create workspace = %d %v", status, data)
	}
	id, err := uuid.Parse(data["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	// liveServer only clears the accounts these tests make, so a workspace left
	// behind would collide on its slug the next time this runs.
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = srv.db.Exec(ctx, `DELETE FROM workspace_members WHERE workspace_id=$1`, id)
		_, _ = srv.db.Exec(ctx, `DELETE FROM workspaces WHERE id=$1`, id)
	})
	return id
}

// The handoff route is the one with a real program on the other end: another
// muni fetches the claim and takes the name from this header alone. It had its
// own escaper, closer to right than the download routes' but still leaving `=`
// and `:` standing, either of which a parser refuses.
func TestAHandoffClaimNamesTheFileSoTheTakerReadsTheTitleBack(t *testing.T) {
	srv := newServerUnderTest(t)
	document := markdownDocumentOwnedByAdmin(t, srv, awkwardTitle)

	status, claim := issueClaim(t, srv.admin, srv.URL, document, "markdown")
	if status != http.StatusCreated {
		t.Fatalf("issue = %d %+v", status, claim)
	}
	if want := awkwardTitle + ".md"; claim.Filename != want {
		t.Fatalf("claim filename = %q, want %q", claim.Filename, want)
	}

	// The taker has no session here; the claim is the whole credential.
	resp, err := (&http.Client{}).Get(srv.URL + "/api/v1/handoff/claims/" + claim.Claim)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("redeem = %d %s", resp.StatusCode, body)
	}

	disposition := resp.Header.Get("Content-Disposition")
	if got := filenameFrom(t, disposition); got != claim.Filename {
		t.Errorf("filename read back = %q, want the name the claim promised %q (from %q)", got, claim.Filename, disposition)
	}
}
