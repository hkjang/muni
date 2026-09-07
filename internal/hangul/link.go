package hangul

import (
	"strings"

	"github.com/hkjang/muni/internal/richdoc"
)

// LinkAddress reads where a hyperlink field points and answers only when the
// address is one a document may carry.
//
// Both Hangul formats write the same thing: the address, then options, all
// separated by semicolons, with the colon of the scheme escaped —
// `http\://www.hancom.co.kr;1;0;0;`. A bare host with no scheme is a web
// address and is completed as one.
//
// Everything else is refused. A link is the one thing an imported document
// carries that a reader will click, and a file that says `javascript\:…` is
// not describing a destination — it is describing an attack. Word, HTML and
// Markdown imports have always checked this; these two did not, and their
// links went into stored documents unexamined.
func LinkAddress(command string) string {
	address := command
	if cut := strings.Index(address, ";"); cut >= 0 {
		address = address[:cut]
	}
	address = strings.TrimSpace(strings.ReplaceAll(address, `\:`, ":"))
	if address == "" {
		return ""
	}
	if !strings.Contains(address, ":") {
		address = "http://" + address
	}
	return richdoc.SafeLink(address)
}
