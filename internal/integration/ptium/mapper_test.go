package ptium

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

func parse(t *testing.T, raw string) *richdoc.Node {
	t.Helper()
	node, err := richdoc.Parse(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	return node
}

func source() BriefSource {
	return BriefSource{Type: "muni", DocumentID: "doc-1", Revision: 17, Title: "사업 추진 계획"}
}

func TestBriefKeepsTheDocumentStructure(t *testing.T) {
	document := parse(t, `{"type":"doc","content":[
		{"type":"heading","attrs":{"level":1,"blockId":"blk_bg"},"content":[{"type":"text","text":"추진 배경"}]},
		{"type":"paragraph","content":[{"type":"text","text":"현재 42개 시스템이 개별 운영되고 있습니다."}]},
		{"type":"heading","attrs":{"level":1,"blockId":"blk_goal"},"content":[{"type":"text","text":"추진 목표"}]},
		{"type":"bulletList","content":[
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"운영 비용 18% 절감"}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"개발 생산성 30% 향상"}]}]}]}
	]}`)
	brief := BuildBrief(document, source(), Options{Audience: "executive", SlideCount: 10})

	if len(brief.Sections) != 2 {
		t.Fatalf("sections = %d: %+v", len(brief.Sections), brief.Sections)
	}
	if brief.Sections[0].Title != "추진 배경" || brief.Sections[1].Title != "추진 목표" {
		t.Fatalf("headings became sections incorrectly: %+v", brief.Sections)
	}
	// Two figures in a list are measures, not prose.
	goals := brief.Sections[1].Blocks[0]
	if goals.Kind != BlockMetrics || len(goals.Metrics) != 2 {
		t.Fatalf("the goal list was not read as measures: %+v", goals)
	}
	if goals.Metrics[0].Value != "18%" {
		t.Errorf("metric value = %q", goals.Metrics[0].Value)
	}
}

func TestBriefKeepsContentBeforeTheFirstHeading(t *testing.T) {
	document := parse(t, `{"type":"doc","content":[
		{"type":"paragraph","content":[{"type":"text","text":"머리말입니다."}]},
		{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"본론"}]}
	]}`)
	brief := BuildBrief(document, source(), Options{})
	if len(brief.Sections) != 2 || brief.Sections[0].Title != "" {
		t.Fatalf("the opening was dropped: %+v", brief.Sections)
	}
	if brief.Sections[0].Blocks[0].Text != "머리말입니다." {
		t.Fatalf("unexpected opening: %+v", brief.Sections[0])
	}
}

func TestOrderedListsBecomeSteps(t *testing.T) {
	document := parse(t, `{"type":"doc","content":[
		{"type":"orderedList","content":[
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"요구사항 정리"}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"설계 검토"}]}]}]}
	]}`)
	brief := BuildBrief(document, source(), Options{})
	block := brief.Sections[0].Blocks[0]
	if block.Kind != BlockSteps || len(block.Items) != 2 {
		t.Fatalf("an ordered list should read as steps: %+v", block)
	}
}

func TestDatedListsBecomeATimeline(t *testing.T) {
	document := parse(t, `{"type":"doc","content":[
		{"type":"bulletList","content":[
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"2026 Q1: 설계"}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"2026 Q3: 오픈"}]}]}]}
	]}`)
	brief := BuildBrief(document, source(), Options{})
	block := brief.Sections[0].Blocks[0]
	if block.Kind != BlockTimeline || len(block.Events) != 2 {
		t.Fatalf("a dated list should read as a timeline: %+v", block)
	}
	if block.Events[1].What != "오픈" {
		t.Errorf("event text = %q", block.Events[1].What)
	}
}

func TestOrdinaryListsStayBullets(t *testing.T) {
	document := parse(t, `{"type":"doc","content":[
		{"type":"bulletList","content":[
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"첫 항목"}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"둘째 항목"}]}]}]}
	]}`)
	block := BuildBrief(document, source(), Options{}).Sections[0].Blocks[0]
	if block.Kind != BlockBullets {
		t.Fatalf("a plain list should stay bullets: %+v", block)
	}
}

func TestTablesKeepTheirHeader(t *testing.T) {
	document := parse(t, `{"type":"doc","content":[
		{"type":"table","attrs":{"blockId":"blk_t"},"content":[
			{"type":"tableRow","content":[
				{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"연도"}]}]},
				{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"목표"}]}]}]},
			{"type":"tableRow","content":[
				{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"2026"}]}]},
				{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"10건"}]}]}]}]}
	]}`)
	block := BuildBrief(document, source(), Options{}).Sections[0].Blocks[0]
	if block.Kind != BlockTable || len(block.Header) != 2 || len(block.Rows) != 1 {
		t.Fatalf("table not mapped: %+v", block)
	}
	if block.Header[0] != "연도" || block.Rows[0][1] != "10건" {
		t.Fatalf("table contents wrong: %+v", block)
	}
}

func TestCitationsPointAtTheSourceBlocks(t *testing.T) {
	document := parse(t, `{"type":"doc","content":[
		{"type":"heading","attrs":{"level":1,"blockId":"blk_bg"},"content":[{"type":"text","text":"추진 배경"}]}
	]}`)
	brief := BuildBrief(document, source(), Options{})
	if len(brief.Citations) != 1 {
		t.Fatalf("citations = %+v", brief.Citations)
	}
	citation := brief.Citations[0]
	if citation.BlockID != "blk_bg" || citation.Revision != 17 || citation.Document != "사업 추진 계획" {
		t.Fatalf("citation does not identify the source: %+v", citation)
	}
}

func TestRenderPromptLabelsTheStructure(t *testing.T) {
	document := parse(t, `{"type":"doc","content":[
		{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"추진 목표"}]},
		{"type":"orderedList","content":[
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"설계"}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"구현"}]}]}]},
		{"type":"bulletList","content":[
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"비용 18% 절감"}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"생산성 30% 향상"}]}]}]}
	]}`)
	prompt := RenderPrompt(BuildBrief(document, source(), Options{Purpose: "의사결정", Minutes: 10}))

	for _, expected := range []string{"사업 추진 계획", "발표 목적: 의사결정", "발표 시간: 10분", "## 추진 목표", "순서", "1. 설계", "수치"} {
		if !strings.Contains(prompt, expected) {
			t.Errorf("prompt missing %q:\n%s", expected, prompt)
		}
	}
}

func TestRenderPromptStaysWithinTheProviderLimit(t *testing.T) {
	blocks := make([]string, 0, 400)
	for index := 0; index < 400; index++ {
		blocks = append(blocks, `{"type":"paragraph","content":[{"type":"text","text":"`+strings.Repeat("긴 문단입니다. ", 20)+`"}]}`)
	}
	document := parse(t, `{"type":"doc","content":[`+strings.Join(blocks, ",")+`]}`)
	prompt := RenderPrompt(BuildBrief(document, source(), Options{}))
	if len(prompt) > maxPromptChars+64 {
		t.Fatalf("prompt is %d bytes, over the %d limit", len(prompt), maxPromptChars)
	}
	if !strings.Contains(prompt, "생략") {
		t.Error("a truncated prompt should say so")
	}
}

// "18% 절감" and "18% 증가" mean opposite things; a metric that keeps only the
// words before the figure loses the difference.
func TestMetricLabelsKeepTheirDirection(t *testing.T) {
	document := parse(t, `{"type":"doc","content":[
		{"type":"bulletList","content":[
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"운영 비용 18% 절감"}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"장애 건수 12건 증가"}]}]}]}
	]}`)
	block := BuildBrief(document, source(), Options{}).Sections[0].Blocks[0]
	if block.Kind != BlockMetrics {
		t.Fatalf("expected measures: %+v", block)
	}
	if block.Metrics[0].Label != "운영 비용 절감" {
		t.Errorf("label = %q, want the direction kept", block.Metrics[0].Label)
	}
	if block.Metrics[1].Label != "장애 건수 증가" {
		t.Errorf("label = %q, want the direction kept", block.Metrics[1].Label)
	}
	if block.Metrics[0].Value != "18%" || block.Metrics[1].Value != "12건" {
		t.Errorf("values = %+v", block.Metrics)
	}
}
