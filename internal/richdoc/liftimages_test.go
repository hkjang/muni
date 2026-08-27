package richdoc

import (
	"encoding/json"
	"testing"
)

// The editor's image is a block of its own, so a paragraph holding one is a
// document it will not open. These tests describe the shape every importer
// has to hand it.

func TestLiftImagesSplitsTheSentenceAroundThePicture(t *testing.T) {
	image := &Node{Type: "image"}
	image.SetAttr("src", "/api/v1/attachments/abc")
	paragraph := Paragraph(Text("사진 앞"), image, Text("사진 뒤"))
	doc := Doc(paragraph)

	LiftImages(doc)

	if got := len(doc.Content); got != 3 {
		t.Fatalf("블록 3개를 기대했지만 %d개입니다: %s", got, dump(t, doc))
	}
	if doc.Content[0].Type != "paragraph" || doc.Content[0].PlainText() != "사진 앞" {
		t.Fatalf("앞 문단이 남지 않았습니다: %s", dump(t, doc))
	}
	if doc.Content[1].Type != "image" {
		t.Fatalf("이미지가 블록으로 올라오지 않았습니다: %s", dump(t, doc))
	}
	if doc.Content[2].Type != "paragraph" || doc.Content[2].PlainText() != "사진 뒤" {
		t.Fatalf("뒤 문단이 남지 않았습니다: %s", dump(t, doc))
	}
}

func TestLiftImagesLeavesNoEmptyParagraphBehind(t *testing.T) {
	image := &Node{Type: "image"}
	image.SetAttr("src", "/api/v1/attachments/abc")
	doc := Doc(Paragraph(image))

	LiftImages(doc)

	if len(doc.Content) != 1 || doc.Content[0].Type != "image" {
		t.Fatalf("이미지만 남아야 합니다: %s", dump(t, doc))
	}
}

func TestLiftImagesKeepsTheParagraphsFormatting(t *testing.T) {
	image := &Node{Type: "image"}
	image.SetAttr("src", "/api/v1/attachments/abc")
	paragraph := Paragraph(Text("가운데"), image)
	paragraph.SetAttr("textAlign", "center")
	doc := Doc(paragraph)

	LiftImages(doc)

	if doc.Content[0].AttrString("textAlign") != "center" {
		t.Fatalf("정렬이 사라졌습니다: %s", dump(t, doc))
	}
	// A paragraph that splits in two must not hand both halves the same
	// attribute map, or aligning one of them would align the other.
	doc.Content[0].SetAttr("textAlign", "right")
	if paragraph.AttrString("textAlign") != "center" {
		t.Fatal("나뉜 문단이 원본의 속성 맵을 함께 씁니다")
	}
}

func TestLiftImagesReachesInsideTableCellsAndLists(t *testing.T) {
	cellImage := &Node{Type: "image"}
	cellImage.SetAttr("src", "/api/v1/attachments/cell")
	listImage := &Node{Type: "image"}
	listImage.SetAttr("src", "/api/v1/attachments/list")
	doc := Doc(
		&Node{Type: "table", Content: []*Node{
			{Type: "tableRow", Content: []*Node{
				{Type: "tableCell", Content: []*Node{Paragraph(Text("표"), cellImage)}},
			}},
		}},
		&Node{Type: "bulletList", Content: []*Node{
			{Type: "listItem", Content: []*Node{Paragraph(Text("항목"), listImage)}},
		}},
	)

	LiftImages(doc)

	cell := doc.Content[0].Content[0].Content[0]
	if len(cell.Content) != 2 || cell.Content[1].Type != "image" {
		t.Fatalf("표 안의 이미지가 그대로입니다: %s", dump(t, doc))
	}
	item := doc.Content[1].Content[0]
	if len(item.Content) != 2 || item.Content[1].Type != "image" {
		t.Fatalf("목록 안의 이미지가 그대로입니다: %s", dump(t, doc))
	}
}

func TestLiftImagesLeavesProseAlone(t *testing.T) {
	doc := Doc(Paragraph(Text("사진이 없는 문단")))
	before := dump(t, doc)

	LiftImages(doc)

	if after := dump(t, doc); after != before {
		t.Fatalf("이미지가 없는 문서가 바뀌었습니다:\n전: %s\n후: %s", before, after)
	}
}

func dump(t *testing.T, node *Node) string {
	t.Helper()
	encoded, err := json.Marshal(node)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

// A list item is "paragraph block*". Lifting a picture that was the item's
// only content would leave an image where the opening paragraph has to be,
// and the editor refuses that as flatly as it refuses the shape we started in.
func TestLiftImagesKeepsAListItemOpeningWithAParagraph(t *testing.T) {
	image := &Node{Type: "image"}
	image.SetAttr("src", "/api/v1/attachments/abc")
	doc := Doc(&Node{Type: "bulletList", Content: []*Node{
		{Type: "listItem", Content: []*Node{Paragraph(image)}},
	}})

	LiftImages(doc)

	item := doc.Content[0].Content[0]
	if len(item.Content) != 2 || item.Content[0].Type != "paragraph" || item.Content[1].Type != "image" {
		t.Fatalf("목록 항목이 문단으로 시작하지 않습니다: %s", dump(t, doc))
	}
}

// A heading is one entry in the outline, the contents list and the numbering.
// Splitting it around a picture made two of them where the author wrote one.
func TestLiftImagesDoesNotSplitAHeadingInTwo(t *testing.T) {
	image := &Node{Type: "image"}
	image.SetAttr("src", "/api/v1/attachments/abc")
	heading := &Node{Type: "heading", Content: []*Node{Text("제목 앞"), image, Text("제목 뒤")}}
	heading.SetAttr("level", 2)
	doc := Doc(heading)

	LiftImages(doc)

	headings := 0
	for _, block := range doc.Content {
		if block.Type == "heading" {
			headings++
		}
	}
	if headings != 1 {
		t.Fatalf("제목이 %d개가 되었습니다: %s", headings, dump(t, doc))
	}
	if doc.Content[0].PlainText() != "제목 앞제목 뒤" {
		t.Errorf("제목의 글자가 바뀌었습니다: %s", dump(t, doc))
	}
	if len(doc.Content) != 2 || doc.Content[1].Type != "image" {
		t.Errorf("그림이 제목 뒤에 오지 않았습니다: %s", dump(t, doc))
	}
}
