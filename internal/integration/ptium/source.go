package ptium

import "strings"

// Slide is one slide of a deck, kept as the exact text Ptium compiled it from.
// Round tripping the source is exact, so a slide nobody touched comes back
// byte for byte — which is what keeps a person's edits in Ptium intact when
// muni only means to update its neighbour.
type Slide struct {
	Position int // 1-based, as Ptium numbers slides
	Title    string
	Source   string
}

// Deck is a parsed deck source.
type Deck struct {
	// Preamble is anything before the first slide, kept verbatim.
	Preamble string
	Slides   []Slide
}

// SplitSlides divides deck source into slides.
//
// The rule matches Ptium's own parser: a line whose trimmed form starts with a
// hash begins a slide. A title that really starts with a hash is escaped with a
// backslash, so it does not trip this.
func SplitSlides(source string) Deck {
	deck := Deck{}
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")

	preamble := make([]string, 0, 4)
	current := make([]string, 0, 16)
	title := ""
	started := false

	flush := func() {
		if !started {
			return
		}
		deck.Slides = append(deck.Slides, Slide{
			Position: len(deck.Slides) + 1,
			Title:    title,
			Source:   strings.Join(current, "\n"),
		})
		current = current[:0]
	}

	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			flush()
			started = true
			title = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#"))
			current = append(current[:0], line)
			continue
		}
		if !started {
			preamble = append(preamble, line)
			continue
		}
		current = append(current, line)
	}
	flush()
	deck.Preamble = strings.Join(preamble, "\n")
	return deck
}

// Source reassembles the deck. Each slide keeps the exact lines it was split
// from, including the blank line that separated it from the next, so a deck
// nobody edited comes back byte for byte.
func (d Deck) Source() string {
	parts := make([]string, 0, len(d.Slides)+1)
	if d.Preamble != "" {
		parts = append(parts, d.Preamble)
	}
	for _, slide := range d.Slides {
		parts = append(parts, slide.Source)
	}
	return strings.Join(parts, "\n")
}

// Replace swaps one slide's text, leaving every other slide untouched.
func (d *Deck) Replace(position int, source string) bool {
	for index := range d.Slides {
		if d.Slides[index].Position != position {
			continue
		}
		// A replacement ends with the blank line that separates slides, so the
		// deck keeps its shape when it is put back together.
		d.Slides[index].Source = strings.TrimRight(source, "\n") + "\n"
		d.Slides[index].Title = firstTitle(source)
		return true
	}
	return false
}

func firstTitle(source string) string {
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			return strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		}
	}
	return ""
}
