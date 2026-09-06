package docx

import (
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

// pngPixel is a 1x1 image: enough for the writer to measure, small enough to
// keep the fixture readable.
var pngPixel = []byte{
	0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0a, 'I', 'D', 'A', 'T', 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 'I', 'E', 'N', 'D', 0xae,
	0x42, 0x60, 0x82,
}

// A picture is drawn into a box of the document's choosing, and Word writes
// both halves of that box in <wp:extent>. The reader kept the width and threw
// the height away, so a picture squashed to fit a line came back as a square
// pixel stretched to 163 across — the writer had always read the two as a
// pair, and only one of them survived a round trip.
func TestDrawnPictureHeightSurvivesARoundTrip(t *testing.T) {
	image := &richdoc.Node{Type: "image"}
	image.SetAttr("src", "/api/v1/attachments/abc")
	image.SetAttr("width", 163)
	image.SetAttr("height", 40)
	data := build(t, richdoc.Doc(image), Options{Title: "그림", ResolveImage: func(string) (Image, bool) {
		return Image{Data: pngPixel, MediaType: "image/png"}, true
	}})
	imported, _, _, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	picture := firstImage(imported)
	if picture == nil {
		t.Fatal("그림이 사라졌습니다")
	}
	if width := picture.AttrInt("width", 0); width != 163 {
		t.Errorf("그린 너비가 %d 입니다, 163 이어야 합니다", width)
	}
	if height := picture.AttrInt("height", 0); height != 40 {
		t.Errorf("그린 높이가 %d 입니다, 40 이어야 합니다", height)
	}
}

// A picture whose box is the shape of its bytes has a height too, and reading
// it costs nothing — but a picture with no extent at all must not gain one.
func TestAPictureWithoutAnExtentHasNoSize(t *testing.T) {
	image := &richdoc.Node{Type: "image"}
	image.SetAttr("src", "/api/v1/attachments/abc")
	data := build(t, richdoc.Doc(image), Options{Title: "그림", ResolveImage: func(string) (Image, bool) {
		return Image{Data: pngPixel, MediaType: "image/png"}, true
	}})
	imported, _, _, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	picture := firstImage(imported)
	if picture == nil {
		t.Fatal("그림이 사라졌습니다")
	}
	// The writer measures the bytes when the node says nothing, so the sizes
	// that come back are the pixel's own — square.
	if width, height := picture.AttrInt("width", 0), picture.AttrInt("height", 0); width != height {
		t.Errorf("1x1 그림이 %dx%d 로 들어왔습니다", width, height)
	}
}

func firstImage(node *richdoc.Node) *richdoc.Node {
	if node == nil {
		return nil
	}
	if node.Type == "image" {
		return node
	}
	for _, child := range node.Content {
		if found := firstImage(child); found != nil {
			return found
		}
	}
	return nil
}
