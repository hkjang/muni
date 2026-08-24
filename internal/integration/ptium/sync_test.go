package ptium

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hkjang/muni/internal/richdoc"
)

func briefFrom(t *testing.T, raw string, revision int) (Brief, *richdoc.Node) {
	t.Helper()
	document, err := richdoc.Parse(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	brief := BuildBrief(document, BriefSource{
		Type: "muni", DocumentID: "doc-1", Revision: revision, Title: "사업 추진 계획",
	}, Options{})
	return brief, document
}

const beforeDocument = `{"type":"doc","content":[
	{"type":"heading","attrs":{"level":1,"blockId":"blk_bg"},"content":[{"type":"text","text":"추진 배경"}]},
	{"type":"paragraph","attrs":{"blockId":"blk_p1"},"content":[{"type":"text","text":"현재 42개 시스템이 개별 운영되고 있습니다."}]},
	{"type":"heading","attrs":{"level":1,"blockId":"blk_goal"},"content":[{"type":"text","text":"추진 목표"}]},
	{"type":"paragraph","attrs":{"blockId":"blk_p2"},"content":[{"type":"text","text":"운영 비용을 절감합니다."}]}
]}`

const syncDeck = `# 사업 추진 계획
@cover

# 추진 배경
- 42개 시스템이 개별 운영

# 추진 목표
- 운영 비용 절감
`

func TestPlanTouchesOnlyTheSlideWhoseSectionChanged(t *testing.T) {
	before, beforeDoc := briefFrom(t, beforeDocument, 17)
	afterRaw := strings.Replace(beforeDocument, "42개 시스템이 개별 운영되고 있습니다", "51개 시스템이 개별 운영되고 있습니다", 1)
	after, afterDoc := briefFrom(t, afterRaw, 22)

	plan := PlanSync(richdoc.Diff(beforeDoc, afterDoc), SplitSlides(syncDeck), before, after)

	if plan.Revise != 1 || plan.Add != 0 || plan.Remove != 0 {
		t.Fatalf("expected exactly one slide to be revised: %+v", plan)
	}
	if plan.Keep != 2 {
		t.Fatalf("the untouched slides should be kept: %+v", plan)
	}
	var revised SlideImpact
	for _, impact := range plan.Impacts {
		if impact.Action == SlideRevise {
			revised = impact
		}
	}
	if revised.Title != "추진 배경" || revised.Position != 2 {
		t.Fatalf("the wrong slide was picked: %+v", revised)
	}
	if !strings.Contains(revised.Instruction, "51개") || !strings.Contains(revised.Instruction, "추진 배경") {
		t.Errorf("the instruction does not say what changed: %q", revised.Instruction)
	}
	if plan.FromRevision != 17 || plan.ToRevision != 22 {
		t.Errorf("revisions = %d → %d", plan.FromRevision, plan.ToRevision)
	}
}

func TestPlanIsEmptyWhenNothingChanged(t *testing.T) {
	before, beforeDoc := briefFrom(t, beforeDocument, 17)
	after, afterDoc := briefFrom(t, beforeDocument, 17)
	plan := PlanSync(richdoc.Diff(beforeDoc, afterDoc), SplitSlides(syncDeck), before, after)
	if plan.Changed() {
		t.Fatalf("an unchanged document should need no slide work: %+v", plan)
	}
	if plan.Keep != 3 {
		t.Fatalf("every slide should be kept: %+v", plan)
	}
}

func TestANewSectionAsksForASlide(t *testing.T) {
	before, beforeDoc := briefFrom(t, beforeDocument, 17)
	afterRaw := `{"type":"doc","content":[
		{"type":"heading","attrs":{"level":1,"blockId":"blk_bg"},"content":[{"type":"text","text":"추진 배경"}]},
		{"type":"paragraph","attrs":{"blockId":"blk_p1"},"content":[{"type":"text","text":"현재 42개 시스템이 개별 운영되고 있습니다."}]},
		{"type":"heading","attrs":{"level":1,"blockId":"blk_goal"},"content":[{"type":"text","text":"추진 목표"}]},
		{"type":"paragraph","attrs":{"blockId":"blk_p2"},"content":[{"type":"text","text":"운영 비용을 절감합니다."}]},
		{"type":"heading","attrs":{"level":1,"blockId":"blk_risk"},"content":[{"type":"text","text":"위험 요인"}]},
		{"type":"paragraph","attrs":{"blockId":"blk_p3"},"content":[{"type":"text","text":"일정 지연 가능성이 있습니다."}]}
	]}`
	after, afterDoc := briefFrom(t, afterRaw, 23)

	plan := PlanSync(richdoc.Diff(beforeDoc, afterDoc), SplitSlides(syncDeck), before, after)
	if plan.Add != 1 {
		t.Fatalf("a new section should ask for a slide: %+v", plan)
	}
	var added SlideImpact
	for _, impact := range plan.Impacts {
		if impact.Action == SlideAdd {
			added = impact
		}
	}
	if added.Title != "위험 요인" || added.Position != 0 {
		t.Fatalf("unexpected addition: %+v", added)
	}
}

func TestARemovedSectionMarksItsSlide(t *testing.T) {
	before, beforeDoc := briefFrom(t, beforeDocument, 17)
	afterRaw := `{"type":"doc","content":[
		{"type":"heading","attrs":{"level":1,"blockId":"blk_bg"},"content":[{"type":"text","text":"추진 배경"}]},
		{"type":"paragraph","attrs":{"blockId":"blk_p1"},"content":[{"type":"text","text":"현재 42개 시스템이 개별 운영되고 있습니다."}]}
	]}`
	after, afterDoc := briefFrom(t, afterRaw, 24)

	plan := PlanSync(richdoc.Diff(beforeDoc, afterDoc), SplitSlides(syncDeck), before, after)
	if plan.Remove != 1 {
		t.Fatalf("the slide for a deleted section should be flagged: %+v", plan)
	}
	for _, impact := range plan.Impacts {
		if impact.Action == SlideRemove && impact.Title != "추진 목표" {
			t.Errorf("the wrong slide was flagged: %+v", impact)
		}
	}
}

// A generator rarely copies a heading word for word.
func TestTitlesMatchAcrossSmallRewordings(t *testing.T) {
	cases := []struct {
		slide, section string
		want           bool
	}{
		{"추진 배경", "추진 배경", true},
		{"추진 배경 ", "추진배경", true},
		{"2. 추진 배경", "추진 배경", true},
		{"추진 배경과 현황", "추진 배경", true},
		{"추진 목표", "추진 배경", false},
		{"", "추진 배경", false},
		{"Q1", "Q2", false},
	}
	for _, item := range cases {
		if got := titlesMatch(item.slide, item.section); got != item.want {
			t.Errorf("titlesMatch(%q, %q) = %v", item.slide, item.section, got)
		}
	}
}

func TestPlanKeepsSlidesThatHaveNoSectionAtAll(t *testing.T) {
	before, beforeDoc := briefFrom(t, beforeDocument, 17)
	afterRaw := strings.Replace(beforeDocument, "42개", "51개", 1)
	after, afterDoc := briefFrom(t, afterRaw, 22)

	// A cover and a closing slide come from the generator, not from a heading.
	deck := SplitSlides("# 사업 추진 계획\n@cover\n\n# 추진 배경\n- 42개\n\n# 감사합니다\n@closing\n")
	plan := PlanSync(richdoc.Diff(beforeDoc, afterDoc), deck, before, after)
	if plan.Keep != 2 || plan.Revise != 1 {
		t.Fatalf("slides without a matching section must be left alone: %+v", plan)
	}
}
