package richdoc

import "testing"

func nest(depth int) *Node {
	root := Doc()
	current := root
	for index := 0; index < depth; index++ {
		child := &Node{Type: "blockquote"}
		current.Content = append(current.Content, child)
		current = child
	}
	return root
}

func TestWithinLimitsAcceptsADocumentSomebodyWrote(t *testing.T) {
	if !WithinLimits(nest(20)) {
		t.Errorf("보통 깊이의 문서를 거절했습니다")
	}
	wide := Doc()
	for index := 0; index < 5000; index++ {
		wide.Content = append(wide.Content, Paragraph(Text("문단")))
	}
	if !WithinLimits(wide) {
		t.Errorf("문단이 많은 문서를 거절했습니다")
	}
}

// Everything that reads a document walks it recursively, and Go dies rather
// than returning when the stack runs out.
func TestWithinLimitsRefusesWhatWouldOverflowTheStack(t *testing.T) {
	if WithinLimits(nest(MaxDepth + 5)) {
		t.Errorf("%d단계보다 깊은 문서를 받아들였습니다", MaxDepth)
	}
	huge := Doc()
	for index := 0; index < MaxNodes+10; index++ {
		huge.Content = append(huge.Content, &Node{Type: "text", Text: "x"})
	}
	if WithinLimits(huge) {
		t.Errorf("조각이 %d개가 넘는 문서를 받아들였습니다", MaxNodes)
	}
}
