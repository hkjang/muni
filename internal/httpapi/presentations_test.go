package httpapi

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/integration/ptium"
	"github.com/hkjang/muni/internal/settings"
)

// fakeRow replays a database row so the mapping can be tested without a
// database.
type fakeRow struct {
	values []any
	err    error
}

func (f fakeRow) Scan(dest ...any) error {
	if f.err != nil {
		return f.err
	}
	for index, target := range dest {
		switch typed := target.(type) {
		case *uuid.UUID:
			*typed = f.values[index].(uuid.UUID)
		case *int:
			*typed = f.values[index].(int)
		case *string:
			*typed = f.values[index].(string)
		case **string:
			*typed = f.values[index].(*string)
		case *time.Time:
			*typed = f.values[index].(time.Time)
		case **time.Time:
			*typed = f.values[index].(*time.Time)
		}
	}
	return nil
}

func presentationRow(linkedRevision, currentRevision int) fakeRow {
	now := time.Now()
	return fakeRow{values: []any{
		uuid.New(), uuid.New(), linkedRevision, "ptium", "pres-1", "사업 추진 계획",
		"completed", 12, (*string)(nil), uuid.New(), now, now, (*time.Time)(nil), currentRevision,
	}}
}

func TestPresentationIsStaleWhenTheDocumentMovedOn(t *testing.T) {
	config := ptium.Config{Enabled: true, BaseURL: "https://ptium.example.com", APIKey: "k"}.Normalize()

	fresh, err := scanPresentation(presentationRow(17, 17), config)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Stale {
		t.Error("a deck built from the current revision is not stale")
	}

	old, err := scanPresentation(presentationRow(17, 22), config)
	if err != nil {
		t.Fatal(err)
	}
	if !old.Stale {
		t.Error("a deck built from an older revision is stale")
	}
	if old.EditorURL != "https://ptium.example.com/presentations/pres-1/editor" {
		t.Errorf("editor link = %q", old.EditorURL)
	}
}

func TestPtiumConfigComesFromSettings(t *testing.T) {
	config := ptiumConfigFrom(settings.Ptium{
		Enabled: true, BaseURL: "https://ptium.example.com/", WebURL: "https://deck.example.com/",
		APIKey: "ptium_key", DefaultTheme: "aurora", DefaultLocale: "ko", TimeoutSeconds: 60,
	})
	if !config.Usable() {
		t.Fatalf("configuration should be usable: %+v", config)
	}
	if config.BaseURL != "https://ptium.example.com" || config.WebURL != "https://deck.example.com" {
		t.Errorf("urls were not normalised: %+v", config)
	}
	if config.TimeoutSeconds != 60 {
		t.Errorf("timeout = %d", config.TimeoutSeconds)
	}
}

func TestPtiumConfigIsUnusableUntilConnected(t *testing.T) {
	for _, item := range []settings.Ptium{
		{Enabled: false, BaseURL: "https://p.example.com", APIKey: "k"},
		{Enabled: true, BaseURL: "https://p.example.com"},
		{Enabled: true, APIKey: "k"},
	} {
		if ptiumConfigFrom(item).Usable() {
			t.Errorf("should not be usable: %+v", item)
		}
	}
}

func TestCreateInputBecomesGeneratorOptions(t *testing.T) {
	input := createPresentationInput{
		Title: "  AI 플랫폼 구축 계획 ", Audience: " 경영진 ", Purpose: "의사결정",
		Tone: "executive", Language: "ko", SlideCount: 10, Minutes: 10, Detail: "간결",
	}
	options := input.options()
	if options.Title != "AI 플랫폼 구축 계획" || options.Audience != "경영진" {
		t.Fatalf("values were not trimmed: %+v", options)
	}
	if options.SlideCount != 10 || options.Minutes != 10 {
		t.Fatalf("options lost: %+v", options)
	}
}
