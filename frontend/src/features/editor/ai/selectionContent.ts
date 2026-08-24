/**
 * toContent turns the model's plain-text answer into editor content. A single
 * paragraph is inserted as inline text so a rewrite stays inside the sentence
 * it replaced; anything longer becomes real paragraphs and list items.
 */
type JSONContent = Record<string, unknown>;

export function toContent(value: string): string | JSONContent | JSONContent[] {
  const blocks = value
    .split(/\n{2,}/)
    .map((block) => block.trim())
    .filter(Boolean);
  const first = blocks[0];
  if (!first) return "";

  const bulletOnly = blocks.every((block) =>
    block.split("\n").every((line) => /^[-*]\s+/.test(line.trim())),
  );
  if (bulletOnly) {
    const items = blocks
      .flatMap((block) => block.split("\n"))
      .map((line) => line.trim().replace(/^[-*]\s+/, ""))
      .filter(Boolean);
    if (items.length > 0) {
      return {
        type: "bulletList",
        content: items.map((item) => ({
          type: "listItem",
          content: [{ type: "paragraph", content: [{ type: "text", text: item }] }],
        })),
      };
    }
  }

  if (blocks.length === 1 && !first.includes("\n")) return first;

  return blocks.map((block) => ({
    type: "paragraph",
    content: withLineBreaks(block),
  }));
}

function withLineBreaks(block: string): JSONContent[] {
  const content: JSONContent[] = [];
  block.split("\n").forEach((line, index) => {
    if (index > 0) content.push({ type: "hardBreak" });
    if (line) content.push({ type: "text", text: line });
  });
  return content;
}
