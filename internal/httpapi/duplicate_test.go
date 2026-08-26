package httpapi

import "testing"

func TestACopyIsNamedAsOne(t *testing.T) {
	if got := copyTitle("2026년 상반기 보고서"); got != "2026년 상반기 보고서 (사본)" {
		t.Fatalf("title = %q", got)
	}
}

func TestCopyingACopyDoesNotStack(t *testing.T) {
	// "사본의 사본의 사본" is what happens when the marker is appended blindly.
	if got := copyTitle("보고서 (사본)"); got != "보고서 (사본) 2" {
		t.Fatalf("title = %q", got)
	}
}

func TestAnUntitledDocumentStillGetsAName(t *testing.T) {
	if got := copyTitle("   "); got != "제목 없는 문서 (사본)" {
		t.Fatalf("title = %q", got)
	}
}

func TestSurroundingSpaceIsNotPartOfTheName(t *testing.T) {
	if got := copyTitle("  보고서  "); got != "보고서 (사본)" {
		t.Fatalf("title = %q", got)
	}
}
