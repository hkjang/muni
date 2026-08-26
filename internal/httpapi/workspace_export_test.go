package httpapi

import (
	"strings"
	"testing"
)

func TestTwoDocumentsWithOneTitleBecomeTwoFiles(t *testing.T) {
	// Otherwise the second overwrites the first and the export quietly loses a
	// document.
	used := map[string]bool{}
	first := uniqueEntryName(used, "2026", "회의록", "md")
	second := uniqueEntryName(used, "2026", "회의록", "md")
	third := uniqueEntryName(used, "2026", "회의록", "md")
	if first == second || second == third || first == third {
		t.Fatalf("names collided: %q %q %q", first, second, third)
	}
	if first != "2026/회의록.md" {
		t.Fatalf("first = %q", first)
	}
	if second != "2026/회의록 (2).md" {
		t.Fatalf("second = %q", second)
	}
}

func TestTheSameTitleInDifferentFoldersIsNotAClash(t *testing.T) {
	used := map[string]bool{}
	a := uniqueEntryName(used, "2025", "회의록", "md")
	b := uniqueEntryName(used, "2026", "회의록", "md")
	if a == b {
		t.Fatalf("folders should keep them apart: %q %q", a, b)
	}
	if strings.Contains(b, "(2)") {
		t.Fatalf("a different folder is not a collision: %q", b)
	}
}

func TestADocumentWithNoTitleStillGetsAFile(t *testing.T) {
	used := map[string]bool{}
	name := uniqueEntryName(used, "", "", "md")
	if name != "제목 없는 문서.md" {
		t.Fatalf("name = %q", name)
	}
}

func TestAnEntryAtTheTopHasNoDirectory(t *testing.T) {
	used := map[string]bool{}
	if name := uniqueEntryName(used, "", "보고서", "html"); name != "보고서.html" {
		t.Fatalf("name = %q", name)
	}
}
