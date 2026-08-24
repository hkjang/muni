package ptium

import (
	"strings"
	"testing"
)

func citationBrief() Brief {
	return Brief{
		Source: BriefSource{Title: "AI전략보고서", Revision: 23},
		Citations: []Citation{
			{BlockID: "blk_bg", Section: "추진 배경", Document: "AI전략보고서", Revision: 23},
			{BlockID: "blk_goal", Section: "추진 목표", Document: "AI전략보고서", Revision: 23},
		},
	}
}

const citationDeck = `# 사업 추진 계획
@cover
> 임원 보고

# 추진 배경
- 42개 시스템이 개별 운영

# 추진 목표
::kpi 핵심 지표
- 운영 비용 절감 | 18%
::

# 감사합니다
@closing
`

func TestCitationsAreWrittenOntoTheSlidesTheyBelongTo(t *testing.T) {
	deck, added := AddCitations(SplitSlides(citationDeck), citationBrief())
	if added != 2 {
		t.Fatalf("added = %d, want the two content slides", added)
	}
	source := deck.Source()
	if !strings.Contains(source, "!source AI전략보고서 (Revision 23) | 추진 배경") {
		t.Errorf("the background slide was not cited:\n%s", source)
	}
	if !strings.Contains(source, "!source AI전략보고서 (Revision 23) | 추진 목표") {
		t.Errorf("the goal slide was not cited:\n%s", source)
	}
}

// A cover and a closing slide make no claim of their own.
func TestCoverAndClosingAreNotCited(t *testing.T) {
	deck, _ := AddCitations(SplitSlides(citationDeck), citationBrief())
	if strings.Contains(deck.Slides[0].Source, "!source") {
		t.Errorf("the cover was cited:\n%s", deck.Slides[0].Source)
	}
	if strings.Contains(deck.Slides[3].Source, "!source") {
		t.Errorf("the closing was cited:\n%s", deck.Slides[3].Source)
	}
}

func TestAddingCitationsTwiceDoesNotStackThem(t *testing.T) {
	deck, first := AddCitations(SplitSlides(citationDeck), citationBrief())
	deck, second := AddCitations(deck, citationBrief())
	if first == 0 || second != 0 {
		t.Fatalf("first = %d, second = %d", first, second)
	}
	if strings.Count(deck.Source(), "!source") != first {
		t.Errorf("citations were duplicated:\n%s", deck.Source())
	}
}

// A citation somebody wrote by hand knows more than one derived from a title.
func TestAHandWrittenCitationIsLeftAlone(t *testing.T) {
	deck := SplitSlides("# 추진 배경\n- 42개 시스템\n!source 2026 시장 조사 보고서 | p.42\n")
	updated, added := AddCitations(deck, citationBrief())
	if added != 0 {
		t.Fatalf("added = %d", added)
	}
	if !strings.Contains(updated.Source(), "2026 시장 조사 보고서") {
		t.Error("the existing citation was lost")
	}
}

func TestCitationEscapesTheFieldSeparator(t *testing.T) {
	brief := Brief{
		Source: BriefSource{Title: "매출 | 비용 분석", Revision: 4},
		Citations: []Citation{
			{Section: "추진 배경", Document: "매출 | 비용 분석", Revision: 4},
		},
	}
	deck, added := AddCitations(SplitSlides("# 추진 배경\n- 내용\n"), brief)
	if added != 1 {
		t.Fatalf("added = %d", added)
	}
	line := ""
	for _, candidate := range strings.Split(deck.Source(), "\n") {
		if strings.HasPrefix(candidate, "!source") {
			line = candidate
		}
	}
	// The pipe inside the title must not be read as a field boundary.
	if !strings.Contains(line, `매출 \| 비용 분석`) {
		t.Fatalf("the separator was not escaped: %q", line)
	}
	if strings.Count(strings.ReplaceAll(line, `\|`, ""), "|") != 1 {
		t.Fatalf("unexpected field count: %q", line)
	}
}

func TestCitationLandsBeforeTheBlankLineBetweenSlides(t *testing.T) {
	deck, _ := AddCitations(SplitSlides(citationDeck), citationBrief())
	rebuilt := deck.Source()
	// The deck must still split into the same slides after the change.
	if len(SplitSlides(rebuilt).Slides) != 4 {
		t.Fatalf("the citation broke the slide boundaries:\n%s", rebuilt)
	}
	if !strings.Contains(rebuilt, "::\n!source") {
		t.Errorf("the citation should follow the slide's content:\n%s", rebuilt)
	}
}

func TestNoCitationsWhenTheDocumentHasNoHeadings(t *testing.T) {
	deck, added := AddCitations(SplitSlides(citationDeck), Brief{})
	if added != 0 || strings.Contains(deck.Source(), "!source") {
		t.Fatal("a brief with no citations should add none")
	}
}
