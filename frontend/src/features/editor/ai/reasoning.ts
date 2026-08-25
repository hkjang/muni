const tags = ["think", "thinking", "reasoning"];

export type StreamedAnswer = {
  /** The answer, with the model's working removed. */
  text: string;
  /** The model is still reasoning and has not started answering. */
  thinking: boolean;
};

/**
 * splitReasoning separates a model's working from its answer.
 *
 * Reasoning models emit their working first. Some, notably the Qwen family when
 * reasoning was never asked for, send only the closing tag: the working arrives
 * as ordinary text and ends with `</think>`. Everything before that is not the
 * answer — and the selection menu writes the answer straight into the document,
 * so leaving it in pastes the model's thinking into the page.
 */
export function splitReasoning(raw: string): StreamedAnswer {
  let text = raw;
  let thinking = false;

  for (const tag of tags) {
    const open = `<${tag}>`;
    const close = `</${tag}>`;

    // Complete blocks are removed wherever they sit.
    for (;;) {
      const start = indexOfFold(text, open);
      if (start < 0) break;
      const end = indexOfFold(text.slice(start), close);
      if (end < 0) {
        // Opened and never closed: the answer has not started yet.
        text = text.slice(0, start);
        thinking = true;
        break;
      }
      text = text.slice(0, start) + text.slice(start + end + close.length);
    }

    // A closing tag with no opening one means everything before it was working.
    const last = lastIndexOfFold(text, close);
    if (last >= 0) {
      text = text.slice(last + close.length);
      thinking = false;
    }
  }

  return { text: text.replace(/^\s+/, ""), thinking };
}

function indexOfFold(value: string, needle: string): number {
  return value.toLowerCase().indexOf(needle);
}

function lastIndexOfFold(value: string, needle: string): number {
  return value.toLowerCase().lastIndexOf(needle);
}
