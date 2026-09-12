// Package tracking puts a visitor tracking snippet into the served page.
//
// muni ships with a content security policy that allows scripts from its own
// origin only, so a snippet cannot simply be pasted into the page: the browser
// refuses it and the administrator sees an empty dashboard and no reason. This
// package produces both halves of the answer — the markup to inject and the
// policy sources it needs — with a per-request nonce so the snippet's inline
// code runs without loosening the policy for everything else.
package tracking

import (
	"errors"
	"fmt"
	"html"
	"net/url"
	"strings"
)

const (
	ProviderNone    = "none"
	ProviderMomento = "momento"
	ProviderGA4     = "ga4"
	ProviderGTM     = "gtm"
	ProviderMatomo  = "matomo"
	ProviderCustom  = "custom"

	// MaxSnippetBytes bounds a pasted snippet. A loader is a few hundred
	// bytes; anything larger is a mistake or an attempt to store something
	// else in a setting.
	MaxSnippetBytes = 8 * 1024

	// ProxyPath is where the same-origin Momento proxy answers. The snippet
	// loads its tracker from and reports to this prefix, so no outside origin
	// has to appear in the policy at all.
	ProxyPath = "/momento"
)

// Config is the tracking setting as stored and shown in the console.
type Config struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider"`
	// MomentoURL and MomentoSiteID reach the organisation's own collector.
	// MomentoProxy sends the tracker through muni instead of straight to it,
	// which is the default because it keeps the policy unchanged.
	MomentoURL    string `json:"momentoUrl"`
	MomentoSiteID string `json:"momentoSiteId"`
	MomentoProxy  bool   `json:"momentoProxy"`
	MeasurementID string `json:"measurementId"`
	MatomoURL     string `json:"matomoUrl"`
	MatomoSiteID  string `json:"matomoSiteId"`
	CustomSnippet string `json:"customSnippet"`
	// AllowedHosts is where an administrator adds an origin the snippet did
	// not name itself. Comma, space or newline separated.
	AllowedHosts string `json:"allowedHosts"`
	IncludeAdmin bool   `json:"includeAdmin"`
	// Placement is "head" or "body".
	Placement string `json:"placement"`
}

// Normalize brings a stored or submitted configuration into range without
// judging it; Validate is what refuses.
func (c Config) Normalize() Config {
	c.Provider = strings.ToLower(strings.TrimSpace(c.Provider))
	if c.Provider == "" {
		c.Provider = ProviderNone
	}
	c.Placement = strings.ToLower(strings.TrimSpace(c.Placement))
	if c.Placement != "body" {
		c.Placement = "head"
	}
	c.MomentoURL = strings.TrimRight(strings.TrimSpace(c.MomentoURL), "/")
	c.MomentoSiteID = strings.TrimSpace(c.MomentoSiteID)
	c.MeasurementID = strings.TrimSpace(c.MeasurementID)
	c.MatomoURL = strings.TrimRight(strings.TrimSpace(c.MatomoURL), "/")
	c.MatomoSiteID = strings.TrimSpace(c.MatomoSiteID)
	c.CustomSnippet = strings.TrimSpace(c.CustomSnippet)
	c.AllowedHosts = strings.TrimSpace(c.AllowedHosts)
	return c
}

// Validate reports what is missing for the chosen provider. A disabled
// configuration is never wrong: the fields are kept as typed so they are
// there again when tracking is switched back on.
func (c Config) Validate() error {
	c = c.Normalize()
	switch c.Provider {
	case ProviderNone, ProviderMomento, ProviderGA4, ProviderGTM, ProviderMatomo, ProviderCustom:
	default:
		return errors.New("방문 추적 제공자는 none, momento, ga4, gtm, matomo, custom 중 하나여야 합니다")
	}
	if len(c.CustomSnippet) > MaxSnippetBytes {
		return fmt.Errorf("추적 코드는 %d바이트를 넘을 수 없습니다", MaxSnippetBytes)
	}
	if !c.Enabled {
		return nil
	}
	switch c.Provider {
	case ProviderMomento:
		if !validURL(c.MomentoURL) {
			return errors.New("Momento 수집기 주소가 올바르지 않습니다")
		}
		if c.MomentoSiteID == "" {
			return errors.New("Momento 사이트 id 가 필요합니다")
		}
	case ProviderGA4, ProviderGTM:
		if c.MeasurementID == "" {
			return errors.New("GA4·GTM 의 측정 id 가 필요합니다")
		}
	case ProviderMatomo:
		if !validURL(c.MatomoURL) {
			return errors.New("Matomo 주소가 올바르지 않습니다")
		}
		if c.MatomoSiteID == "" {
			return errors.New("Matomo 사이트 id 가 필요합니다")
		}
	case ProviderCustom:
		if c.CustomSnippet == "" {
			return errors.New("추적 코드가 비어 있습니다")
		}
	}
	return nil
}

// Active reports whether the page at path should carry the snippet. The
// console is left out unless an administrator asks for it, because what the
// administrators click is rarely the visitor data anybody wanted.
func (c Config) Active(path string) bool {
	c = c.Normalize()
	if !c.Enabled || c.Provider == ProviderNone {
		return false
	}
	if !c.IncludeAdmin && (path == "/admin" || strings.HasPrefix(path, "/admin/")) {
		return false
	}
	return c.Snippet("") != ""
}

// ProxyActive reports whether /momento/* should be forwarded to the collector.
func (c Config) ProxyActive() bool {
	c = c.Normalize()
	return c.Enabled && c.Provider == ProviderMomento && c.MomentoProxy && validURL(c.MomentoURL)
}

// Snippet renders the markup to inject with the nonce on every script tag,
// which is what lets it run under a policy that stays strict.
func (c Config) Snippet(nonce string) string {
	c = c.Normalize()
	if !c.Enabled {
		return ""
	}
	var markup string
	switch c.Provider {
	case ProviderMomento:
		if c.MomentoSiteID == "" || (!c.MomentoProxy && !validURL(c.MomentoURL)) {
			return ""
		}
		site := html.EscapeString(c.MomentoSiteID)
		if c.MomentoProxy {
			// Through the proxy the tracker is a same-origin file and the
			// collector a same-origin path, so 'self' already covers both.
			markup = fmt.Sprintf(`<script async src="%s/tracker.js" data-site-id="%s" data-environment="prd" data-contract-version="1" data-endpoint="%s"></script>`, ProxyPath, site, ProxyPath)
		} else {
			markup = fmt.Sprintf(`<script async src="%s/tracker.js" data-site-id="%s" data-environment="prd" data-contract-version="1"></script>`, html.EscapeString(c.MomentoURL), site)
		}
	case ProviderGA4:
		if c.MeasurementID == "" {
			return ""
		}
		id := html.EscapeString(c.MeasurementID)
		markup = fmt.Sprintf(`<script async src="https://www.googletagmanager.com/gtag/js?id=%s"></script>
<script>window.dataLayer=window.dataLayer||[];function gtag(){dataLayer.push(arguments);}gtag('js',new Date());gtag('config','%s');</script>`, id, id)
	case ProviderGTM:
		if c.MeasurementID == "" {
			return ""
		}
		markup = fmt.Sprintf(`<script>(function(w,d,s,l,i){w[l]=w[l]||[];w[l].push({'gtm.start':new Date().getTime(),event:'gtm.js'});var f=d.getElementsByTagName(s)[0],j=d.createElement(s),dl=l!='dataLayer'?'&l='+l:'';j.async=true;j.src='https://www.googletagmanager.com/gtm.js?id='+i+dl;f.parentNode.insertBefore(j,f);})(window,document,'script','dataLayer','%s');</script>`, html.EscapeString(c.MeasurementID))
	case ProviderMatomo:
		if !validURL(c.MatomoURL) || c.MatomoSiteID == "" {
			return ""
		}
		markup = fmt.Sprintf(`<script>var _paq=window._paq=window._paq||[];_paq.push(['trackPageView']);_paq.push(['enableLinkTracking']);(function(){var u="%s/";_paq.push(['setTrackerUrl',u+'matomo.php']);_paq.push(['setSiteId','%s']);var d=document,g=d.createElement('script'),s=d.getElementsByTagName('script')[0];g.async=true;g.src=u+'matomo.js';s.parentNode.insertBefore(g,s);})();</script>`, html.EscapeString(c.MatomoURL), html.EscapeString(c.MatomoSiteID))
	case ProviderCustom:
		markup = c.CustomSnippet
	default:
		return ""
	}
	return WithNonce(markup, nonce)
}

// WithNonce adds the nonce to every script tag that does not already carry
// one. A pasted snippet is left otherwise untouched.
func WithNonce(markup, nonce string) string {
	if nonce == "" || markup == "" {
		return markup
	}
	var out strings.Builder
	rest := markup
	for {
		index := strings.Index(strings.ToLower(rest), "<script")
		if index < 0 {
			out.WriteString(rest)
			return out.String()
		}
		end := index + len("<script")
		out.WriteString(rest[:end])
		tag := rest[end:]
		if closing := strings.Index(tag, ">"); closing >= 0 {
			tag = tag[:closing]
		}
		if !strings.Contains(strings.ToLower(tag), "nonce=") {
			out.WriteString(` nonce="` + html.EscapeString(nonce) + `"`)
		}
		rest = rest[end:]
	}
}

// PolicySources lists the origins the snippet needs beyond 'self', for the
// script-src, connect-src and img-src directives. The common providers are
// known outright; a pasted snippet names the addresses it loads and reports
// to, and those are read out of it so nobody has to translate a policy error
// into a host name first.
func (c Config) PolicySources() (scripts, connects, images []string) {
	c = c.Normalize()
	if !c.Enabled {
		return nil, nil, nil
	}
	add := func(origin string) {
		scripts = append(scripts, origin)
		connects = append(connects, origin)
		images = append(images, origin)
	}
	switch c.Provider {
	case ProviderMomento:
		if !c.MomentoProxy {
			if origin := OriginOf(c.MomentoURL); origin != "" {
				add(origin)
			}
		}
	case ProviderGA4, ProviderGTM:
		scripts = append(scripts, "https://www.googletagmanager.com")
		connects = append(connects, "https://www.google-analytics.com", "https://analytics.google.com", "https://*.google-analytics.com")
		images = append(images, "https://www.google-analytics.com", "https://www.googletagmanager.com")
	case ProviderMatomo:
		if origin := OriginOf(c.MatomoURL); origin != "" {
			add(origin)
		}
	case ProviderCustom:
		for _, origin := range SnippetOrigins(c.CustomSnippet) {
			add(origin)
		}
	}
	for _, host := range SplitHosts(c.AllowedHosts) {
		add(host)
	}
	return scripts, connects, images
}

// SnippetOrigins lists every http(s) origin written into a snippet, in the
// order met and without repeats: the script it loads, the endpoint it posts
// to, the pixel it requests.
func SnippetOrigins(snippet string) []string {
	origins := make([]string, 0, 2)
	seen := make(map[string]bool, 2)
	lower := strings.ToLower(snippet)
	for index := 0; index < len(snippet); {
		start := strings.Index(lower[index:], "http")
		if start < 0 {
			break
		}
		start += index
		end := start
		for end < len(snippet) && !urlBoundary(snippet[end]) {
			end++
		}
		index = end
		origin := OriginOf(snippet[start:end])
		if origin == "" || seen[origin] {
			continue
		}
		seen[origin] = true
		origins = append(origins, origin)
	}
	return origins
}

// urlBoundary reports the characters that cannot be part of a URL written
// inside HTML or JavaScript, which is where each address ends.
func urlBoundary(letter byte) bool {
	switch letter {
	case '"', '\'', '`', '<', '>', ' ', '\t', '\n', '\r', ')', ',', ';', '\\', '+':
		return true
	}
	return false
}

// OriginOf reduces an address to scheme and host, or "" when it is not an
// http(s) address. A browser extension or a data: URL cannot be allowed and
// is not worth listing.
func OriginOf(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return ""
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	return scheme + "://" + strings.ToLower(parsed.Host)
}

// SplitHosts reads the allow list the administrator typed.
func SplitHosts(list string) []string {
	fields := strings.FieldsFunc(list, func(letter rune) bool {
		return letter == ',' || letter == ' ' || letter == '\n' || letter == '\r' || letter == '\t'
	})
	hosts := fields[:0]
	for _, field := range fields {
		if trimmed := strings.TrimSuffix(strings.TrimSpace(field), "/"); trimmed != "" {
			hosts = append(hosts, trimmed)
		}
	}
	return hosts
}

// AddAllowedHost appends an origin to the allow list, leaving the existing
// entries and their order alone. Adding one that is already there is a no-op.
func AddAllowedHost(existing, origin string) string {
	origin = strings.TrimSuffix(strings.TrimSpace(origin), "/")
	if origin == "" {
		return existing
	}
	for _, host := range SplitHosts(existing) {
		if strings.EqualFold(host, origin) {
			return existing
		}
	}
	if strings.TrimSpace(existing) == "" {
		return origin
	}
	return strings.TrimSpace(existing) + ", " + origin
}

func validURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}
