package ptium

import (
	"fmt"
	"strings"
)

// maxPromptChars matches the limit Ptium accepts for a generation prompt.
const maxPromptChars = 12000

// RenderPrompt turns the brief into the text Ptium generates from.
//
// Ptium designs the storyline itself, so the prompt's job is not to describe
// slides — it is to hand over the document's meaning without flattening it.
// Labelling a list as steps or a set of figures as measures is what lets the
// generator reach for a process diagram or a KPI row instead of a wall of
// bullets.
func RenderPrompt(brief Brief) string {
	var out strings.Builder

	out.WriteString("아래 문서를 바탕으로 발표자료를 만들어 주세요.\n\n")
	out.WriteString("문서 제목: " + brief.Source.Title + "\n")
	if brief.Presentation.Purpose != "" {
		out.WriteString("발표 목적: " + brief.Presentation.Purpose + "\n")
	}
	if brief.Presentation.Minutes > 0 {
		fmt.Fprintf(&out, "발표 시간: %d분\n", brief.Presentation.Minutes)
	}
	if brief.Presentation.Detail != "" {
		out.WriteString("상세 수준: " + brief.Presentation.Detail + "\n")
	}
	out.WriteString("\n원문은 문서 전체가 아니라 발표에 필요한 구조만 정리한 것입니다. " +
		"목록과 표, 수치는 그 형태를 살려 배치해 주세요.\n\n")
	out.WriteString("---\n")

	for _, section := range brief.Sections {
		if section.Title != "" {
			fmt.Fprintf(&out, "\n## %s\n", section.Title)
		}
		for _, block := range section.Blocks {
			out.WriteString(renderBlock(block))
		}
		if out.Len() > maxPromptChars {
			break
		}
	}

	prompt := strings.TrimRight(out.String(), "\n")
	if len(prompt) > maxPromptChars {
		// Cut on a line boundary so the last section is not left half written.
		prompt = prompt[:maxPromptChars]
		if cut := strings.LastIndexByte(prompt, '\n'); cut > maxPromptChars/2 {
			prompt = prompt[:cut]
		}
		prompt += "\n\n(문서가 길어 이후 내용은 생략했습니다.)"
	}
	return prompt
}

func renderBlock(block Block) string {
	var out strings.Builder
	switch block.Kind {
	case BlockParagraph:
		out.WriteString(block.Text + "\n")
	case BlockQuote:
		out.WriteString("인용: " + block.Text + "\n")
	case BlockCode:
		out.WriteString("코드:\n" + block.Text + "\n")
	case BlockBullets:
		out.WriteString("항목:\n")
		for _, item := range block.Items {
			out.WriteString("- " + item + "\n")
		}
	case BlockSteps:
		out.WriteString("순서(단계로 표현하면 좋습니다):\n")
		for index, item := range block.Items {
			fmt.Fprintf(&out, "%d. %s\n", index+1, item)
		}
	case BlockMetrics:
		out.WriteString("수치(지표로 강조하면 좋습니다):\n")
		for _, metric := range block.Metrics {
			if metric.Label != "" {
				out.WriteString("- " + metric.Label + ": " + metric.Value + "\n")
				continue
			}
			out.WriteString("- " + metric.Value + "\n")
		}
	case BlockTimeline:
		out.WriteString("일정(타임라인으로 표현하면 좋습니다):\n")
		for _, event := range block.Events {
			out.WriteString("- " + event.When + ": " + event.What + "\n")
		}
	case BlockTable:
		out.WriteString("표:\n")
		if len(block.Header) > 0 {
			out.WriteString("| " + strings.Join(block.Header, " | ") + " |\n")
		}
		for _, row := range block.Rows {
			out.WriteString("| " + strings.Join(row, " | ") + " |\n")
		}
	case BlockImage:
		if block.Alt != "" {
			out.WriteString("그림: " + block.Alt + "\n")
		}
	}
	return out.String()
}
