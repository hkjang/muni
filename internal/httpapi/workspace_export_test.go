package httpapi

import (
	"path"
	"strings"
	"testing"
)

func TestAFolderNamedDotOrDotDotIsNotAPathElement(t *testing.T) {
	// A folder may be called anything the name check lets through, and `.` and
	// `..` mean something to every unpacking tool. Everything else has to come
	// out of the helper byte for byte as it went in.
	cases := []struct {
		name string
		want string
	}{
		{"2026", "2026"},
		{"회의 자료", "회의 자료"},
		{"...", "..."},
		{"..보관", "..보관"},
		{"a/b", "a-b"},
		{"a/..", "a-.."},  // the slash already made it something else
		{"./..", ".-.."},  // and so did this one
		{"\\..", "-.."},   // a backslash goes the same way
		{".", "_."},       // the two that do not
		{"..", "_.."},     //
		{"  ..  ", "_.."}, // surrounding space is trimmed before the check
		{"", "muni-document"},
	}
	for _, c := range cases {
		if got := safeFolderSegment(c.name); got != c.want {
			t.Errorf("safeFolderSegment(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestAFolderNameCannotWalkOutOfTheArchive(t *testing.T) {
	// The segment is joined onto a prefix, and path.Join cleans as it goes: a
	// `..` element climbs out of the archive root, and a `.` element folds the
	// folder into its parent. Neither may survive the helper.
	names := []string{".", "..", "  ..  ", "./..", "a/..", "...", "보고서"}
	for _, name := range names {
		segment := safeFolderSegment(name)
		for _, prefix := range []string{"", "휴지통", "2026"} {
			joined := path.Join(prefix, segment)
			for _, element := range strings.Split(joined, "/") {
				if element == ".." || element == "." {
					t.Errorf("path.Join(%q, safeFolderSegment(%q)) = %q escapes the archive", prefix, name, joined)
				}
			}
			// A document under a folder in the trash stays in the trash.
			if prefix == "휴지통" && !strings.HasPrefix(joined, "휴지통/") {
				t.Errorf("trashed folder %q landed at %q, outside 휴지통", name, joined)
			}
			// Two different folders never become one directory.
			if prefix == "" && joined == "" {
				t.Errorf("folder %q collapsed onto the archive root", name)
			}
		}
	}
}

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
