package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAGeneratedPasswordAvoidsCharactersPeopleMisread(t *testing.T) {
	// It gets read off one screen and typed on another, sometimes read aloud.
	// O/0 and l/1/I are where that goes wrong.
	for i := 0; i < 50; i++ {
		password, err := generatePassword()
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{"O", "0", "l", "1", "I", "o"} {
			if strings.Contains(password, bad) {
				t.Fatalf("password %q contains the confusable %q", password, bad)
			}
		}
		if err := checkPassword(password); err != nil {
			// A generated password that the policy rejects would make the
			// account impossible to hand over.
			t.Fatalf("policy rejected a generated password %q: %v", password, err)
		}
	}
}

func TestTwoGeneratedPasswordsDiffer(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		p, err := generatePassword()
		if err != nil {
			t.Fatal(err)
		}
		if seen[p] {
			t.Fatalf("generated %q twice", p)
		}
		seen[p] = true
	}
}

func TestATemporaryPasswordLocksTheAccountToChangingIt(t *testing.T) {
	// The account was handed out over email, so the browser is not the only
	// thing that could ignore the prompt. The list has to be short and it has
	// to include signing out.
	allowed := []string{"/api/v1/auth/me", "/api/v1/me/password", "/api/v1/auth/logout"}
	for _, path := range allowed {
		if !allowedBeforePasswordChange(httptest.NewRequest("POST", path, nil)) {
			t.Errorf("%s must stay reachable", path)
		}
	}
	for _, path := range []string{
		"/api/v1/documents", "/api/v1/workspaces", "/api/v1/admin/users",
		"/api/v1/ai/chat", "/mcp", "/api/v1/me/password/other", "/api/v1/me/keys",
	} {
		if allowedBeforePasswordChange(httptest.NewRequest("GET", path, nil)) {
			t.Errorf("%s must be blocked until the password is replaced", path)
		}
	}
}

func TestTheCSVTakesTheFileAPersonActuallyHas(t *testing.T) {
	// Columns in whatever order, Korean headers, a UTF-8 BOM from Excel, and a
	// trailing blank line.
	csv := "\ufeff이름,email,역할\n" +
		"김민수,minsu@example.com,ADMIN\n" +
		"이서연,seoyeon@example.com,\n" +
		"\n"
	rows, err := readUserCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 (a blank final line is not a row)", len(rows))
	}
	if rows[0].DisplayName != "김민수" || rows[0].Email != "minsu@example.com" || rows[0].Role != "ADMIN" {
		t.Fatalf("first row = %+v", rows[0])
	}
	if rows[1].Role != "USER" {
		t.Fatalf("an empty role must default to USER, got %q", rows[1].Role)
	}
	// The line numbers are what the administrator sees in the failure report,
	// so they must match the spreadsheet, header included.
	if rows[0].Line != 2 || rows[1].Line != 3 {
		t.Fatalf("lines = %d, %d; want 2, 3", rows[0].Line, rows[1].Line)
	}
}

func TestACSVWithoutAnEmailColumnIsRejectedUpFront(t *testing.T) {
	// Better than importing zero rows and reporting success.
	if _, err := readUserCSV(strings.NewReader("name,role\n김민수,USER\n")); err == nil {
		t.Fatal("a file with no email column must be refused")
	}
}

func TestARowWithNoEmailIsSkippedNotGuessed(t *testing.T) {
	rows, err := readUserCSV(strings.NewReader("email,name\n,이름만\nreal@example.com,진짜\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Email != "real@example.com" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestEmailIsLowercasedSoTheDuplicateCheckWorks(t *testing.T) {
	rows, err := readUserCSV(strings.NewReader("email\nMinsu@Example.COM\n"))
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Email != "minsu@example.com" {
		t.Fatalf("email = %q", rows[0].Email)
	}
}
