package docx

import (
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

// An office template keeps the shape of its body text in a style: 들여쓰기 and
// 줄간격 are set once in "본문" and every paragraph just names it. muni read the
// paragraph alone, so a document written the way templates are written arrived
// with none of its layout.

const bodyStyles = `<w:style w:type="paragraph" w:styleId="Body"><w:name w:val="본문"/>` +
	`<w:pPr><w:ind w:left="800" w:firstLine="200"/><w:spacing w:line="360" w:lineRule="auto"/><w:jc w:val="both"/></w:pPr>` +
	`</w:style>` +
	`<w:style w:type="paragraph" w:styleId="BodyWide"><w:name w:val="본문넓게"/><w:basedOn w:val="Body"/>` +
	`<w:pPr><w:ind w:left="1600"/></w:pPr></w:style>`

func styledParagraph(styleID, text string, extra string) string {
	return `<w:p><w:pPr><w:pStyle w:val="` + styleID + `"/>` + extra + `</w:pPr>` +
		`<w:r><w:t>` + escapeXML(text) + `</w:t></w:r></w:p>`
}

func paragraphCarrying(t *testing.T, document *richdoc.Node, phrase string) *richdoc.Node {
	t.Helper()
	for _, block := range document.Content {
		if block.PlainText() == phrase {
			return block
		}
	}
	t.Fatalf("%q 를 담은 문단이 없습니다", phrase)
	return nil
}

func TestAParagraphStyleBringsItsLayout(t *testing.T) {
	body := styledParagraph("Body", "스타일문단", "") + `<w:p><w:r><w:t>보통문단</w:t></w:r></w:p>`
	document, _, _, err := Parse(wordPackageWithStyles(t, body, bodyStyles))
	if err != nil {
		t.Fatal(err)
	}
	styled := paragraphCarrying(t, document, "스타일문단")
	if styled.AttrInt("indent", 0) == 0 {
		t.Errorf("들여쓰기가 오지 않았습니다: %v", styled.Attrs)
	}
	if styled.AttrString("lineHeight") != "1.5" {
		t.Errorf("줄간격 = %q", styled.AttrString("lineHeight"))
	}
	if styled.AttrString("textAlign") != "justify" {
		t.Errorf("정렬 = %q", styled.AttrString("textAlign"))
	}
	// A paragraph naming no style is what it always was.
	if plain := paragraphCarrying(t, document, "보통문단"); len(plain.Attrs) != 0 {
		t.Errorf("스타일 없는 문단이 서식을 얻었습니다: %v", plain.Attrs)
	}
}

// The paragraph wins. Someone who centred this one line meant this one line.
func TestTheParagraphOverridesItsStyle(t *testing.T) {
	body := styledParagraph("Body", "가운데문단", `<w:jc w:val="center"/>`)
	document, _, _, err := Parse(wordPackageWithStyles(t, body, bodyStyles))
	if err != nil {
		t.Fatal(err)
	}
	styled := paragraphCarrying(t, document, "가운데문단")
	if got := styled.AttrString("textAlign"); got != "center" {
		t.Errorf("정렬 = %q, 문단이 스타일을 이기지 못했습니다", got)
	}
	// What the paragraph did not say still comes from the style.
	if styled.AttrString("lineHeight") != "1.5" {
		t.Errorf("줄간격 = %q", styled.AttrString("lineHeight"))
	}
}

// A style built on another takes what it overrides from itself and the rest
// from underneath.
func TestAParagraphStyleBuiltOnAnotherTakesBoth(t *testing.T) {
	document, _, _, err := Parse(wordPackageWithStyles(t, styledParagraph("BodyWide", "넓은문단", ""), bodyStyles))
	if err != nil {
		t.Fatal(err)
	}
	wide := paragraphCarrying(t, document, "넓은문단")
	if wide.AttrString("lineHeight") != "1.5" {
		t.Errorf("basedOn 의 줄간격이 오지 않았습니다: %v", wide.Attrs)
	}
	if indent := wide.AttrInt("indent", 0); indent < 4 {
		t.Errorf("들여쓰기 = %d, 스스로 정한 값을 기대했습니다", indent)
	}
}

// Numbering keeps its own path: a style that makes a list still makes a list,
// and merging must not turn one into two.
func TestAStyledListIsStillOneList(t *testing.T) {
	styles := `<w:style w:type="paragraph" w:styleId="ListBullet"><w:name w:val="List Bullet"/>` +
		`<w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="3"/></w:numPr></w:pPr></w:style>`
	body := styledParagraph("ListBullet", "항목하나", "") + styledParagraph("ListBullet", "항목둘", "")
	document, _, _, err := Parse(wordPackageWithStyles(t, body, styles))
	if err != nil {
		t.Fatal(err)
	}
	lists := 0
	for _, block := range document.Content {
		if block.Type == "bulletList" {
			lists++
			if len(block.Content) != 2 {
				t.Errorf("목록 항목 = %d개", len(block.Content))
			}
		}
	}
	if lists != 1 {
		t.Errorf("목록 = %d개, 하나를 기대했습니다: %v", lists, blockTypes(document))
	}
}

// A style naming itself must not spin.
func TestACircularParagraphStyleDoesNotSpin(t *testing.T) {
	styles := `<w:style w:type="paragraph" w:styleId="A"><w:basedOn w:val="B"/><w:pPr><w:jc w:val="center"/></w:pPr></w:style>` +
		`<w:style w:type="paragraph" w:styleId="B"><w:basedOn w:val="A"/><w:pPr><w:ind w:left="800"/></w:pPr></w:style>`
	document, _, _, err := Parse(wordPackageWithStyles(t, styledParagraph("A", "빙글문단", ""), styles))
	if err != nil {
		t.Fatal(err)
	}
	if got := paragraphCarrying(t, document, "빙글문단").AttrString("textAlign"); got != "center" {
		t.Errorf("정렬 = %q", got)
	}
}
