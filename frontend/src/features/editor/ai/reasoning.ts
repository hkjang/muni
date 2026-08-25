const tags = ["think", "thinking", "reasoning"];

export type StreamedAnswer = {
  /** The answer, with the model's working removed. */
  text: string;
  /** The model is still reasoning and has not started answering. */
  thinking: boolean;
  /** The working itself, so a reader who wants it can open it. */
  reasoning: string;
};

/**
 * splitReasoning separates a model's working from its answer.
 *
 * Reasoning models emit their working first. Some, notably the Qwen family when
 * reasoning was never asked for, send only the closing tag: the working arrives
 * as ordinary text and ends with `</think>`. Everything before that is not the
 * answer — and the selection menu writes the answer straight into the document,
 * so leaving it in pastes the model's thinking into the page.
 *
 * The working is kept rather than dropped: the agent panel shows it as it
 * arrives and folds it away once the answer starts.
 */
export function splitReasoning(raw: string): StreamedAnswer {
  let text = raw;
  let thinking = false;
  const working: string[] = [];

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
        working.push(text.slice(start + open.length));
        text = text.slice(0, start);
        thinking = true;
        break;
      }
      working.push(text.slice(start + open.length, start + end));
      text = text.slice(0, start) + text.slice(start + end + close.length);
    }

    // A closing tag with no opening one means everything before it was working.
    const last = lastIndexOfFold(text, close);
    if (last >= 0) {
      working.unshift(text.slice(0, last));
      text = text.slice(last + close.length);
      thinking = false;
    }
  }

  return {
    text: text.replace(/^\s+/, ""),
    thinking,
    reasoning: working.join("\n").trim(),
  };
}

function indexOfFold(value: string, needle: string): number {
  return value.toLowerCase().indexOf(needle);
}

function lastIndexOfFold(value: string, needle: string): number {
  return value.toLowerCase().lastIndexOf(needle);
}
