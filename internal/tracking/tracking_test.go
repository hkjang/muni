package tracking

import (
	"strings"
	"testing"
	"time"
)

func momento() Config {
	return Config{Enabled: true, Provider: ProviderMomento, MomentoURL: "https://momento.corp.example/", MomentoSiteID: "muni", MomentoProxy: true}
}

func TestDisabledCarriesNothing(t *testing.T) {
	for _, config := range []Config{
		{},
		{Provider: ProviderMomento, MomentoURL: "https://momento.corp.example", MomentoSiteID: "muni"},
		{Enabled: true, Provider: ProviderNone},
		{Enabled: true, Provider: ProviderMomento},
	} {
		if config.Active("/") || config.Snippet("n") != "" {
			t.Fatalf("%+v should carry no snippet", config)
		}
		if scripts, connects, images := config.PolicySources(); len(scripts)+len(connects)+len(images) != 0 {
			t.Fatalf("%+v should add no policy sources", config)
		}
	}
}

func TestMomentoThroughTheProxyNamesNoOutsideOrigin(t *testing.T) {
	config := momento()
	snippet := config.Snippet("n0nce")
	for _, want := range []string{`src="/momento/tracker.js"`, `data-endpoint="/momento"`, `data-site-id="muni"`, `nonce="n0nce"`} {
		if !strings.Contains(snippet, want) {
			t.Fatalf("snippet %q lacks %s", snippet, want)
		}
	}
	if strings.Contains(snippet, "momento.corp.example") {
		t.Fatalf("the proxied snippet must not name the collector: %q", snippet)
	}
	if scripts, connects, images := config.PolicySources(); len(scripts)+len(connects)+len(images) != 0 {
		t.Fatalf("the proxied collector needs no policy sources, got %v %v %v", scripts, connects, images)
	}
	if !config.ProxyActive() {
		t.Fatal("proxy should be active")
	}
}

func TestMomentoDirectNamesTheCollector(t *testing.T) {
	config := momento()
	config.MomentoProxy = false
	snippet := config.Snippet("n0nce")
	if !strings.Contains(snippet, `src="https://momento.corp.example/tracker.js"`) || strings.Contains(snippet, "data-endpoint") {
		t.Fatalf("snippet %q", snippet)
	}
	scripts, connects, images := config.PolicySources()
	for _, group := range [][]string{scripts, connects, images} {
		if len(group) != 1 || group[0] != "https://momento.corp.example" {
			t.Fatalf("sources %v %v %v", scripts, connects, images)
		}
	}
	if config.ProxyActive() {
		t.Fatal("proxy should be off")
	}
}

func TestAdminPagesAreLeftOutUnlessAsked(t *testing.T) {
	config := momento()
	for _, path := range []string{"/admin", "/admin/settings", "/admin/users/1"} {
		if config.Active(path) {
			t.Fatalf("%s should not carry the snippet by default", path)
		}
	}
	if !config.Active("/") || !config.Active("/docs/1") || !config.Active("/administration") {
		t.Fatal("ordinary pages should carry the snippet")
	}
	config.IncludeAdmin = true
	if !config.Active("/admin/settings") {
		t.Fatal("include_admin should add the console")
	}
}

func TestNonceGoesOnEveryScriptTag(t *testing.T) {
	config := Config{Enabled: true, Provider: ProviderCustom, CustomSnippet: `<SCRIPT src="https://t.example/a.js"></SCRIPT>
<script nonce="theirs">x()</script>
<script>y()</script>`}
	snippet := config.Snippet("abc")
	if strings.Count(snippet, "<script") != 3 && strings.Count(strings.ToLower(snippet), "<script") != 3 {
		t.Fatalf("snippet %q", snippet)
	}
	if strings.Count(snippet, `nonce="abc"`) != 2 || !strings.Contains(snippet, `nonce="theirs"`) {
		t.Fatalf("every tag without a nonce gets ours, one that has its own keeps it: %q", snippet)
	}
	if WithNonce("<p>no script</p>", "abc") != "<p>no script</p>" {
		t.Fatal("markup without scripts is untouched")
	}
}

func TestOriginsAreReadOutOfASnippet(t *testing.T) {
	snippet := `<script src="https://cdn.example/t.js"></script>
<script>window.__t={endpoint:"https://collect.example/v1/events",pixel:'HTTP://Pixel.Example/p.gif?id=1'};
fetch("https://cdn.example/x");</script><!-- data:image/png;base64,AAA chrome-extension://abc/x.js -->`
	origins := SnippetOrigins(snippet)
	want := []string{"https://cdn.example", "https://collect.example", "http://pixel.example"}
	if strings.Join(origins, " ") != strings.Join(want, " ") {
		t.Fatalf("origins %v, want %v", origins, want)
	}
	config := Config{Enabled: true, Provider: ProviderCustom, CustomSnippet: snippet, AllowedHosts: "https://extra.example/, https://cdn.example"}
	scripts, connects, images := config.PolicySources()
	if len(scripts) != 5 || scripts[3] != "https://extra.example" || len(connects) != 5 || len(images) != 5 {
		t.Fatalf("sources %v %v %v", scripts, connects, images)
	}
}

func TestKnownProvidersNeedNoPolicyKnowledge(t *testing.T) {
	ga := Config{Enabled: true, Provider: ProviderGA4, MeasurementID: "G-1"}
	scripts, connects, images := ga.PolicySources()
	if scripts[0] != "https://www.googletagmanager.com" || len(connects) != 3 || len(images) != 2 {
		t.Fatalf("ga4 sources %v %v %v", scripts, connects, images)
	}
	if !strings.Contains(ga.Snippet("n"), `gtag('config','G-1')`) {
		t.Fatal("ga4 snippet")
	}
	matomo := Config{Enabled: true, Provider: ProviderMatomo, MatomoURL: "https://matomo.corp.example/", MatomoSiteID: "3"}
	scripts, _, _ = matomo.PolicySources()
	if len(scripts) != 1 || scripts[0] != "https://matomo.corp.example" {
		t.Fatalf("matomo sources %v", scripts)
	}
	if !strings.Contains(matomo.Snippet("n"), `u="https://matomo.corp.example/"`) {
		t.Fatalf("matomo snippet %q", matomo.Snippet("n"))
	}
}

func TestValidate(t *testing.T) {
	big := Config{Provider: ProviderCustom, CustomSnippet: strings.Repeat("x", MaxSnippetBytes+1)}
	if err := big.Validate(); err == nil || !strings.Contains(err.Error(), "8192") {
		t.Fatalf("an oversized snippet is refused even while disabled: %v", err)
	}
	if err := (Config{Enabled: true, Provider: "pixel"}).Validate(); err == nil {
		t.Fatal("unknown provider")
	}
	if err := (Config{Enabled: true, Provider: ProviderMomento, MomentoURL: "momento.corp.example", MomentoSiteID: "x"}).Validate(); err == nil {
		t.Fatal("a collector address without a scheme is not an address")
	}
	if err := (Config{Enabled: true, Provider: ProviderMomento, MomentoURL: "https://momento.corp.example"}).Validate(); err == nil {
		t.Fatal("momento needs a site id")
	}
	if err := (Config{Enabled: true, Provider: ProviderGA4}).Validate(); err == nil {
		t.Fatal("ga4 needs a measurement id")
	}
	if err := (Config{Enabled: true, Provider: ProviderCustom}).Validate(); err == nil {
		t.Fatal("custom needs a snippet")
	}
	if err := (Config{Enabled: false, Provider: ProviderCustom}).Validate(); err != nil {
		t.Fatalf("a disabled configuration is never wrong: %v", err)
	}
	if err := momento().Validate(); err != nil {
		t.Fatal(err)
	}
	if got := (Config{Placement: "BODY"}).Normalize().Placement; got != "body" {
		t.Fatalf("placement %q", got)
	}
	if got := (Config{Placement: "footer"}).Normalize().Placement; got != "head" {
		t.Fatalf("placement %q", got)
	}
}

func TestAddAllowedHost(t *testing.T) {
	if got := AddAllowedHost("", "https://a.example/"); got != "https://a.example" {
		t.Fatalf("%q", got)
	}
	if got := AddAllowedHost("https://a.example", "https://A.example"); got != "https://a.example" {
		t.Fatalf("duplicates are not added: %q", got)
	}
	if got := AddAllowedHost("https://a.example", "https://b.example"); got != "https://a.example, https://b.example" {
		t.Fatalf("%q", got)
	}
}

func TestRecorderKeepsDistinctOriginsNotCounts(t *testing.T) {
	recorder := NewRecorder()
	moment := time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)
	recorder.now = func() time.Time { return moment }
	for range 5 {
		recorder.Record("https://momento.corp.example/collect/v1/events", "connect-src https://x", "https://muni.corp.example/docs/1")
		moment = moment.Add(time.Second)
	}
	recorder.Record("chrome-extension://abc/inject.js", "script-src", "/")
	recorder.Record("inline", "script-src-elem", "/")
	recorder.Record("https://momento.corp.example/tracker.js", "script-src-elem", "/")
	items := recorder.List(Config{})
	if len(items) != 2 {
		t.Fatalf("items %+v", items)
	}
	if items[0].Directive != "script-src-elem" || items[1].Origin != "https://momento.corp.example" || items[1].Count != 5 || items[1].Directive != "connect-src" {
		t.Fatalf("most recent first, counts folded: %+v", items)
	}
	if items[1].FirstSeen.Equal(items[1].LastSeen) {
		t.Fatal("last seen should move")
	}

	config := momento()
	config.MomentoProxy = false
	if listed := recorder.List(config); !listed[0].Allowed || !listed[1].Allowed {
		t.Fatalf("an origin the configuration allows is marked: %+v", listed)
	}
	ga := Config{Enabled: true, Provider: ProviderGA4, MeasurementID: "G-1"}
	moment = moment.Add(time.Second)
	recorder.Record("https://region1.google-analytics.com/g/collect", "connect-src", "/")
	if listed := recorder.List(ga); !listed[0].Allowed {
		t.Fatalf("wildcards count: %+v", listed[0])
	}

	recorder.Forget()
	if len(recorder.List(Config{})) != 0 {
		t.Fatal("forget")
	}
}

func TestRecorderIsBounded(t *testing.T) {
	recorder := NewRecorder()
	moment := time.Unix(0, 0)
	recorder.now = func() time.Time { moment = moment.Add(time.Second); return moment }
	for index := range MaxViolations + 10 {
		recorder.Record("https://host"+string(rune('a'+index%26))+strings.Repeat("x", index/26)+".example/", "img-src", "/")
	}
	items := recorder.List(Config{})
	if len(items) != MaxViolations {
		t.Fatalf("%d items", len(items))
	}
	if items[len(items)-1].Origin == "https://hosta.example" {
		t.Fatal("the oldest should have been evicted")
	}
}
