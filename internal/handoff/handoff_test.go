package handoff

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func peerList(origins ...string) Config {
	config := Config{}
	for _, origin := range origins {
		config.Peers = append(config.Peers, Peer{Origin: origin, Name: "peer", Receives: []string{FormatMarkdown}})
	}
	return config
}

func TestAllowedMatchesTheWholeOriginOnly(t *testing.T) {
	config := peerList("https://UMM.intra/")
	for _, source := range []string{"https://umm.intra", "https://umm.intra/", "HTTPS://umm.intra"} {
		if _, ok := config.Allowed(source); !ok {
			t.Errorf("%q should be allowed", source)
		}
	}
	for _, source := range []string{
		"http://umm.intra",           // scheme differs
		"https://umm.intra:8443",     // port differs
		"https://umm.intra/handoff",  // a path is not an origin
		"https://umm.intra?x=1",      // nor a query
		"https://user@umm.intra",     // nor userinfo
		"https://umm.intra.evil.com", // a longer host
		"file:///etc/passwd",
		"",
	} {
		if _, ok := config.Allowed(source); ok {
			t.Errorf("%q should be refused", source)
		}
	}
	if _, ok := (Config{}).Allowed("https://umm.intra"); ok {
		t.Error("an empty list allows nothing")
	}
}

func TestValidateRefusesWhatCannotBeMeant(t *testing.T) {
	for name, config := range map[string]Config{
		"not an origin":  {Peers: []Peer{{Origin: "umm.intra"}}},
		"not http":       {Peers: []Peer{{Origin: "ftp://umm.intra"}}},
		"twice":          {Peers: []Peer{{Origin: "https://umm.intra"}, {Origin: "https://UMM.intra/"}}},
		"unknown format": {Peers: []Peer{{Origin: "https://umm.intra", Receives: []string{"pdf"}}}},
		"long name":      {Peers: []Peer{{Origin: "https://umm.intra", Name: strings.Repeat("가", 61)}}},
	} {
		if config.Validate() == nil {
			t.Errorf("%s: should be refused", name)
		}
	}
	ok := Config{Peers: []Peer{{Origin: "https://ptium.intra/", Name: " ptium ", Receives: []string{"Markdown", "docx", "docx"}}}}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	normalized := ok.Normalize().Peers[0]
	if normalized.Origin != "https://ptium.intra" || normalized.Name != "ptium" || len(normalized.Receives) != 2 {
		t.Errorf("normalized = %+v", normalized)
	}
}

func TestTargetsShowOnlyWhatBothSidesCanDo(t *testing.T) {
	config := Config{Peers: []Peer{
		{Origin: "https://ptium.intra", Name: "ptium", Receives: []string{"markdown", "docx", "csv"}},
		{Origin: "https://kanpic.intra", Name: "kanpic", Receives: []string{"csv", "xlsx"}},
		{Origin: "https://umm.intra", Name: "umm", Receives: nil},
		{Origin: "https://weekly.intra", Receives: []string{"docx"}},
	}}
	targets := config.Targets(true)
	if len(targets) != 2 {
		t.Fatalf("targets = %+v", targets)
	}
	if targets[0].Name != "ptium" || strings.Join(targets[0].Formats, ",") != "markdown,docx" {
		t.Errorf("ptium = %+v", targets[0])
	}
	if targets[1].Name != "weekly.intra" || strings.Join(targets[1].Formats, ",") != "docx" {
		t.Errorf("weekly = %+v", targets[1])
	}
	// With docx export switched off the docx-only peer disappears too.
	withoutDOCX := config.Targets(false)
	if len(withoutDOCX) != 1 || strings.Join(withoutDOCX[0].Formats, ",") != "markdown" {
		t.Errorf("without docx = %+v", withoutDOCX)
	}
	if len((Config{}).Targets(true)) != 0 {
		t.Error("an empty list has no targets")
	}
}

func TestFetchNeverAsksAnUnlistedSource(t *testing.T) {
	var calls atomic.Int32
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, _ = w.Write([]byte("# hi"))
	}))
	defer source.Close()
	client := NewClient(http.DefaultTransport)
	_, err := Fetch(context.Background(), client, Config{}, source.URL, "abcdefghijklmnopqrstuvwxyz")
	if !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("err = %v", err)
	}
	if calls.Load() != 0 {
		t.Fatal("an unlisted source was asked")
	}
	// A malformed claim is refused before any request too.
	_, err = Fetch(context.Background(), client, peerList(source.URL), source.URL, "../../etc")
	if !errors.Is(err, ErrBadClaim) || calls.Load() != 0 {
		t.Fatalf("err = %v, calls = %d", err, calls.Load())
	}
	document, err := Fetch(context.Background(), client, peerList(source.URL), source.URL, "abcdefghijklmnopqrstuvwxyz")
	if err != nil || string(document.Body) != "# hi" || document.Format != FormatMarkdown {
		t.Fatalf("document = %+v, err = %v", document, err)
	}
}

func TestFetchDoesNotFollowRedirects(t *testing.T) {
	var followed atomic.Int32
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/elsewhere" {
			followed.Add(1)
		}
		http.Redirect(w, r, "/elsewhere", http.StatusFound)
	}))
	defer source.Close()
	_, err := Fetch(context.Background(), NewClient(http.DefaultTransport), peerList(source.URL), source.URL, "abcdefghijklmnopqrstuvwxyz")
	if !errors.Is(err, ErrRedirect) {
		t.Fatalf("err = %v", err)
	}
	if followed.Load() != 0 {
		t.Fatal("the redirect was followed")
	}
}

func TestFetchCutsOffAnOversizedBody(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		// No Content-Length: the size is only known by reading.
		w.(http.Flusher).Flush()
		chunk := strings.Repeat("a", 1<<20)
		for i := 0; i <= MaxBodyBytes>>20; i++ {
			if _, err := w.Write([]byte(chunk)); err != nil {
				return
			}
		}
	}))
	defer source.Close()
	_, err := Fetch(context.Background(), NewClient(http.DefaultTransport), peerList(source.URL), source.URL, "abcdefghijklmnopqrstuvwxyz")
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v", err)
	}
	declared := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		w.Header().Set("Content-Length", "99999999")
		_, _ = w.Write([]byte("x"))
	}))
	defer declared.Close()
	_, err = Fetch(context.Background(), NewClient(http.DefaultTransport), peerList(declared.URL), declared.URL, "abcdefghijklmnopqrstuvwxyz")
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("declared: err = %v", err)
	}
}

func TestFetchRefusesAFormatItCannotRead(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.presentationml.presentation")
		_, _ = w.Write([]byte("PK"))
	}))
	defer source.Close()
	_, err := Fetch(context.Background(), NewClient(http.DefaultTransport), peerList(source.URL), source.URL, "abcdefghijklmnopqrstuvwxyz")
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v", err)
	}
	gone := httptest.NewServer(http.NotFoundHandler())
	defer gone.Close()
	_, err = Fetch(context.Background(), NewClient(http.DefaultTransport), peerList(gone.URL), gone.URL, "abcdefghijklmnopqrstuvwxyz")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("404: err = %v", err)
	}
}

func TestFilenameOfReadsBothForms(t *testing.T) {
	for header, want := range map[string]string{
		`attachment; filename="report.md"`:                                            "report.md",
		`attachment; filename*=UTF-8''2026%EB%85%84%20%EA%B0%9C%ED%8E%B8%EC%95%88.md`: "2026년 개편안.md",
		`attachment; filename="../../x.md"`:                                           "x.md",
		`attachment`:                                                                  "",
		``:                                                                            "",
	} {
		if got := filenameOf(header); got != want {
			t.Errorf("%q: got %q, want %q", header, got, want)
		}
	}
}
