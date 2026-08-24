package ptium

import (
	"strconv"
	"strings"
)

// AddCitations writes a source line onto each slide that came from a known part
// of the document.
//
// A deck that states a number and cannot say where it came from is the first
// thing anyone in a company asks about. muni knows which heading each slide was
// built from, so it can answer that without the model having to remember to.
//
// Slides that already cite something are left alone: a citation someone wrote
// by hand is better than one derived from a title match, and running this twice
// must not stack duplicates.
func AddCitations(deck Deck, brief Brief) (Deck, int) {
	if len(brief.Citations) == 0 {
		return deck, 0
	}
	added := 0
	for index, slide := range deck.Slides {
		if hasCitation(slide.Source) || isFrontOrBack(slide.Source) {
			continue
		}
		citation, ok := citationFor(slide.Title, brief)
		if !ok {
			continue
		}
		deck.Slides[index].Source = appendDirective(slide.Source, citationLine(citation))
		added++
	}
	return deck, added
}

func citationFor(slideTitle string, brief Brief) (Citation, bool) {
	for _, citation := range brief.Citations {
		if citation.Section != "" && titlesMatch(slideTitle, citation.Section) {
			return citation, true
		}
	}
	return Citation{}, false
}

// citationLine writes the directive Ptium reads. The marker is optional, so a
// slide that makes no numbered claim still gets a readable attribution.
func citationLine(citation Citation) string {
	title := citation.Document
	if citation.Revision > 0 {
		title += " (Revision " + strconv.Itoa(citation.Revision) + ")"
	}
	fields := []string{escapeField(title)}
	if citation.Section != "" {
		fields = append(fields, escapeField(citation.Section))
	}
	return "!source " + strings.Join(fields, " | ")
}

// escapeField protects the separator Ptium splits fields on.
func escapeField(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, "|", `\|`)
}

func hasCitation(source string) bool {
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(trimmed, "!source") || strings.HasPrefix(strings.TrimSpace(line), "!출처") {
			return true
		}
	}
	return false
}

// isFrontOrBack reports a cover or closing slide, which carries no claim of its
// own and reads worse with a source line under it.
func isFrontOrBack(source string) bool {
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.ToLower(strings.TrimSpace(line))
		switch trimmed {
		case "@cover", "@closing", "@표지", "@마무리":
			return true
		}
	}
	return false
}

// appendDirective adds a line at the end of a slide, before the blank line that
// separates it from the next one.
func appendDirective(source, directive string) string {
	lines := strings.Split(strings.TrimRight(source, "\n"), "\n")
	end := len(lines)
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:end]...)
	out = append(out, directive)
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n") + "\n"
}
