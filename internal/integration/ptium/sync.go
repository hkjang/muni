package ptium

import (
	"sort"
	"strings"
	"unicode"

	"github.com/hkjang/muni/internal/richdoc"
)

// What a document change means for one slide.
const (
	SlideKeep   = "keep"
	SlideRevise = "revise"
	SlideAdd    = "add"
	SlideRemove = "remove"
)

// SlideImpact is one slide's share of a document change.
type SlideImpact struct {
	Position int    `json:"position,omitempty"`
	Title    string `json:"title"`
	Action   string `json:"action"`
	Section  string `json:"section,omitempty"`
	Reason   string `json:"reason,omitempty"`
	// Instruction is what to ask for when redrafting this slide.
	Instruction string `json:"instruction,omitempty"`
}

type SyncPlan struct {
	FromRevision int           `json:"fromRevision"`
	ToRevision   int           `json:"toRevision"`
	Impacts      []SlideImpact `json:"impacts"`
	Revise       int           `json:"revise"`
	Add          int           `json:"add"`
	Remove       int           `json:"remove"`
	Keep         int           `json:"keep"`
}

// Changed reports whether anything needs doing.
func (p SyncPlan) Changed() bool { return p.Revise+p.Add+p.Remove > 0 }

// PlanSync works out which slides a document change touches.
//
// Regenerating the whole deck would throw away everything a person did in the
// presentation editor, which is usually most of its value. Slides whose source
// material did not change are left exactly as they are; only the ones whose
// section moved are put in front of the model again.
func PlanSync(diff richdoc.DiffResult, deck Deck, before, after Brief) SyncPlan {
	plan := SyncPlan{FromRevision: before.Source.Revision, ToRevision: after.Source.Revision}

	// Which section does each changed block belong to, and what changed in it.
	changedSections := sectionChanges(diff, after, before)
	matched := map[string]bool{}

	for _, slide := range deck.Slides {
		section, reason, ok := matchSection(slide.Title, changedSections)
		if !ok {
			plan.Impacts = append(plan.Impacts, SlideImpact{
				Position: slide.Position, Title: slide.Title, Action: SlideKeep,
			})
			plan.Keep++
			continue
		}
		matched[section] = true
		plan.Impacts = append(plan.Impacts, SlideImpact{
			Position: slide.Position, Title: slide.Title, Action: SlideRevise,
			Section: section, Reason: reason,
			Instruction: reviseInstruction(section, reason),
		})
		plan.Revise++
	}

	// A section that gained content but has no slide needs one; a section that
	// went away leaves a slide with nothing behind it.
	for _, section := range sortedSections(changedSections) {
		change := changedSections[section]
		if matched[section] || change.kind != sectionAdded {
			continue
		}
		plan.Impacts = append(plan.Impacts, SlideImpact{
			Title: section, Action: SlideAdd, Section: section, Reason: change.reason,
			Instruction: "문서에 새로 추가된 내용입니다:\n" + change.reason,
		})
		plan.Add++
	}
	for _, section := range sortedSections(changedSections) {
		change := changedSections[section]
		if change.kind != sectionRemoved {
			continue
		}
		for _, slide := range deck.Slides {
			if !titlesMatch(slide.Title, section) {
				continue
			}
			for index := range plan.Impacts {
				if plan.Impacts[index].Position != slide.Position {
					continue
				}
				if plan.Impacts[index].Action == SlideKeep {
					plan.Keep--
				} else if plan.Impacts[index].Action == SlideRevise {
					plan.Revise--
				}
				plan.Impacts[index] = SlideImpact{
					Position: slide.Position, Title: slide.Title, Action: SlideRemove,
					Section: section, Reason: change.reason,
				}
				plan.Remove++
			}
		}
	}
	return plan
}

const (
	sectionChanged = "changed"
	sectionAdded   = "added"
	sectionRemoved = "removed"
)

type sectionChange struct {
	kind   string
	reason string
}

// sectionChanges attributes each changed block to the heading it sits under.
func sectionChanges(diff richdoc.DiffResult, after, before Brief) map[string]sectionChange {
	owner := blockOwners(after)
	for block, section := range blockOwners(before) {
		if _, exists := owner[block]; !exists {
			owner[block] = section
		}
	}
	afterTitles := sectionTitles(after)
	beforeTitles := sectionTitles(before)

	changes := map[string]sectionChange{}
	note := func(section, kind, line string) {
		if section == "" {
			return
		}
		existing, seen := changes[section]
		if !seen {
			changes[section] = sectionChange{kind: kind, reason: line}
			return
		}
		if existing.kind != kind {
			existing.kind = sectionChanged
		}
		if len(existing.reason) < 600 {
			existing.reason += "\n" + line
		}
		changes[section] = existing
	}

	for _, block := range diff.Blocks {
		switch block.Status {
		case richdoc.BlockAdded:
			note(owner[block.BlockID], sectionChanged, "추가: "+truncate(block.After, 200))
		case richdoc.BlockRemoved:
			note(owner[block.BlockID], sectionChanged, "삭제: "+truncate(block.Before, 200))
		case richdoc.BlockChanged:
			note(owner[block.BlockID], sectionChanged,
				"변경: "+truncate(block.Before, 120)+" → "+truncate(block.After, 120))
		}
	}

	// Whole sections that appeared or went away.
	for title := range afterTitles {
		if !beforeTitles[title] {
			changes[title] = sectionChange{kind: sectionAdded, reason: "문서에 '" + title + "' 항목이 추가되었습니다."}
		}
	}
	for title := range beforeTitles {
		if !afterTitles[title] {
			changes[title] = sectionChange{kind: sectionRemoved, reason: "문서에서 '" + title + "' 항목이 사라졌습니다."}
		}
	}
	return changes
}

// blockOwners maps every block id to the heading it belongs under.
func blockOwners(brief Brief) map[string]string {
	owners := map[string]string{}
	for _, section := range brief.Sections {
		if section.BlockID != "" {
			owners[section.BlockID] = section.Title
		}
		for _, block := range section.Blocks {
			if block.BlockID != "" {
				owners[block.BlockID] = section.Title
			}
		}
	}
	return owners
}

func sectionTitles(brief Brief) map[string]bool {
	titles := map[string]bool{}
	for _, section := range brief.Sections {
		if section.Title != "" {
			titles[section.Title] = true
		}
	}
	return titles
}

func sortedSections(changes map[string]sectionChange) []string {
	names := make([]string, 0, len(changes))
	for name := range changes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func matchSection(slideTitle string, changes map[string]sectionChange) (string, string, bool) {
	for _, section := range sortedSections(changes) {
		if changes[section].kind == sectionRemoved {
			continue
		}
		if titlesMatch(slideTitle, section) {
			return section, changes[section].reason, true
		}
	}
	return "", "", false
}

// titlesMatch compares a slide title to a document heading. A generator often
// shortens or rephrases a heading slightly, so an exact match would miss most
// of the pairs it is meant to find.
func titlesMatch(slideTitle, section string) bool {
	left, right := normalizeTitle(slideTitle), normalizeTitle(section)
	if left == "" || right == "" {
		return false
	}
	if left == right {
		return true
	}
	if len(left) >= 4 && len(right) >= 4 {
		return strings.Contains(left, right) || strings.Contains(right, left)
	}
	return false
}

func normalizeTitle(value string) string {
	var out strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out.WriteRune(unicode.ToLower(r))
		}
	}
	return out.String()
}

func reviseInstruction(section, reason string) string {
	return "이 슬라이드가 참고한 문서의 '" + section + "' 부분이 다음과 같이 바뀌었습니다.\n" +
		reason + "\n\n" +
		"바뀐 내용에 맞게 이 슬라이드를 고쳐 주세요. 슬라이드의 구성과 형식은 그대로 두고 달라진 부분만 반영하세요."
}
