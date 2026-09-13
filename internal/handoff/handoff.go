// Package handoff passes a document to another service without anyone
// downloading a file.
//
// The shape is the one every service on the network follows (HANDOFF-STANDARD):
// the sender issues a claim — a short, single-use token bound to one document
// — and the receiver, given the sender's origin and the claim, fetches the
// document from the sender itself. No service holds another's credentials.
//
// This package is the part that does not touch a database: which formats muni
// sends and receives, the administrator's list of peers, and the fetch on the
// receiving side with every guard the standard demands, because the source is
// a value that came in from outside.
package handoff

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"
)

const (
	FormatMarkdown = "markdown"
	FormatDOCX     = "docx"

	// ClaimTTL is how long a claim can be redeemed. The standard says five
	// minutes at most; the token is the only credential, so it stays short.
	ClaimTTL = 5 * time.Minute

	// MaxBodyBytes and FetchTimeout bound what the receiving side is willing
	// to take from a peer, however trusted the list says it is.
	MaxBodyBytes = 25 << 20
	FetchTimeout = 30 * time.Second

	// ClaimsPath is the sender's endpoint; the receiver appends the claim.
	ClaimsPath = "/api/v1/handoff/claims"
)

// Sends and Receives are muni's row in the standard's format table. A button
// for a format the peer cannot receive is never shown; a document in a format
// muni cannot read is refused before it is fetched.
var (
	Sends    = []string{FormatMarkdown, FormatDOCX}
	Receives = []string{FormatMarkdown}

	// KnownFormats is everything any service on the network sends or
	// receives, which is what the administrator may tick for a peer.
	KnownFormats = []string{FormatMarkdown, FormatDOCX, "csv", "xlsx", "pptx", "txt"}
)

// mediaTypes is what a format arrives as. The check is on the parsed media
// type, so a charset parameter neither helps nor hurts.
var mediaTypes = map[string][]string{
	FormatMarkdown: {"text/markdown", "text/x-markdown"},
	FormatDOCX:     {"application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
}

// Peer is one service the administrator allowed: where it lives, what to call
// it in a menu, and which formats it can take.
type Peer struct {
	Origin   string   `json:"origin"`
	Name     string   `json:"name"`
	Receives []string `json:"receives"`
}

// Config is the allow list. It is empty by default, and while it is empty
// nothing is sent and nothing is accepted: a fresh install is unchanged.
type Config struct {
	Peers []Peer `json:"peers"`
}

// Normalize brings a stored or submitted list into shape without judging it:
// origins are lowered to scheme and host, names trimmed, formats deduplicated.
func (c Config) Normalize() Config {
	peers := make([]Peer, 0, len(c.Peers))
	for _, peer := range c.Peers {
		peer.Origin = OriginOf(peer.Origin)
		peer.Name = strings.TrimSpace(peer.Name)
		formats := make([]string, 0, len(peer.Receives))
		for _, format := range peer.Receives {
			format = strings.ToLower(strings.TrimSpace(format))
			if format != "" && !slices.Contains(formats, format) {
				formats = append(formats, format)
			}
		}
		peer.Receives = formats
		peers = append(peers, peer)
	}
	c.Peers = peers
	return c
}

// Validate refuses a list that could not be meant: an origin that is not an
// http(s) origin, the same origin twice, or a format no service uses.
func (c Config) Validate() error {
	c = c.Normalize()
	seen := map[string]bool{}
	for _, peer := range c.Peers {
		if peer.Origin == "" {
			return errors.New("문서를 주고받을 서비스의 주소는 http 또는 https 오리진이어야 합니다")
		}
		if seen[peer.Origin] {
			return fmt.Errorf("같은 주소가 두 번 있습니다: %s", peer.Origin)
		}
		seen[peer.Origin] = true
		if len([]rune(peer.Name)) > 60 {
			return errors.New("서비스 이름은 60자 이하여야 합니다")
		}
		for _, format := range peer.Receives {
			if !slices.Contains(KnownFormats, format) {
				return fmt.Errorf("알 수 없는 형식입니다: %s", format)
			}
		}
	}
	return nil
}

// Allowed reports whether a source a browser brought is on the list. The
// comparison is on the whole origin, after the same normalisation the list
// went through; a path, query or userinfo on the source is not an origin and
// matches nothing.
func (c Config) Allowed(source string) (Peer, bool) {
	origin := strictOrigin(source)
	if origin == "" {
		return Peer{}, false
	}
	for _, peer := range c.Normalize().Peers {
		if peer.Origin == origin {
			return peer, true
		}
	}
	return Peer{}, false
}

// Target is a peer as the send menu sees it: only the formats muni can send
// and the peer can receive, with docx left out when the caller says so.
type Target struct {
	Origin  string   `json:"origin"`
	Name    string   `json:"name"`
	Formats []string `json:"formats"`
}

// Targets lists where a document can go. A peer that receives nothing muni
// sends is not a destination and does not appear.
func (c Config) Targets(docxAllowed bool) []Target {
	targets := []Target{}
	for _, peer := range c.Normalize().Peers {
		if peer.Origin == "" {
			continue
		}
		formats := []string{}
		for _, format := range Sends {
			if format == FormatDOCX && !docxAllowed {
				continue
			}
			if slices.Contains(peer.Receives, format) {
				formats = append(formats, format)
			}
		}
		if len(formats) == 0 {
			continue
		}
		name := peer.Name
		if name == "" {
			name = strings.TrimPrefix(strings.TrimPrefix(peer.Origin, "https://"), "http://")
		}
		targets = append(targets, Target{Origin: peer.Origin, Name: name, Formats: formats})
	}
	return targets
}

// OriginOf reduces an address to its origin: scheme and host, lowered. Anything
// that is not an http(s) address is "".
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

// strictOrigin is OriginOf for a value that must already be an origin. A
// source with a path would let "https://umm.intra/../../other" claim to be
// umm; it is refused rather than trimmed.
func strictOrigin(raw string) string {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return ""
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return ""
	}
	return OriginOf(raw)
}

// ValidClaim reports whether a claim looks like one a peer would have issued:
// what RandomToken produces, and nothing that could be a path.
func ValidClaim(claim string) bool {
	if len(claim) < 16 || len(claim) > 256 {
		return false
	}
	for _, letter := range claim {
		switch {
		case letter >= 'a' && letter <= 'z', letter >= 'A' && letter <= 'Z', letter >= '0' && letter <= '9', letter == '-', letter == '_':
		default:
			return false
		}
	}
	return true
}

// Document is what a claim turned out to be.
type Document struct {
	Format      string
	Filename    string
	ContentType string
	Body        []byte
}

// The ways a fetch fails, each of which the receiving page says in words.
var (
	ErrNotAllowed  = errors.New("source is not on the allow list")
	ErrBadClaim    = errors.New("claim is malformed")
	ErrNotFound    = errors.New("claim was refused by the source")
	ErrRedirect    = errors.New("source answered with a redirect")
	ErrUnsupported = errors.New("source sent a format this service cannot read")
	ErrTooLarge    = errors.New("document is larger than the limit")
	ErrUnreachable = errors.New("source could not be reached")
)

// NewClient is the client every fetch goes through: it never follows a
// redirect — a peer that redirects is sending the request somewhere the list
// did not approve — and it gives up after FetchTimeout.
func NewClient(transport http.RoundTripper) *http.Client {
	return &http.Client{
		Transport: transport,
		Timeout:   FetchTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// Fetch redeems a claim at a source. The allow list is checked first, and a
// source that is not on it costs no request at all. The response is refused
// before its body is read when its type is not one muni receives, and the body
// is cut off at MaxBodyBytes.
func Fetch(ctx context.Context, client *http.Client, config Config, source, claim string) (Document, error) {
	peer, ok := config.Allowed(source)
	if !ok {
		return Document{}, ErrNotAllowed
	}
	if !ValidClaim(claim) {
		return Document{}, ErrBadClaim
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, peer.Origin+ClaimsPath+"/"+claim, nil)
	if err != nil {
		return Document{}, ErrUnreachable
	}
	request.Header.Set("Accept", strings.Join(accepted(), ", "))
	response, err := client.Do(request)
	if err != nil {
		return Document{}, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode >= 300 && response.StatusCode < 400:
		return Document{}, ErrRedirect
	case response.StatusCode == http.StatusNotFound:
		return Document{}, ErrNotFound
	case response.StatusCode != http.StatusOK:
		return Document{}, fmt.Errorf("%w: status %d", ErrUnreachable, response.StatusCode)
	}
	format, ok := formatOf(response.Header.Get("Content-Type"))
	if !ok {
		return Document{}, ErrUnsupported
	}
	if response.ContentLength > MaxBodyBytes {
		return Document{}, ErrTooLarge
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, MaxBodyBytes+1))
	if err != nil {
		return Document{}, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	if len(body) > MaxBodyBytes {
		return Document{}, ErrTooLarge
	}
	return Document{
		Format:      format,
		Filename:    filenameOf(response.Header.Get("Content-Disposition")),
		ContentType: response.Header.Get("Content-Type"),
		Body:        body,
	}, nil
}

// accepted lists the media types of the formats muni receives.
func accepted() []string {
	types := []string{}
	for _, format := range Receives {
		types = append(types, mediaTypes[format]...)
	}
	return types
}

// formatOf names the received format for a Content-Type, or reports that it
// is not one muni reads.
func formatOf(contentType string) (string, bool) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", false
	}
	for _, format := range Receives {
		if slices.Contains(mediaTypes[format], strings.ToLower(mediaType)) {
			return format, true
		}
	}
	return "", false
}

// filenameOf reads the name the sender gave the file, from either form of
// the Content-Disposition header, and keeps only the base name.
func filenameOf(disposition string) string {
	_, params, err := mime.ParseMediaType(disposition)
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(params["filename"])
	name = strings.NewReplacer("\\", "/", "\x00", "").Replace(name)
	name = path.Base(name)
	if name == "." || name == "/" {
		return ""
	}
	return name
}

// MediaType is what a format goes out as, charset included where the format
// is text — it is what the standard's example shows a receiver.
func MediaType(format string) string {
	switch format {
	case FormatMarkdown:
		return "text/markdown; charset=utf-8"
	case FormatDOCX:
		return mediaTypes[FormatDOCX][0]
	}
	return "application/octet-stream"
}

// Extension is the file suffix a format is saved under.
func Extension(format string) string {
	switch format {
	case FormatMarkdown:
		return ".md"
	default:
		return "." + format
	}
}

// NormalizeFormat accepts the spellings a peer might use for a format muni
// sends, and reports whether it is one.
func NormalizeFormat(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case FormatMarkdown, "md":
		return FormatMarkdown, true
	case FormatDOCX:
		return FormatDOCX, true
	}
	return "", false
}
