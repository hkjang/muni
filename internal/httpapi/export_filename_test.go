package httpapi

import (
	"strings"
	"testing"
)

// A document title is allowed 240 characters by the API, and safeFilename cuts
// it to 100 before it becomes a download name or an archive entry name. What
// the cut leaves behind is what these pin down: a name, and nothing else. The
// cut used to borrow the AI context helper, which appends a sentence saying the
// text was shortened — perfectly right in a prompt, and wrong in a file name,
// where it arrived as a newline followed by Korean prose.
func TestALongTitleIsCutToANameAndNothingElse(t *testing.T) {
	long := strings.Repeat("가", 120)
	got := safeFilename(long)

	if want := strings.Repeat("가", 100); got != want {
		t.Errorf("safeFilename(120 runes) = %q, want the first 100 runes", got)
	}
	if strings.Contains(got, "생략됨") {
		t.Errorf("safeFilename left a context notice in the name: %q", got)
	}
	if strings.ContainsAny(got, "\r\n") {
		t.Errorf("safeFilename left a line break in the name: %q", got)
	}
}

// Everything the five callers of safeFilename already relied on — the download
// name, the presentation name, the archive's own file name, its entry names and
// its folder segments — has to come out byte for byte as it did.
func TestSafeFilenameLeavesShorterTitlesAlone(t *testing.T) {
	cases := map[string]struct {
		in   string
		want string
	}{
		"plain title":         {"2026년 3분기 개편안", "2026년 3분기 개편안"},
		"exactly the limit":   {strings.Repeat("나", 100), strings.Repeat("나", 100)},
		"one under the limit": {strings.Repeat("나", 99), strings.Repeat("나", 99)},
		"one over the limit":  {strings.Repeat("나", 101), strings.Repeat("나", 100)},
		"empty":               {"", "muni-document"},
		"only spaces":         {"   ", "muni-document"},
		"separators":          {"계획/2026\\하반기", "계획-2026-하반기"},
		"line breaks":         {"제목\r\n둘째 줄", "제목  둘째 줄"},
		"a null byte":         {"제목\x00", "제목"},
		"surrounding spaces":  {"  제목  ", "제목"},
		"a dot":               {".", "."},
		"two dots":            {"..", ".."},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := safeFilename(tc.in); got != tc.want {
				t.Errorf("safeFilename(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// The cut lands wherever the 100th rune happens to be, which can be in the
// middle of a phrase — and a name that ends in a space is one an unpacker or a
// file system may quietly rewrite, so the cut takes the trailing space with it.
func TestACutThatLandsOnASpaceDoesNotEndTheNameWithOne(t *testing.T) {
	// 98 runes, then a space, then more: the cut falls two runes past the space.
	title := strings.Repeat("다", 98) + "  뒷부분은 잘린다"
	got := safeFilename(title)
	if got != strings.Repeat("다", 98) {
		t.Errorf("safeFilename = %q, want the 98 runes before the spaces", got)
	}
}
