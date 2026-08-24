package ptium

import (
	"strings"
	"testing"
)

const deckSource = `// 2027 계획 발표자료
# 사업 추진 계획
@cover
> 2027년 상반기 · 임원 보고

# 추진 배경
@content
- 현재 42개 시스템이 개별 운영되고 있습니다
!notes 질문이 나오면 통합 일정으로 답한다

# 추진 목표
::kpi 핵심 지표
- 운영 비용 절감 | 18%
- 개발 생산성 향상 | 30%
::
`

func TestSplitSlidesFollowsPtiumsRule(t *testing.T) {
	deck := SplitSlides(deckSource)
	if len(deck.Slides) != 3 {
		t.Fatalf("slides = %d: %+v", len(deck.Slides), deck.Slides)
	}
	if deck.Slides[0].Title != "사업 추진 계획" || deck.Slides[2].Title != "추진 목표" {
		t.Fatalf("titles wrong: %+v", deck.Slides)
	}
	if deck.Slides[0].Position != 1 || deck.Slides[2].Position != 3 {
		t.Fatalf("positions wrong: %+v", deck.Slides)
	}
	if !strings.Contains(deck.Preamble, "2027 계획 발표자료") {
		t.Errorf("the comment before the first slide was lost: %q", deck.Preamble)
	}
	// A component row and speaker notes belong to their slide, not to a new one.
	if !strings.Contains(deck.Slides[2].Source, "운영 비용 절감 | 18%") {
		t.Errorf("component rows were split off: %q", deck.Slides[2].Source)
	}
	if !strings.Contains(deck.Slides[1].Source, "!notes") {
		t.Errorf("speaker notes were split off: %q", deck.Slides[1].Source)
	}
}

// The whole point of touching only the slides that changed is that the rest
// come back exactly as a person left them.
func TestUntouchedSlidesRoundTripExactly(t *testing.T) {
	deck := SplitSlides(deckSource)
	rebuilt := deck.Source()
	if strings.TrimSpace(rebuilt) != strings.TrimSpace(deckSource) {
		t.Fatalf("round trip changed the deck:\n--- before ---\n%s\n--- after ---\n%s", deckSource, rebuilt)
	}
}

func TestReplaceTouchesOneSlideOnly(t *testing.T) {
	deck := SplitSlides(deckSource)
	if !deck.Replace(2, "# 추진 배경\n@content\n- 이제 51개 시스템이 개별 운영되고 있습니다\n") {
		t.Fatal("slide 2 was not replaced")
	}
	rebuilt := deck.Source()
	if !strings.Contains(rebuilt, "51개") {
		t.Error("the replacement did not land")
	}
	if strings.Contains(rebuilt, "42개") {
		t.Error("the old text survived")
	}
	// Everything else is untouched, including a person's speaker notes.
	for _, kept := range []string{"@cover", "> 2027년 상반기 · 임원 보고", "운영 비용 절감 | 18%", "// 2027 계획 발표자료"} {
		if !strings.Contains(rebuilt, kept) {
			t.Errorf("replacing one slide disturbed %q", kept)
		}
	}
	if strings.Contains(rebuilt, "!notes 질문이") {
		// The notes belonged to the replaced slide, so losing them is correct.
		t.Log("notes on the replaced slide were replaced, as expected")
	}
}

func TestReplaceRejectsAPositionThatIsNotThere(t *testing.T) {
	deck := SplitSlides(deckSource)
	if deck.Replace(99, "# 없음") {
		t.Fatal("replacing a missing slide should fail")
	}
}

func TestSplitSlidesHandlesAnEmptyDeck(t *testing.T) {
	if deck := SplitSlides(""); len(deck.Slides) != 0 {
		t.Fatalf("slides = %+v", deck.Slides)
	}
}

func TestEscapedTitleDoesNotStartASlide(t *testing.T) {
	// A title that itself begins with a hash is written with a backslash, which
	// is how Ptium tells the two apart.
	deck := SplitSlides("# 첫 슬라이드\n\\#해시로 시작하는 문장\n")
	if len(deck.Slides) != 1 {
		t.Fatalf("an escaped line started a slide: %+v", deck.Slides)
	}
}
