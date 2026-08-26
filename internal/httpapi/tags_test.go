package httpapi

import (
	"strings"
	"testing"
)

func TestTagNamesAreTrimmedAndDeduplicated(t *testing.T) {
	got := normalizeTagNames([]string{" 상반기 ", "상반기", "대외비", ""})
	if len(got) != 2 || got[0] != "상반기" || got[1] != "대외비" {
		t.Fatalf("tags = %#v", got)
	}
}

func TestTagNamesFoldCase(t *testing.T) {
	// The column is citext, so "Draft" and "draft" are the same tag; sending
	// both must not try to create it twice.
	got := normalizeTagNames([]string{"Draft", "draft", "DRAFT"})
	if len(got) != 1 {
		t.Fatalf("tags = %#v", got)
	}
}

func TestATagIsALabelNotASentence(t *testing.T) {
	long := strings.Repeat("가", 50)
	if got := normalizeTagNames([]string{long, "짧은 태그"}); len(got) != 1 || got[0] != "짧은 태그" {
		t.Fatalf("an over-long tag should be dropped: %#v", got)
	}
}

func TestNoTagsIsNotAnError(t *testing.T) {
	if got := normalizeTagNames(nil); len(got) != 0 {
		t.Fatalf("tags = %#v", got)
	}
	if got := normalizeTagNames([]string{"  ", "\t"}); len(got) != 0 {
		t.Fatalf("whitespace is not a tag: %#v", got)
	}
}
