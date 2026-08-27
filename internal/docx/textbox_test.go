package docx

import (
	"strings"
	"testing"
)

// A Korean office document keeps things in text boxes that the page cannot do
// without: the 붙임 label, the stamp beside a signature, a note in the margin.
// muni has no box to put them in, and the drawing they arrive in is not a
// picture — so muni found no image, kept nothing, and left an empty paragraph
// where the words had been.

// wordTextBox writes the shape Word writes: the same box offered twice, once
// for readers that understand the modern shape extension and once for the rest.
func wordTextBox(paragraphs ...string) string {
	choice, fallback := "", ""
	for _, text := range paragraphs {
		choice += `<w:p><w:r><w:t>` + escapeXML(text) + `</w:t></w:r></w:p>`
		fallback += `<w:p><w:r><w:t>` + escapeXML(text) + `</w:t></w:r></w:p>`
	}
	return `<w:r><mc:AlternateContent>` +
		`<mc:Choice Requires="wps"><w:drawing><wp:inline><a:graphic><a:graphicData>` +
		`<wps:wsp><wps:txbx><w:txbxContent>` + choice + `</w:txbxContent></wps:txbx>` +
		`</wps:wsp></a:graphicData></a:graphic></wp:inline></w:drawing></mc:Choice>` +
		`<mc:Fallback><w:pict><v:shape><v:textbox><w:txbxContent>` + fallback +
		`</w:txbxContent></v:textbox></v:shape></w:pict></mc:Fallback>` +
		`</mc:AlternateContent></w:r>`
}

func importedText(t *testing.T, body string) string {
	t.Helper()
	document, _, _, err := Parse(wordPackage(t, body))
	if err != nil {
		t.Fatal(err)
	}
	return document.PlainText()
}

func TestATextBoxKeepsItsWords(t *testing.T) {
	body := `<w:p><w:r><w:t>본문앞</w:t></w:r></w:p>` +
		`<w:p>` + wordTextBox("붙임 제1호") + `</w:p>` +
		`<w:p><w:r><w:t>본문뒤</w:t></w:r></w:p>`
	text := importedText(t, body)
	for _, want := range []string{"본문앞", "붙임 제1호", "본문뒤"} {
		if !strings.Contains(text, want) {
			t.Errorf("%q 가 사라졌습니다: %q", want, text)
		}
	}
}

// The box is offered twice over. Reading both branches would put every stamp
// in the document twice.
func TestATextBoxIsReadOnce(t *testing.T) {
	text := importedText(t, `<w:p>`+wordTextBox("직인")+`</w:p>`)
	if count := strings.Count(text, "직인"); count != 1 {
		t.Errorf("직인이 %d번 나옵니다: %q", count, text)
	}
}

// A two-line stamp read as one line is a different stamp.
func TestATextBoxKeepsItsLines(t *testing.T) {
	document, _, _, err := Parse(wordPackage(t, `<w:p>`+wordTextBox("기획조정실", "제2026-1호")+`</w:p>`))
	if err != nil {
		t.Fatal(err)
	}
	breaks := 0
	for _, block := range document.Content {
		for _, child := range block.Content {
			if child.Type == "hardBreak" {
				breaks++
			}
		}
	}
	if breaks != 1 {
		t.Errorf("줄바꿈 = %d개: %s", breaks, document.PlainText())
	}
}

// A drawing that really is a picture is still a picture.
func TestAPictureIsStillAPicture(t *testing.T) {
	body := `<w:p><w:r><w:pict><v:shape><v:imagedata r:id="rIdX"/></v:shape></w:pict></w:r></w:p>`
	document, _, _, err := Parse(wordPackage(t, body))
	if err != nil {
		t.Fatal(err)
	}
	// The relationship is missing, so there is nothing to keep — what matters
	// is that the picture path ran rather than the text-box one.
	if text := document.PlainText(); strings.TrimSpace(text) != "" {
		t.Errorf("그림에서 글자가 나왔습니다: %q", text)
	}
}

// Some producers write <mc:Fallback/> empty. Preferring nothing over the
// Choice throws away the only copy of the shape's words.
func TestAnEmptyFallbackFallsThroughToTheChoice(t *testing.T) {
	body := `<w:p><w:r><mc:AlternateContent>` +
		`<mc:Choice Requires="wps"><w:drawing><wps:wsp><wps:txbx><w:txbxContent>` +
		`<w:p><w:r><w:t>선택지 안 글자</w:t></w:r></w:p>` +
		`</w:txbxContent></wps:txbx></wps:wsp></w:drawing></mc:Choice>` +
		`<mc:Fallback/></mc:AlternateContent></w:r></w:p>`
	if text := importedText(t, body); !strings.Contains(text, "선택지 안 글자") {
		t.Errorf("빈 Fallback 때문에 글자가 사라졌습니다: %q", text)
	}
}

// A 직인 grouped with a 붙임 label is one drawing holding two boxes. Keeping
// the first is keeping half the stamp.
func TestEveryBoxInAGroupIsRead(t *testing.T) {
	body := `<w:p><w:r><w:pict><v:group>` +
		`<v:shape><v:textbox><w:txbxContent><w:p><w:r><w:t>붙임 제1호</w:t></w:r></w:p></w:txbxContent></v:textbox></v:shape>` +
		`<v:shape><v:textbox><w:txbxContent><w:p><w:r><w:t>기획조정실장</w:t></w:r></w:p></w:txbxContent></v:textbox></v:shape>` +
		`</v:group></w:pict></w:r></w:p>`
	text := importedText(t, body)
	for _, want := range []string{"붙임 제1호", "기획조정실장"} {
		if !strings.Contains(text, want) {
			t.Errorf("%q 가 사라졌습니다: %q", want, text)
		}
	}
}

// A shape at block level rather than inside a run carries words too.
func TestABlockLevelTextBoxKeepsItsWords(t *testing.T) {
	body := `<w:p><w:r><w:t>본문앞</w:t></w:r></w:p>` +
		`<mc:AlternateContent><mc:Fallback><w:pict><v:shape><v:textbox><w:txbxContent>` +
		`<w:p><w:r><w:t>블록 상자 글자</w:t></w:r></w:p>` +
		`</w:txbxContent></v:textbox></v:shape></w:pict></mc:Fallback></mc:AlternateContent>` +
		`<w:p><w:r><w:t>본문뒤</w:t></w:r></w:p>`
	text := importedText(t, body)
	for _, want := range []string{"본문앞", "블록 상자 글자", "본문뒤"} {
		if !strings.Contains(text, want) {
			t.Errorf("%q 가 사라졌습니다: %q", want, text)
		}
	}
}
