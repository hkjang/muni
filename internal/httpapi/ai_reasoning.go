package httpapi

import "strings"

// reasoningTags are the wrappers models put their thinking in.
var reasoningTags = []string{"think", "thinking", "reasoning"}

// stripReasoning removes a model's thinking from its answer.
//
// Reasoning models emit their working before the answer. Some of them, notably
// the Qwen family when reasoning was never asked for, emit only the closing
// tag: the thinking arrives as plain text and ends with </think>. Everything up
// to that point is working, not answer, and muni writes answers into documents
// — so leaving it in means the thinking gets pasted into the page.
func stripReasoning(value string) string {
	for _, tag := range reasoningTags {
		value = stripTagPairs(value, tag)
		value = stripDanglingClose(value, tag)
	}
	return strings.TrimSpace(value)
}

// stripTagPairs removes complete <tag>…</tag> blocks.
func stripTagPairs(value, tag string) string {
	open, close := "<"+tag+">", "</"+tag+">"
	for {
		start := indexFold(value, open)
		if start < 0 {
			return value
		}
		end := indexFold(value[start:], close)
		if end < 0 {
			// An unterminated block is thinking that never finished; the answer
			// has not started yet.
			return value[:start]
		}
		value = value[:start] + value[start+end+len(close):]
	}
}

// stripDanglingClose drops everything up to a closing tag that had no opening
// one, which is what an answer looks like when the model was not asked to
// reason but did anyway.
func stripDanglingClose(value, tag string) string {
	close := "</" + tag + ">"
	last := lastIndexFold(value, close)
	if last < 0 {
		return value
	}
	return value[last+len(close):]
}

func indexFold(value, needle string) int {
	return strings.Index(strings.ToLower(value), needle)
}

func lastIndexFold(value, needle string) int {
	return strings.LastIndex(strings.ToLower(value), needle)
}
