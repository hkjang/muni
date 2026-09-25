package httpapi

import (
	"fmt"
	"mime"
	"testing"
)

// Every download muni sends names the file twice in Content-Disposition: a
// plain ASCII `filename` for a client that understands nothing else, and
// `filename*=UTF-8”…`, the parameter that actually carries the title. RFC 8187
// spells that second one as an ext-value, and an ext-value is not a URL path —
// it is a token, so everything outside a short attr-char set has to be percent
// encoded or the parameter stops being a parameter.
//
// So these do not compare the escaper against itself. They build the header the
// routes build and read it back with mime.ParseMediaType, which is the parser
// on the other end of the wire: internal/handoff's filenameOf calls exactly
// that on a header muni sent. A title is right here only if a taker gets it
// back rune for rune.
func TestAnExtValueIsReadBackUnchangedByAHeaderParser(t *testing.T) {
	titles := map[string]string{
		"a Korean title":            "회의록",
		"a space":                   "9월 회의",
		"parentheses":               "2026년 계획(초안)",
		"a semicolon and an equals": "구분;키=값",
		"a comma, colon and at":     "쉼표, 콜론: 골뱅이@사내",
		"a percent sign":            "달성률 100%",
		"a double quote":            `그건 "좋아"`,
		"a question mark and hash":  "왜?#1",
		"square brackets":           "[대외비] 보고",
		"an apostrophe":             "don't",
		"a backslash-free ASCII":    "quarterly-report_v2.final",
		"already-escaped-looking":   "%ED%9A%8C 그대로",
		"every attr-char":           "!#$&+-.^_`|~",
	}
	for name, title := range titles {
		t.Run(name, func(t *testing.T) {
			// The shape of export.go's header, down to the ASCII fallback.
			header := fmt.Sprintf(`attachment; filename="document.md"; filename*=UTF-8''%s.md`, extValueEscape(title))
			mediaType, params, err := mime.ParseMediaType(header)
			if err != nil {
				t.Fatalf("mime.ParseMediaType(%q) = %v", header, err)
			}
			if mediaType != "attachment" {
				t.Errorf("media type = %q, want attachment", mediaType)
			}
			if want := title + ".md"; params["filename"] != want {
				t.Errorf("filename read back = %q, want %q (from %q)", params["filename"], want, header)
			}
		})
	}
}

// attr-char is a real set, not a formality: a name made only of characters
// inside it stays legible in the header, which is what anyone reading a request
// log or a proxy trace sees. Escaping more than the rule requires would be
// safe and unreadable, so the boundary is pinned here.
func TestAnExtValueLeavesAttrCharsAlone(t *testing.T) {
	cases := map[string]string{
		"letters and digits": "report2026",
		"the punctuation":    "!#$&+-.^_`|~",
		"mixed":              "muni-document_v1.2",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if got := extValueEscape(in); got != in {
				t.Errorf("extValueEscape(%q) = %q, want it unchanged", in, got)
			}
		})
	}
}

// And everything outside attr-char is escaped, upper-case hex, one byte at a
// time — the form RFC 8187's own example uses.
func TestAnExtValueEscapesEverythingElse(t *testing.T) {
	cases := map[string]struct {
		in   string
		want string
	}{
		"space":        {"a b", "a%20b"},
		"percent":      {"50%", "50%25"},
		"semicolon":    {"a;b", "a%3Bb"},
		"equals":       {"a=b", "a%3Db"},
		"quote":        {`a"b`, "a%22b"},
		"apostrophe":   {"a'b", "a%27b"},
		"star":         {"a*b", "a%2Ab"},
		"one Korean":   {"회", "%ED%9A%8C"},
		"a null byte":  {"a\x00b", "a%00b"},
		"a line break": {"a\nb", "a%0Ab"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := extValueEscape(tc.in); got != tc.want {
				t.Errorf("extValueEscape(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
