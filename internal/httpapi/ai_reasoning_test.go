package httpapi

import "testing"

func TestStripReasoning(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			// What the Qwen family sends when reasoning was never asked for.
			name:  "closing tag with no opening one",
			input: "사용자가 요약을 원한다. 문서는 3개 항목으로...\n</think>\n요약: 세 가지 항목입니다.",
			want:  "요약: 세 가지 항목입니다.",
		},
		{
			name:  "a complete block",
			input: "<think>먼저 생각한다</think>답변입니다.",
			want:  "답변입니다.",
		},
		{
			name:  "a block in the middle",
			input: "앞부분 <think>중간 생각</think> 뒷부분",
			want:  "앞부분  뒷부분",
		},
		{
			name:  "thinking that never finished is not an answer",
			input: "<think>아직 생각 중이고 답은 없다",
			want:  "",
		},
		{
			name:  "no reasoning at all",
			input: "그냥 답변입니다.",
			want:  "그냥 답변입니다.",
		},
		{
			name:  "the last closing tag wins",
			input: "생각1</think>생각2</think>진짜 답변",
			want:  "진짜 답변",
		},
		{
			name:  "other wrappers",
			input: "<reasoning>내부</reasoning>결과",
			want:  "결과",
		},
		{
			name:  "case is ignored",
			input: "생각\n</THINK>\n답변",
			want:  "답변",
		},
		{
			name:  "empty input",
			input: "",
			want:  "",
		},
	}
	for _, item := range cases {
		if got := stripReasoning(item.input); got != item.want {
			t.Errorf("%s: stripReasoning(%q) = %q, want %q", item.name, item.input, got, item.want)
		}
	}
}

// The patch endpoint looks for JSON; reasoning full of braces in front of it
// would be read as the answer.
func TestReasoningIsRemovedBeforePatchParsing(t *testing.T) {
	answer := stripReasoning(`블록 {blk_a} 를 고쳐야 한다고 생각한다.
</think>
{"edits":[{"blockId":"blk_a","newText":"고친 문장"}]}`)
	edits, err := parsePatchEdits(answer)
	if err != nil {
		t.Fatalf("parse: %v (answer=%q)", err, answer)
	}
	if len(edits) != 1 || edits[0].NewText != "고친 문장" {
		t.Fatalf("unexpected edits: %+v", edits)
	}
}
