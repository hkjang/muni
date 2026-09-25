package httpapi

import (
	"io"
	"mime"
	"net/http"
	"strings"
	"testing"
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
