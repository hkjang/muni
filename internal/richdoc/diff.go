package richdoc

import (
	"strings"
	"unicode"
)

// Block-level outcomes of comparing two revisions.
const (
	BlockUnchanged = "unchanged"
	BlockAdded     = "added"
	BlockRemoved   = "removed"
	BlockChanged   = "changed"
	BlockMoved     = "moved"
)

// InlineOp is one run of a word-level comparison of a changed block.
type InlineOp struct {
	Op   string `json:"op"` // equal | insert | delete
	Text string `json:"text"`
}

type BlockDiff struct {
	Status    string     `json:"status"`
	BlockID   string     `json:"blockId,omitempty"`
	Type      string     `json:"type"`
	Before    string     `json:"before,omitempty"`
	After     string     `json:"after,omitempty"`
	Inline    []InlineOp `json:"inline,omitempty"`
	FromIndex int        `json:"fromIndex"`
	ToIndex   int        `json:"toIndex"`
}

type DiffSummary struct {
	Added     int `json:"added"`
	Removed   int `json:"removed"`
	Changed   int `json:"changed"`
	Moved     int `json:"moved"`
	Unchanged int `json:"unchanged"`
}

type DiffResult struct {
	Summary DiffSummary `json:"summary"`
	Blocks  []BlockDiff `json:"blocks"`
}

// block is one comparable unit of a document.
type block struct {
	id    string
	kind  string
	text  string
	index int
}

// leafTypes are the blocks that carry text directly. Containers are walked
// through so a paragraph inside a list item is compared as itself; a table is
// compared whole because splitting it into cells loses the shape a reader sees.
var leafTypes = map[string]bool{
	"paragraph":      true,
	"heading":        true,
	"codeBlock":      true,
	"image":          true,
	"horizontalRule": true,
	"pageBreak":      true,
}

func flatten(document *Node) []block {
	blocks := make([]block, 0, 32)
	var walk func(*Node)
	walk = func(node *Node) {
		if node == nil {
			return
		}
		switch {
		case node.Type == "table":
			blocks = append(blocks, newBlock(node, tableText(node), len(blocks)))
			return
		case leafTypes[node.Type]:
			blocks = append(blocks, newBlock(node, blockText(node), len(blocks)))
			return
		}
		for _, child := range node.Content {
			walk(child)
		}
	}
	walk(document)
	return blocks
}

func newBlock(node *Node, text string, index int) block {
	id, _ := node.Attr(BlockIDAttr).(string)
	return block{id: id, kind: node.Type, text: text, index: index}
}

func blockText(node *Node) string {
	if node.Type == "image" {
		if alt := node.AttrString("alt"); alt != "" {
			return "[" + alt + "]"
		}
		return "[" + node.AttrString("src") + "]"
	}
	if node.Type == "horizontalRule" {
		return "---"
	}
	if node.Type == "pageBreak" {
		return "[페이지 나누기]"
	}
	var out strings.Builder
	var walk func(*Node)
	walk = func(current *Node) {
		if current.Type == "text" {
			out.WriteString(current.Text)
		}
		if current.Type == "hardBreak" {
			out.WriteString("\n")
		}
		for _, child := range current.Content {
			walk(child)
		}
	}
	walk(node)
	return out.String()
}

func tableText(node *Node) string {
	rows := make([]string, 0, len(node.Content))
	for _, row := range node.Content {
		cells := make([]string, 0, len(row.Content))
		for _, cell := range row.Content {
			cells = append(cells, strings.TrimSpace(blockText(cell)))
		}
		rows = append(rows, strings.Join(cells, " | "))
	}
	return strings.Join(rows, "\n")
}

// Diff compares two revisions block by block.
//
// Blocks are paired by their stable id first, which is what keeps an edit from
// reading as "everything below this point changed" when a paragraph is inserted
// above. Documents written before ids existed fall back to matching identical
// text, then to the most similar remaining block of the same type.
func Diff(before, after *Node) DiffResult {
	fromBlocks := flatten(before)
	toBlocks := flatten(after)

	pairs := matchBlocks(fromBlocks, toBlocks)
	moved := detectMoves(pairs)

	result := DiffResult{Blocks: make([]BlockDiff, 0, len(toBlocks)+len(fromBlocks))}
	matchedFrom := make([]bool, len(fromBlocks))
	pairedTo := make(map[int]int, len(pairs))
	for _, pair := range pairs {
		matchedFrom[pair.from] = true
		pairedTo[pair.to] = pair.from
	}

	// Removed blocks are reported at the position they used to hold, so the
	// result reads in document order.
	nextRemoved := 0
	emitRemovedBefore := func(fromIndex int) {
		for nextRemoved < len(fromBlocks) && nextRemoved < fromIndex {
			if !matchedFrom[nextRemoved] {
				item := fromBlocks[nextRemoved]
				result.Blocks = append(result.Blocks, BlockDiff{
					Status: BlockRemoved, BlockID: item.id, Type: item.kind,
					Before: item.text, FromIndex: item.index, ToIndex: -1,
				})
				result.Summary.Removed++
			}
			nextRemoved++
		}
	}

	for toIndex, item := range toBlocks {
		fromIndex, paired := pairedTo[toIndex]
		if !paired {
			result.Blocks = append(result.Blocks, BlockDiff{
				Status: BlockAdded, BlockID: item.id, Type: item.kind,
				After: item.text, FromIndex: -1, ToIndex: item.index,
			})
			result.Summary.Added++
			continue
		}
		emitRemovedBefore(fromIndex)
		nextRemoved = fromIndex + 1
		source := fromBlocks[fromIndex]

		entry := BlockDiff{
			BlockID: item.id, Type: item.kind,
			Before: source.text, After: item.text,
			FromIndex: source.index, ToIndex: item.index,
		}
		if entry.BlockID == "" {
			entry.BlockID = source.id
		}
		switch {
		case source.text != item.text:
			entry.Status = BlockChanged
			entry.Inline = InlineDiff(source.text, item.text)
			result.Summary.Changed++
		case moved[toIndex]:
			entry.Status = BlockMoved
			result.Summary.Moved++
		default:
			entry.Status = BlockUnchanged
			result.Summary.Unchanged++
		}
		result.Blocks = append(result.Blocks, entry)
	}
	emitRemovedBefore(len(fromBlocks))

	return result
}

type pair struct{ from, to int }

func matchBlocks(fromBlocks, toBlocks []block) []pair {
	usedFrom := make([]bool, len(fromBlocks))
	usedTo := make([]bool, len(toBlocks))
	pairs := make([]pair, 0, len(toBlocks))

	// 1. Stable ids. Only ids that appear exactly once on each side are
	//    trustworthy; a duplicate means something copied a block.
	fromByID := uniqueIndex(fromBlocks)
	toByID := uniqueIndex(toBlocks)
	for id, fromIndex := range fromByID {
		if toIndex, ok := toByID[id]; ok {
			pairs = append(pairs, pair{from: fromIndex, to: toIndex})
			usedFrom[fromIndex] = true
			usedTo[toIndex] = true
		}
	}

	// 2. Identical text, in order. This carries documents written before ids
	//    existed, and re-anchors blocks whose id was lost.
	for toIndex, item := range toBlocks {
		if usedTo[toIndex] {
			continue
		}
		for fromIndex, source := range fromBlocks {
			if usedFrom[fromIndex] || source.kind != item.kind || source.text != item.text {
				continue
			}
			pairs = append(pairs, pair{from: fromIndex, to: toIndex})
			usedFrom[fromIndex] = true
			usedTo[toIndex] = true
			break
		}
	}

	// 3. Most similar remaining block of the same type, so an edited paragraph
	//    reads as a change rather than a delete followed by an insert.
	pairs = append(pairs, matchBySimilarity(fromBlocks, toBlocks, usedFrom, usedTo)...)

	sortPairsByTo(pairs)
	return pairs
}

// similarityBudget caps the quadratic fallback; large documents rely on ids.
const similarityBudget = 40000

func matchBySimilarity(fromBlocks, toBlocks []block, usedFrom, usedTo []bool) []pair {
	remainingFrom := make([]int, 0)
	for index := range fromBlocks {
		if !usedFrom[index] {
			remainingFrom = append(remainingFrom, index)
		}
	}
	remainingTo := make([]int, 0)
	for index := range toBlocks {
		if !usedTo[index] {
			remainingTo = append(remainingTo, index)
		}
	}
	if len(remainingFrom)*len(remainingTo) > similarityBudget {
		return nil
	}

	pairs := make([]pair, 0)
	for _, toIndex := range remainingTo {
		best, bestScore := -1, 0.0
		for _, fromIndex := range remainingFrom {
			if usedFrom[fromIndex] || fromBlocks[fromIndex].kind != toBlocks[toIndex].kind {
				continue
			}
			score := similarity(fromBlocks[fromIndex].text, toBlocks[toIndex].text)
			if score > bestScore {
				best, bestScore = fromIndex, score
			}
		}
		// Below half the content in common the blocks are different enough
		// that reporting an insert and a delete is more honest than a change.
		if best >= 0 && bestScore >= 0.5 {
			pairs = append(pairs, pair{from: best, to: toIndex})
			usedFrom[best] = true
			usedTo[toIndex] = true
		}
	}
	return pairs
}

func uniqueIndex(blocks []block) map[string]int {
	seen := map[string]int{}
	duplicated := map[string]bool{}
	for index, item := range blocks {
		if item.id == "" {
			continue
		}
		if _, exists := seen[item.id]; exists {
			duplicated[item.id] = true
			continue
		}
		seen[item.id] = index
	}
	for id := range duplicated {
		delete(seen, id)
	}
	return seen
}

func sortPairsByTo(pairs []pair) {
	for index := 1; index < len(pairs); index++ {
		for cursor := index; cursor > 0 && pairs[cursor].to < pairs[cursor-1].to; cursor-- {
			pairs[cursor], pairs[cursor-1] = pairs[cursor-1], pairs[cursor]
		}
	}
}

// detectMoves marks the paired blocks that had to be reordered. Everything
// outside the longest run that kept its relative order counts as moved, which
// is the smallest honest set.
func detectMoves(pairs []pair) map[int]bool {
	moved := map[int]bool{}
	if len(pairs) < 2 {
		return moved
	}
	sequence := make([]int, len(pairs))
	for index, item := range pairs {
		sequence[index] = item.from
	}
	keep := longestIncreasing(sequence)
	inPlace := make(map[int]bool, len(keep))
	for _, index := range keep {
		inPlace[index] = true
	}
	for index, item := range pairs {
		if !inPlace[index] {
			moved[item.to] = true
		}
	}
	return moved
}

// longestIncreasing returns the positions of a longest strictly increasing
// subsequence.
func longestIncreasing(values []int) []int {
	if len(values) == 0 {
		return nil
	}
	tails := make([]int, 0, len(values))
	previous := make([]int, len(values))
	for index, value := range values {
		previous[index] = -1
		low, high := 0, len(tails)
		for low < high {
			middle := (low + high) / 2
			if values[tails[middle]] < value {
				low = middle + 1
			} else {
				high = middle
			}
		}
		if low > 0 {
			previous[index] = tails[low-1]
		}
		if low == len(tails) {
			tails = append(tails, index)
		} else {
			tails[low] = index
		}
	}
	out := make([]int, len(tails))
	cursor := tails[len(tails)-1]
	for position := len(tails) - 1; position >= 0; position-- {
		out[position] = cursor
		cursor = previous[cursor]
	}
	return out
}

func similarity(left, right string) float64 {
	if left == right {
		return 1
	}
	if left == "" || right == "" {
		return 0
	}
	a, b := tokenize(left), tokenize(right)
	if len(a)*len(b) > similarityBudget {
		return 0
	}
	common := lcsLength(a, b)
	return 2 * float64(common) / float64(len(a)+len(b))
}

// inlineBudget caps the word-level comparison; beyond it the block is reported
// as changed without a breakdown rather than spending seconds on one paragraph.
const inlineBudget = 1 << 20

// InlineDiff compares two block texts word by word.
func InlineDiff(before, after string) []InlineOp {
	a, b := tokenize(before), tokenize(after)
	if len(a)*len(b) > inlineBudget {
		return compactOps([]InlineOp{{Op: "delete", Text: before}, {Op: "insert", Text: after}})
	}
	table := lcsTable(a, b)
	ops := make([]InlineOp, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			ops = append(ops, InlineOp{Op: "equal", Text: a[i]})
			i++
			j++
		case table[i+1][j] >= table[i][j+1]:
			ops = append(ops, InlineOp{Op: "delete", Text: a[i]})
			i++
		default:
			ops = append(ops, InlineOp{Op: "insert", Text: b[j]})
			j++
		}
	}
	for ; i < len(a); i++ {
		ops = append(ops, InlineOp{Op: "delete", Text: a[i]})
	}
	for ; j < len(b); j++ {
		ops = append(ops, InlineOp{Op: "insert", Text: b[j]})
	}
	return compactOps(ops)
}

func compactOps(ops []InlineOp) []InlineOp {
	out := make([]InlineOp, 0, len(ops))
	for _, op := range ops {
		if op.Text == "" {
			continue
		}
		if len(out) > 0 && out[len(out)-1].Op == op.Op {
			out[len(out)-1].Text += op.Text
			continue
		}
		out = append(out, op)
	}
	return out
}

func lcsTable(a, b []string) [][]int {
	table := make([][]int, len(a)+1)
	for index := range table {
		table[index] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				table[i][j] = table[i+1][j+1] + 1
			} else if table[i+1][j] >= table[i][j+1] {
				table[i][j] = table[i+1][j]
			} else {
				table[i][j] = table[i][j+1]
			}
		}
	}
	return table
}

func lcsLength(a, b []string) int {
	return lcsTable(a, b)[0][0]
}

// tokenize splits text into comparison units: a word at a time for scripts
// that use spaces, a character at a time for CJK, which does not.
func tokenize(value string) []string {
	tokens := make([]string, 0, len(value)/3+1)
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}
	var previousSpace bool
	for _, r := range value {
		switch {
		case isCJK(r):
			flush()
			tokens = append(tokens, string(r))
			previousSpace = false
		case unicode.IsSpace(r):
			if !previousSpace {
				flush()
			}
			current.WriteRune(r)
			previousSpace = true
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if previousSpace {
				flush()
			}
			current.WriteRune(r)
			previousSpace = false
		default:
			flush()
			tokens = append(tokens, string(r))
			previousSpace = false
		}
	}
	flush()
	return tokens
}

func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hangul, r) ||
		unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r)
}
