package ptium

import (
	"net/url"
	"strings"
)

// Config is what an administrator sets to connect a Ptium server.
type Config struct {
	Enabled        bool
	BaseURL        string
	APIKey         string
	DefaultTheme   string
	DefaultLang    string
	TimeoutSeconds int
	// WebURL is where a person is sent to edit the deck. It defaults to the
	// API base because the usual deployment serves both from one origin.
	WebURL string
}

const (
	defaultTimeoutSeconds = 120
	defaultLanguage       = "ko"
)

// Normalize fills in the values an administrator did not set and trims the
// rest, so a half-filled form fails with a clear message rather than a
// malformed request.
func (c Config) Normalize() Config {
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	c.WebURL = strings.TrimRight(strings.TrimSpace(c.WebURL), "/")
	c.APIKey = strings.TrimSpace(c.APIKey)
	c.DefaultTheme = strings.TrimSpace(c.DefaultTheme)
	c.DefaultLang = strings.TrimSpace(c.DefaultLang)
	if c.DefaultLang == "" {
		c.DefaultLang = defaultLanguage
	}
	if c.TimeoutSeconds < 5 || c.TimeoutSeconds > 900 {
		c.TimeoutSeconds = defaultTimeoutSeconds
	}
	if c.WebURL == "" {
		c.WebURL = c.BaseURL
	}
	return c
}

// Usable reports whether a request can be attempted at all.
func (c Config) Usable() bool {
	if !c.Enabled || c.BaseURL == "" || c.APIKey == "" {
		return false
	}
	parsed, err := url.Parse(c.BaseURL)
	return err == nil && parsed.Scheme != "" && parsed.Host != ""
}

// EditorURL is the deep link that takes a person to the deck in Ptium.
//
// The trailing /editor is not decoration. Ptium routes on the exact path
// /presentations/{id}/editor and has no route for the deck on its own, so a
// link without it lands on the not-found page.
func (c Config) EditorURL(presentationID string) string {
	if c.WebURL == "" || presentationID == "" {
		return ""
	}
	return c.WebURL + "/presentations/" + url.PathEscape(presentationID) + "/editor"
}
