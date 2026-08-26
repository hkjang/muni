package ptium

import "testing"

func TestEditorURLPointsAtTheEditorRoute(t *testing.T) {
	// Ptium routes on the exact path /presentations/{id}/editor and has no
	// route for the deck on its own, so a link without the suffix lands on the
	// not-found page.
	config := Config{WebURL: "https://ptium.example.com"}.Normalize()
	if got := config.EditorURL("pres-1"); got != "https://ptium.example.com/presentations/pres-1/editor" {
		t.Fatalf("editor link = %q", got)
	}
}

func TestEditorURLEscapesTheIdentifier(t *testing.T) {
	config := Config{WebURL: "https://ptium.example.com"}.Normalize()
	if got := config.EditorURL("a b/c"); got != "https://ptium.example.com/presentations/a%20b%2Fc/editor" {
		t.Fatalf("editor link = %q", got)
	}
}

func TestEditorURLIsEmptyWithoutSomewhereToPoint(t *testing.T) {
	if got := (Config{}).EditorURL("pres-1"); got != "" {
		t.Fatalf("editor link = %q", got)
	}
	if got := (Config{WebURL: "https://ptium.example.com"}).EditorURL(""); got != "" {
		t.Fatalf("editor link = %q", got)
	}
}
