package hwpx

import (
	"strings"
	"testing"
)

// A hyperlink in HWPX is a field: a begin control, the words, an end
// control, the address in the begin's Command parameter with its colon
// escaped. The shape is what Hangul writes, checked against a second
// implementation's output.
func TestAHyperlinkFieldIsReadAsALink(t *testing.T) {
	document := parseFile(t, `<hp:p paraPrIDRef="0" styleIDRef="0">`+
		`<hp:run charPrIDRef="0"><hp:t>앞 </hp:t></hp:run>`+
		`<hp:run charPrIDRef="0"><hp:ctrl><hp:fieldBegin id="7" type="HYPERLINK" name="" editable="0" dirty="1"><hp:parameters cnt="1" name=""><hp:stringParam name="Command">http\://www.hancom.co.kr;1;0;0;</hp:stringParam></hp:parameters></hp:fieldBegin></hp:ctrl></hp:run>`+
		`<hp:run charPrIDRef="0"><hp:t>한컴</hp:t></hp:run>`+
		`<hp:run charPrIDRef="0"><hp:ctrl><hp:fieldEnd beginIDRef="7"/></hp:ctrl></hp:run>`+
		`<hp:run charPrIDRef="0"><hp:t> 뒤</hp:t></hp:run></hp:p>`, nil)
	if marks := markedText(t, document, "한컴"); !has(marks, "link") {
		t.Errorf("한컴에 링크가 없습니다: %v", marks)
	}
	if marks := markedText(t, document, "앞 "); has(marks, "link") {
		t.Errorf("링크 밖의 글에 링크가 붙었습니다: %v", marks)
	}
}

func TestALinkIsWrittenAsAFieldAndComesBack(t *testing.T) {
	document := everyNode(t)
	back := roundTrip(t, document)
	phrase := ""
	var find func(n interface{})
	_ = find
	for _, phraseTry := range []string{"링크", "link", "예시"} {
		if hasMarkOn(document, phraseTry, "link") {
			phrase = phraseTry
		}
	}
	if phrase == "" {
		t.Skip("every-node 에 링크 글이 없습니다")
	}
	if !hasMarkOn(back, phrase, "link") {
		t.Errorf("%q의 링크가 왕복에서 사라졌습니다", phrase)
	}
	built, _ := Build(document, Options{Title: "링크"})
	body := partsOf(t, built)["Contents/section0.xml"]
	if !strings.Contains(body, `type="HYPERLINK"`) || !strings.Contains(body, `<hp:stringParam name="Command">https\://`) || !strings.Contains(body, `<hp:fieldEnd beginIDRef="1"/>`) {
		t.Errorf("링크가 한글의 필드 모양으로 쓰이지 않았습니다")
	}
}
