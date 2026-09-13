package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/settings"
)

// Silent SSO is a browser bouncing between muni and the identity provider with
// nothing on screen. The whole job is making sure it bounces at most once, so
// these follow the two legs of one attempt — the start that asks the provider
// for prompt=none and the callback that receives its refusal — against the
// state row that ties them together.

// fakeIssuer serves just enough discovery for go-oidc to accept it. The
// authorization endpoint is never called: the test reads the redirect instead
// of following it.
func fakeIssuer(t *testing.T) *httptest.Server {
	t.Helper()
	var issuer *httptest.Server
	issuer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                issuer.URL,
			"authorization_endpoint":                issuer.URL + "/authorize",
			"token_endpoint":                        issuer.URL + "/token",
			"jwks_uri":                              issuer.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	}))
	t.Cleanup(issuer.Close)
	return issuer
}

// configureOIDC points the service at the fake issuer and restores whatever was
// there before, so the shared database does not keep a provider that no longer
// exists once the test is over.
func configureOIDC(t *testing.T, srv *serverUnderTest, issuer string, autoLogin bool) {
	t.Helper()
	resp, err := srv.admin.Get(srv.URL + "/api/v1/admin/settings")
	if err != nil {
		t.Fatal(err)
	}
	var current struct {
		Data map[string]any `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&current)
	resp.Body.Close()
	original, _ := json.Marshal(current.Data)
	oidc, _ := current.Data["oidc"].(map[string]any)
	if oidc == nil {
		oidc = map[string]any{}
	}
	hadSecret, _ := oidc["secretSet"].(bool)
	t.Cleanup(func() {
		// Saving the original form leaves the secret in place (an empty
		// secret means "keep"), and every live test seals with its own random
		// key — a secret this test wrote would be unreadable to the next one.
		// Removed first so a restore that fails cannot leave it behind.
		if !hadSecret {
			if _, err := srv.db.Exec(context.Background(), `DELETE FROM app_settings WHERE key='oidc.client_secret'`); err != nil {
				t.Fatal(err)
			}
		}
		putSettings(t, srv, original)
	})
	oidc["enabled"] = true
	oidc["issuerUrl"] = issuer
	oidc["clientId"] = "muni"
	oidc["clientSecret"] = "silent-sso-secret"
	oidc["scopes"] = []string{"openid", "profile", "email"}
	oidc["defaultRole"] = "USER"
	oidc["autoLogin"] = autoLogin
	current.Data["oidc"] = oidc
	body, _ := json.Marshal(current.Data)
	putSettings(t, srv, body)
}

func putSettings(t *testing.T, srv *serverUnderTest, body []byte) {
	t.Helper()
	req, _ := http.NewRequest("PUT", srv.URL+"/api/v1/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	saved, err := srv.admin.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(saved.Body)
	saved.Body.Close()
	if saved.StatusCode != 200 {
		t.Fatalf("settings = %d %s", saved.StatusCode, raw)
	}
}

// noRedirects is a client that hands back the 302 instead of following it to
// a provider that is not there.
func noRedirects() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func redirectedTo(t *testing.T, client *http.Client, target string) *url.URL {
	t.Helper()
	resp, err := client.Get(target)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("GET %s = %d %s", target, resp.StatusCode, raw)
	}
	location, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return location
}

func publicSystemAutoLogin(t *testing.T, srv *serverUnderTest) bool {
	t.Helper()
	resp, err := http.Get(srv.URL + "/api/v1/system/public")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Data struct {
			OIDCAutoLogin bool `json:"oidcAutoLogin"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out.Data.OIDCAutoLogin
}

// With auto-login off, ?prompt=none is just a query string somebody typed: the
// start is an ordinary login and the browser is not told to try silently.
func TestPromptNoneIsIgnoredWhileAutoLoginIsOff(t *testing.T) {
	srv := newServerUnderTest(t)
	configureOIDC(t, srv, fakeIssuer(t).URL, false)
	if publicSystemAutoLogin(t, srv) {
		t.Fatal("the public system document should not advertise auto-login")
	}
	location := redirectedTo(t, noRedirects(), srv.URL+"/api/v1/auth/oidc/start?prompt=none&return_to=%2Fdocs%2F1")
	if location.Path != "/authorize" {
		t.Fatalf("start went to %s", location)
	}
	if got := location.Query().Get("prompt"); got != "" {
		t.Fatalf("prompt=%q reached the provider with auto-login off", got)
	}
	// And the provider's refusal of that ordinary login is still an error.
	state := location.Query().Get("state")
	landed := redirectedTo(t, noRedirects(), srv.URL+"/api/v1/auth/oidc/callback?error=login_required&state="+url.QueryEscape(state))
	if landed.String() != "/login?error=login_required" {
		t.Fatalf("landed on %s", landed)
	}
}

// With auto-login on, the start carries prompt=none and the refusal lands on
// the login screen with the marker that stops a retry — and the deep link the
// visitor came in with, so signing in by hand still ends up there.
func TestASilentRefusalLandsOnTheLoginScreenWithTheMarker(t *testing.T) {
	srv := newServerUnderTest(t)
	configureOIDC(t, srv, fakeIssuer(t).URL, true)
	if !publicSystemAutoLogin(t, srv) {
		t.Fatal("the public system document should advertise auto-login")
	}
	location := redirectedTo(t, noRedirects(), srv.URL+"/api/v1/auth/oidc/start?prompt=none&return_to=%2Fdocs%2F1%3Ftab%3Dhistory")
	if got := location.Query().Get("prompt"); got != "none" {
		t.Fatalf("prompt=%q, want none: %s", got, location)
	}
	state := location.Query().Get("state")
	if state == "" {
		t.Fatalf("no state in %s", location)
	}
	landed := redirectedTo(t, noRedirects(), srv.URL+"/api/v1/auth/oidc/callback?error=login_required&state="+url.QueryEscape(state))
	if landed.Path != "/login" || landed.Query().Get("sso") != "none" || landed.Query().Get("error") != "" {
		t.Fatalf("landed on %s", landed)
	}
	if got := landed.Query().Get("return_to"); got != "/docs/1?tab=history" {
		t.Fatalf("return_to=%q", got)
	}
	// The state row is used up by the refusal: presenting it again is no
	// longer a silent attempt, so it cannot be replayed to mint the marker.
	again := redirectedTo(t, noRedirects(), srv.URL+"/api/v1/auth/oidc/callback?error=login_required&state="+url.QueryEscape(state))
	if again.String() != "/login?error=login_required" {
		t.Fatalf("replayed state landed on %s", again)
	}
	// An ordinary start under the same settings still reports its refusal as
	// an error; silence is a property of the attempt, not of the setting.
	plain := redirectedTo(t, noRedirects(), srv.URL+"/api/v1/auth/oidc/start")
	if plain.Query().Get("prompt") != "" {
		t.Fatalf("an ordinary start asked for prompt=%q", plain.Query().Get("prompt"))
	}
	refused := redirectedTo(t, noRedirects(), srv.URL+"/api/v1/auth/oidc/callback?error=login_required&state="+url.QueryEscape(plain.Query().Get("state")))
	if refused.String() != "/login?error=login_required" {
		t.Fatalf("ordinary refusal landed on %s", refused)
	}
}

// The marker page must never carry a return_to that leads off the site; the
// callback is the one place that value re-enters an address.
func TestASilentRefusalKeepsOnlyASafeReturnPath(t *testing.T) {
	srv := newServerUnderTest(t)
	configureOIDC(t, srv, fakeIssuer(t).URL, true)
	for _, hostile := range []string{"//evil.example/x", "https://evil.example/", "/"} {
		location := redirectedTo(t, noRedirects(), srv.URL+"/api/v1/auth/oidc/start?prompt=none&return_to="+url.QueryEscape(hostile))
		landed := redirectedTo(t, noRedirects(), srv.URL+"/api/v1/auth/oidc/callback?error=login_required&state="+url.QueryEscape(location.Query().Get("state")))
		if landed.String() != silentRefusedPath {
			t.Fatalf("return_to %q landed on %s", hostile, landed)
		}
	}
}

func TestSilentLoginIsOnlyRequestedWhenTheSettingAllowsIt(t *testing.T) {
	on := settings.OIDC{AutoLogin: true}
	off := settings.OIDC{}
	for _, tc := range []struct {
		query string
		cfg   settings.OIDC
		want  bool
	}{
		{"prompt=none", on, true},
		{"prompt=none", off, false},
		{"prompt=login", on, false},
		{"", on, false},
	} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/start?"+tc.query, nil)
		if got := silentLoginRequested(r, tc.cfg); got != tc.want {
			t.Errorf("%q autoLogin=%v: got %v", tc.query, tc.cfg.AutoLogin, got)
		}
	}
	if !strings.HasPrefix(silentRefusedPath, "/login?") {
		t.Fatalf("the refusal must land on the login page: %q", silentRefusedPath)
	}
}
