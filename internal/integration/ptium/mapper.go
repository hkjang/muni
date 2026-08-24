package ptium

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/hkjang/muni/internal/richdoc"
)

const (
	maxBriefSections = 40
	maxBriefBlocks   = 200
	maxItemsPerBlock = 12
	maxTableRows     = 12
)

// BuildBrief reads the document tree rather than a flattened string.
//
// A generator handed "운영 비용 18% 절감, 개발 생산성 30% 향상" as prose has to
// guess that those are two bullets and two numbers; handed the list and the
// numbers it can lay them out as a KPI row. Keeping the structure the author
// already expressed is the whole point of reading ProseMirror JSON.
func BuildBrief(document *richdoc.Node, source BriefSource, options Options) Brief {
	brief := Brief{
		Version: "1.0",
		Source:  source,
		Presentation: BriefPreferences{
			Title:      firstNonEmpty(options.Title, source.Title),
			Audience:   options.Audience,
			Purpose:    options.Purpose,
			Tone:       options.Tone,
			Language:   options.Language,
			SlideCount: options.SlideCount,
			Minutes:    options.Minutes,
			Detail:     options.Detail,
		},
	}

	// Content before the first heading belongs to the deck's opening.
	current := &Section{Title: "", Level: 0}
	blocks := 0
	flush := func() {
		if current != nil && (current.Title != "" || len(current.Blocks) > 0) &&
			len(brief.Sections) < maxBriefSections {
			brief.Sections = append(brief.Sections, *current)
		}
	}

	var walk func(*richdoc.Node)
	walk = func(node *richdoc.Node) {
		if node == nil || blocks >= maxBriefBlocks {
			return
		}
		switch node.Type {
		case "heading":
			flush()
			current = &Section{
				Title:   strings.TrimSpace(node.PlainText()),
				Level:   node.AttrInt("level", 1),
				BlockID: node.AttrString(richdoc.BlockIDAttr),
			}
			return
		case "paragraph":
			if block, ok := paragraphBlock(node); ok {
				current.Blocks = append(current.Blocks, block)
				blocks++
			}
			return
		case "bulletList", "taskList":
			if block, ok := listBlock(node, BlockBullets); ok {
				current.Blocks = append(current.Blocks, block)
				blocks++
			}
			return
		case "orderedList":
			if block, ok := listBlock(node, BlockSteps); ok {
				current.Blocks = append(current.Blocks, block)
				blocks++
			}
			return
		case "table":
			if block, ok := tableBlock(node); ok {
				current.Blocks = append(current.Blocks, block)
				blocks++
			}
			return
		case "blockquote":
			if text := strings.TrimSpace(node.PlainText()); text != "" {
				current.Blocks = append(current.Blocks, Block{
					Kind: BlockQuote, Text: text,
					BlockID: node.AttrString(richdoc.BlockIDAttr),
				})
				blocks++
			}
			return
		case "codeBlock":
			if text := strings.TrimSpace(node.PlainText()); text != "" {
				current.Blocks = append(current.Blocks, Block{
					Kind: BlockCode, Text: text,
					BlockID: node.AttrString(richdoc.BlockIDAttr),
				})
				blocks++
			}
			return
		case "image":
			current.Blocks = append(current.Blocks, Block{
				Kind: BlockImage,
				Alt:  node.AttrString("alt"),
				URL:  node.AttrString("src"),
			})
			blocks++
			return
		}
		for _, child := range node.Content {
			walk(child)
		}
	}
	walk(document)
	flush()

	brief.Citations = collectCitations(brief, source)
	return brief
}

func paragraphBlock(node *richdoc.Node) (Block, bool) {
	text := strings.TrimSpace(node.PlainText())
	if text == "" {
		return Block{}, false
	}
	block := Block{Kind: BlockParagraph, Text: text, BlockID: node.AttrString(richdoc.BlockIDAttr)}
	// A sentence built around figures reads better as a KPI than as prose.
	if metrics := extractMetrics(text); len(metrics) >= 2 {
		block.Kind = BlockMetrics
		block.Metrics = metrics
	}
	return block, true
}

func listBlock(node *richdoc.Node, kind string) (Block, bool) {
	items := make([]string, 0, len(node.Content))
	for _, item := range node.Content {
		text := strings.TrimSpace(item.PlainText())
		if text == "" {
			continue
		}
		items = append(items, collapse(text))
		if len(items) >= maxItemsPerBlock {
			break
		}
	}
	if len(items) == 0 {
		return Block{}, false
	}
	block := Block{Kind: kind, Items: items, BlockID: node.AttrString(richdoc.BlockIDAttr)}

	// A list where every line carries a date is a timeline, and a list where
	// every line carries a figure is a set of measures.
	if events := extractEvents(items); len(events) == len(items) && len(events) >= 2 {
		block.Kind = BlockTimeline
		block.Events = events
		return block, true
	}
	if metrics := metricsFromItems(items); len(metrics) == len(items) && len(metrics) >= 2 {
		block.Kind = BlockMetrics
		block.Metrics = metrics
	}
	return block, true
}

func tableBlock(node *richdoc.Node) (Block, bool) {
	rows := make([][]string, 0, len(node.Content))
	header := []string(nil)
	for _, row := range node.Content {
		if row == nil || row.Type != "tableRow" {
			continue
		}
		cells := make([]string, 0, len(row.Content))
		isHeader := len(row.Content) > 0
		for _, cell := range row.Content {
			if cell == nil {
				continue
			}
			if cell.Type != "tableHeader" {
				isHeader = false
			}
			cells = append(cells, collapse(strings.TrimSpace(cell.PlainText())))
		}
		if len(cells) == 0 {
			continue
		}
		if isHeader && header == nil && len(rows) == 0 {
			header = cells
			continue
		}
		rows = append(rows, cells)
		if len(rows) >= maxTableRows {
			break
		}
	}
	if len(rows) == 0 && header == nil {
		return Block{}, false
	}
	return Block{
		Kind: BlockTable, Header: header, Rows: rows,
		BlockID: node.AttrString(richdoc.BlockIDAttr),
	}, true
}

var (
	// A figure with a unit or a percentage, and the words around it.
	metricPattern = regexp.MustCompile(`([^,·\n]{0,24}?)\s*([0-9][0-9,.]*\s*(?:%|퍼센트|배|억원|만원|원|건|명|개|시간|일|주|개월|년|GB|TB|MB))`)
	// A year, a quarter, or a month at the start of a line.
	eventPattern = regexp.MustCompile(`^((?:20[0-9]{2}|19[0-9]{2})(?:\s*년)?(?:\s*(?:Q[1-4]|[1-9][0-2]?\s*월|상반기|하반기))?|Q[1-4]|[1-9][0-2]?월)\s*[:：\-–—]?\s*(.+)$`)
)

func extractMetrics(text string) []Metric {
	matches := metricPattern.FindAllStringSubmatch(text, maxItemsPerBlock)
	metrics := make([]Metric, 0, len(matches))
	for _, match := range matches {
		label := strings.TrimSpace(strings.Trim(match[1], "·,"))
		value := collapse(match[2])
		if value == "" {
			continue
		}
		metrics = append(metrics, Metric{Label: label, Value: value})
	}
	return metrics
}

func metricsFromItems(items []string) []Metric {
	metrics := make([]Metric, 0, len(items))
	for _, item := range items {
		found := extractMetrics(item)
		if len(found) != 1 {
			return nil
		}
		metric := found[0]
		if metric.Label == "" {
			metric.Label = strings.TrimSpace(strings.Replace(item, metric.Value, "", 1))
		}
		metrics = append(metrics, metric)
	}
	return metrics
}

func extractEvents(items []string) []Event {
	events := make([]Event, 0, len(items))
	for _, item := range items {
		match := eventPattern.FindStringSubmatch(item)
		if match == nil {
			return nil
		}
		events = append(events, Event{When: collapse(match[1]), What: strings.TrimSpace(match[2])})
	}
	return events
}

func collectCitations(brief Brief, source BriefSource) []Citation {
	citations := make([]Citation, 0, len(brief.Sections))
	for _, section := range brief.Sections {
		if section.BlockID == "" {
			continue
		}
		citations = append(citations, Citation{
			BlockID:  section.BlockID,
			Section:  section.Title,
			Document: source.Title,
			Revision: source.Revision,
		})
	}
	return citations
}

func collapse(value string) string {
	return strings.Join(strings.FieldsFunc(value, func(r rune) bool {
		return unicode.IsSpace(r)
	}), " ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
