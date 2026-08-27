package docx

import (
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

// Word's "Strong" and "Emphasis" are character styles: the run says only which
// style it wears, and the bold or the italic lives in styles.xml. Text
// formatted through the styles gallery — which is how most templates do it —
// used to arrive as plain text.

const characterStyles = `<w:style w:type="character" w:styleId="Strong"><w:name w:val="Strong"/><w:rPr><w:b/></w:rPr></w:style>` +
	`<w:style w:type="character" w:styleId="Emphasis"><w:name w:val="Emphasis"/><w:rPr><w:i/></w:rPr></w:style>` +
	`<w:style w:type="character" w:styleId="Bureau"><w:name w:val="부서강조"/><w:basedOn w:val="Strong"/>` +
	`<w:rPr><w:color w:val="C00000"/></w:rPr></w:style>` +
	`<w:style w:type="character" w:styleId="Loud"><w:name w:val="Loud"/><w:rPr><w:b/><w:u w:val="single"/></w:rPr></w:style>`

func styledRun(styleID, text string) string {
	return `<w:r><w:rPr><w:rStyle w:val="` + styleID + `"/></w:rPr><w:t>` + escapeXML(text) + `</w:t></w:r>`
}

func markedText(t *testing.T, document *richdoc.Node, phrase string) []string {
	t.Helper()
	var found []string
	var walk func(*richdoc.Node) bool
	walk = func(node *richdoc.Node) bool {
		if node == nil {
			return false
		}
		if node.Type == "text" && strings.Contains(node.Text, phrase) {
			for _, mark := range node.Marks {
				found = append(found, mark.Type)
			}
			return true
		}
		for _, child := range node.Content {
			if walk(child) {
				return true
			}
		}
		return false
	}
	if !walk(document) {
		t.Fatalf("%q 를 찾지 못했습니다", phrase)
	}
	return found
}

func has(marks []string, want string) bool {
	for _, mark := range marks {
		if mark == want {
			return true
		}
	}
	return false
}

func TestACharacterStyleBringsItsFormatting(t *testing.T) {
	body := wordParagraph(styledRun("Strong", "강조된말"), styledRun("Emphasis", "기울인말"))
	document, _, _, err := Parse(wordPackageWithStyles(t, body, characterStyles))
	if err != nil {
		t.Fatal(err)
	}
	if marks := markedText(t, document, "강조된말"); !has(marks, "bold") {
		t.Errorf("Strong 이 굵게 오지 않았습니다: %v", marks)
	}
	if marks := markedText(t, document, "기울인말"); !has(marks, "italic") {
		t.Errorf("Emphasis 가 기울지 않았습니다: %v", marks)
	}
}

// A style built on another gets both, which is how an office template says
// "the bold one, in our red".
func TestAStyleBuiltOnAnotherGetsBoth(t *testing.T) {
	document, _, _, err := Parse(wordPackageWithStyles(t, wordParagraph(styledRun("Bureau", "부서이름")), characterStyles))
	if err != nil {
		t.Fatal(err)
	}
	marks := markedText(t, document, "부서이름")
	if !has(marks, "bold") {
		t.Errorf("basedOn 의 굵게가 오지 않았습니다: %v", marks)
	}
	if !has(marks, "textStyle") {
		t.Errorf("색이 오지 않았습니다: %v", marks)
	}
}

// The run wins. A style is what a run starts from, not what it is: a writer
// who turned the bold off meant it off.
func TestTheRunOverridesItsStyle(t *testing.T) {
	body := wordParagraph(`<w:r><w:rPr><w:rStyle w:val="Loud"/><w:b w:val="0"/></w:rPr><w:t>굵기끈말</w:t></w:r>`)
	document, _, _, err := Parse(wordPackageWithStyles(t, body, characterStyles))
	if err != nil {
		t.Fatal(err)
	}
	marks := markedText(t, document, "굵기끈말")
	if has(marks, "bold") {
		t.Errorf("런에서 끈 굵기가 스타일 때문에 살아났습니다: %v", marks)
	}
	// The rest of the style still applies.
	if !has(marks, "underline") {
		t.Errorf("스타일의 밑줄이 사라졌습니다: %v", marks)
	}
}

// A style naming itself, or a pair naming each other, must not spin.
func TestACircularStyleDoesNotSpin(t *testing.T) {
	styles := `<w:style w:type="character" w:styleId="A"><w:basedOn w:val="B"/><w:rPr><w:b/></w:rPr></w:style>` +
		`<w:style w:type="character" w:styleId="B"><w:basedOn w:val="A"/><w:rPr><w:i/></w:rPr></w:style>`
	document, _, _, err := Parse(wordPackageWithStyles(t, wordParagraph(styledRun("A", "빙글빙글")), styles))
	if err != nil {
		t.Fatal(err)
	}
	if marks := markedText(t, document, "빙글빙글"); !has(marks, "bold") {
		t.Errorf("marks = %v", marks)
	}
}

// A run naming a style the file does not define is the run alone.
func TestAMissingStyleIsNotAnError(t *testing.T) {
	document, _, _, err := Parse(wordPackageWithStyles(t, wordParagraph(styledRun("없는스타일", "그냥말")), characterStyles))
	if err != nil {
		t.Fatal(err)
	}
	if marks := markedText(t, document, "그냥말"); len(marks) != 0 {
		t.Errorf("marks = %v", marks)
	}
}
