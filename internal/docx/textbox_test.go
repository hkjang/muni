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
