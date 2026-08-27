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

// An on/off property is the element, not an attribute of it: <w:b/> means bold
// and <w:b w:val="0"/> means not bold. Merging the style's w:val into the
// run's bare element turned the author's bold off.
func TestARunsOwnSwitchIsNotFlippedByItsStyle(t *testing.T) {
	styles := `<w:style w:type="character" w:styleId="NoBold"><w:name w:val="굵기끔"/>` +
		`<w:rPr><w:b w:val="0"/></w:rPr></w:style>`
	body := `<w:p><w:r><w:rPr><w:rStyle w:val="NoBold"/><w:b/></w:rPr><w:t>굵게쓴글자</w:t></w:r></w:p>` +
		`<w:p><w:r><w:rPr><w:rStyle w:val="NoBold"/></w:rPr><w:t>스타일만글자</w:t></w:r></w:p>`
	document, _, _, err := Parse(wordPackageWithStyles(t, body, styles))
	if err != nil {
		t.Fatal(err)
	}
	if marks := markedText(t, document, "굵게쓴글자"); !has(marks, "bold") {
		t.Errorf("쓴 사람이 준 굵기를 스타일이 껐습니다: %v", marks)
	}
	// And where the run says nothing, the style still speaks.
	if marks := markedText(t, document, "스타일만글자"); has(marks, "bold") {
		t.Errorf("스타일이 끈 굵기가 살아났습니다: %v", marks)
	}
}

// A run that named a font has chosen one. Letting the style fill in the other
// half of w:rFonts changes which font is read: a run saying only 바탕체, under
// a style saying only Cambria, came out Cambria.
func TestAKoreanRunKeepsItsKoreanFont(t *testing.T) {
	styles := `<w:style w:type="character" w:styleId="Latin"><w:name w:val="라틴"/>` +
		`<w:rPr><w:rFonts w:ascii="Cambria"/></w:rPr></w:style>`
	body := `<w:p><w:r><w:rPr><w:rStyle w:val="Latin"/><w:rFonts w:eastAsia="바탕체"/></w:rPr>` +
		`<w:t>한글글자</w:t></w:r></w:p>`
	document, _, _, err := Parse(wordPackageWithStyles(t, body, styles))
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := marshalDocument(t, document)
	if !contains(encoded, "바탕체") {
		t.Errorf("한글 글꼴이 사라졌습니다: %s", encoded)
	}
	if contains(encoded, "Cambria") {
		t.Errorf("런이 요청하지 않은 라틴 글꼴이 붙었습니다: %s", encoded)
	}
}

// A run that names no font at all — a bare w:hint="eastAsia", which Korean
// files write constantly — still takes the style's.
func TestARunThatNamesNoFontTakesTheStyles(t *testing.T) {
	styles := `<w:style w:type="character" w:styleId="Named"><w:name w:val="이름"/>` +
		`<w:rPr><w:rFonts w:ascii="맑은 고딕"/></w:rPr></w:style>`
	body := `<w:p><w:r><w:rPr><w:rStyle w:val="Named"/><w:rFonts w:hint="eastAsia"/></w:rPr>` +
		`<w:t>이름붙은글자</w:t></w:r></w:p>`
	document, _, _, err := Parse(wordPackageWithStyles(t, body, styles))
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := marshalDocument(t, document)
	if !contains(encoded, "맑은 고딕") {
		t.Errorf("스타일의 글꼴이 오지 않았습니다: %s", encoded)
	}
}

// And the font a run names still decides whether it is code. Preferring the
// Hangul half would have read D2Coding as 맑은 고딕 and lost the code mark.
func TestACodeRunIsStillCode(t *testing.T) {
	body := `<w:p><w:r><w:rPr><w:rFonts w:ascii="D2Coding" w:eastAsia="맑은 고딕"/></w:rPr>` +
		`<w:t>코드글자</w:t></w:r></w:p>`
	document, _, _, err := Parse(wordPackageWithStyles(t, body, ""))
	if err != nil {
		t.Fatal(err)
	}
	if marks := markedText(t, document, "코드글자"); !has(marks, "code") {
		t.Errorf("코드 표시가 사라졌습니다: %v", marks)
	}
}

// Word emits w:cs routinely, and muni never reads it. A run naming only that
// has not chosen the font muni will show, so treating it as a choice discarded
// the style's — and with it the code mark that font decides.
func TestARunNamingOnlyAFontMuniIgnoresStillTakesTheStyles(t *testing.T) {
	styles := `<w:style w:type="character" w:styleId="코드"><w:name w:val="코드"/>` +
		`<w:rPr><w:rFonts w:ascii="D2Coding"/></w:rPr></w:style>`
	body := `<w:p><w:r><w:rPr><w:rStyle w:val="코드"/><w:rFonts w:hint="eastAsia" w:cs="Times New Roman"/></w:rPr>` +
		`<w:t>코드글자</w:t></w:r></w:p>`
	document, _, _, err := Parse(wordPackageWithStyles(t, body, styles))
	if err != nil {
		t.Fatal(err)
	}
	if marks := markedText(t, document, "코드글자"); !has(marks, "code") {
		t.Errorf("스타일의 글꼴과 코드 표시가 사라졌습니다: %v", marks)
	}
}
