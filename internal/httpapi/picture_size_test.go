package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
)

const sizedPicturePixel = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

// A picture drawn in a shape that is not the shape of its bytes — a 한글
// header stamp squashed onto one line — carries a height beside its width. The
// HTML the browser and the PDF are both built from wrote only the width, and
// the stylesheet's height:auto then stretched the picture back to the shape of
// its bytes. The ratio has to travel with it.
func TestRenderedPictureKeepsTheShapeItWasDrawnIn(t *testing.T) {
	out := renderHTML(json.RawMessage(`{"type":"doc","content":[
		{"type":"image","attrs":{"src":"/api/v1/attachments/abc","alt":"직인","width":163,"height":40}}]}`))
	for _, want := range []string{`width="163"`, `height="40"`, `aspect-ratio:163/40`} {
		if !strings.Contains(out, want) {
			t.Errorf("%s 가 없습니다: %s", want, out)
		}
	}
}

// A picture nobody resized says nothing, and an attribute of its own would be
// a guess about bytes the renderer never reads.
func TestRenderedPictureWithoutASizeSaysNothing(t *testing.T) {
	out := renderHTML(json.RawMessage(`{"type":"doc","content":[
		{"type":"image","attrs":{"src":"/api/v1/attachments/abc","alt":"직인"}}]}`))
	if strings.Contains(out, "height=") || strings.Contains(out, "aspect-ratio") {
		t.Errorf("크기가 없는 그림에 크기가 붙었습니다: %s", out)
	}
}

// muni exports HTML and imports it back — a workspace archive is read the same
// way it was written — so a size the writer puts on the tag has to come home.
func TestImportedPictureKeepsItsDrawnSize(t *testing.T) {
	content, _, err := htmlDocument([]byte(`<body><p><img src="` + sizedPicturePixel + `" width="163" height="40"></p></body>`))
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(content)
	for _, want := range []string{`"width":163`, `"height":40`} {
		if !strings.Contains(encoded, want) {
			t.Errorf("%s 가 없습니다: %s", want, encoded)
		}
	}
}

// A height with no width beside it is not a shape, and a tag pasted from
// elsewhere often carries the unit.
func TestImportedPictureSizeIgnoresAHeightAlone(t *testing.T) {
	content, _, err := htmlDocument([]byte(`<body><p><img src="` + sizedPicturePixel + `" height="40"></p>` +
		`<p><img src="` + sizedPicturePixel + `" width="163px" height="40px"></p></body>`))
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(content)
	if strings.Count(encoded, `"height":40`) != 1 {
		t.Errorf("높이가 한 번만 있어야 합니다: %s", encoded)
	}
	if !strings.Contains(encoded, `"width":163`) {
		t.Errorf("px 가 붙은 너비를 읽지 못했습니다: %s", encoded)
	}
}
