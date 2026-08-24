// Package ptium turns muni documents into presentations by talking to a Ptium
// server over REST. Ptium owns storyline design, slide generation and export;
// muni owns the document, its revisions and the link between the two.
package ptium

// Brief is the contract between a source of content and a presentation
// generator. It is deliberately not muni-shaped: the same structure can carry a
// Confluence page or a Word file later without changing anything downstream.
type Brief struct {
	Version      string           `json:"version"`
	Source       BriefSource      `json:"source"`
	Presentation BriefPreferences `json:"presentation"`
	Sections     []Section        `json:"sections"`
	Citations    []Citation       `json:"citations,omitempty"`
}

type BriefSource struct {
	Type       string `json:"type"`
	DocumentID string `json:"documentId"`
	Revision   int    `json:"revision"`
	Title      string `json:"title"`
}

type BriefPreferences struct {
	Title      string `json:"title"`
	Audience   string `json:"audience"`
	Purpose    string `json:"purpose"`
	Tone       string `json:"tone"`
	Language   string `json:"language"`
	SlideCount int    `json:"slideCount"`
	Minutes    int    `json:"minutes,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

// Section is one heading's worth of content. Blocks keep the shape they had in
// the document — a list stays a list — because a generator that is told "these
// three lines are steps" can lay them out far better than one handed a
// paragraph of prose.
type Section struct {
	Title   string  `json:"title"`
	Level   int     `json:"level"`
	BlockID string  `json:"blockId,omitempty"`
	Blocks  []Block `json:"blocks,omitempty"`
}

// Kinds of content a section can hold.
const (
	BlockParagraph = "paragraph"
	BlockBullets   = "bullets"
	BlockSteps     = "steps"
	BlockTable     = "table"
	BlockQuote     = "quote"
	BlockCode      = "code"
	BlockImage     = "image"
	BlockMetrics   = "metrics"
	BlockTimeline  = "timeline"
)

type Block struct {
	Kind    string     `json:"kind"`
	BlockID string     `json:"blockId,omitempty"`
	Text    string     `json:"text,omitempty"`
	Items   []string   `json:"items,omitempty"`
	Rows    [][]string `json:"rows,omitempty"`
	Header  []string   `json:"header,omitempty"`
	Metrics []Metric   `json:"metrics,omitempty"`
	Events  []Event    `json:"events,omitempty"`
	Alt     string     `json:"alt,omitempty"`
	URL     string     `json:"url,omitempty"`
}

// Metric is a number a deck can show as a KPI instead of burying in a sentence.
type Metric struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Event is a dated item a deck can lay out as a timeline.
type Event struct {
	When string `json:"when"`
	What string `json:"what"`
}

// Citation points back at the block a claim came from, so a slide can say where
// it got its numbers.
type Citation struct {
	BlockID  string `json:"blockId"`
	Section  string `json:"section"`
	Document string `json:"document"`
	Revision int    `json:"revision"`
}

// Options are the choices a person makes before generating.
type Options struct {
	Title      string
	Audience   string
	Purpose    string
	Tone       string
	Language   string
	SlideCount int
	Minutes    int
	Detail     string
	Theme      string
	TemplateID string
}

// Presentation is the part of a Ptium presentation muni keeps track of.
type Presentation struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Status          string   `json:"status"`
	SlideCount      int      `json:"slideCount"`
	TemplateID      string   `json:"templateId"`
	TemplateName    string   `json:"templateName"`
	GenerationNotes []string `json:"generationNotes"`
	UpdatedAt       string   `json:"updatedAt"`
}

// Terminal reports whether generation has finished, either way.
func (p Presentation) Terminal() bool {
	return p.Status == "completed" || p.Status == "failed"
}
