import type { MDBlock, MDInline, MDMark } from "./markdown";
import { looksLikeMarkdown, parseMarkdown } from "./markdown";

/**
 * Editor content, in the JSON shape Tiptap accepts. It is deliberately loose:
 * the schema is checked by the editor when the content is inserted.
 */
export type EditorContent = Record<string, unknown>;

const markNames: Record<MDMark["type"], string> = {
  bold: "bold",
  italic: "italic",
  code: "code",
  strike: "strike",
  link: "link",
};

function markJSON(mark: MDMark): EditorContent {
  if (mark.type === "link")
    return { type: "link", attrs: { href: mark.href, target: "_blank" } };
  return { type: markNames[mark.type] };
}

function inlineJSON(inline: MDInline[]): EditorContent[] {
  const out: EditorContent[] = [];
  for (const piece of inline) {
    // A newline inside a paragraph was a soft break in the source.
    const segments = piece.text.split("\n");
    segments.forEach((segment, index) => {
      if (index > 0) out.push({ type: "hardBreak" });
      if (!segment) return;
      out.push(
        piece.marks.length > 0
          ? { type: "text", text: segment, marks: piece.marks.map(markJSON) }
          : { type: "text", text: segment },
      );
    });
  }
  return out;
}

function blockJSON(block: MDBlock): EditorContent | EditorContent[] {
  switch (block.type) {
    case "heading":
      return {
        type: "heading",
        attrs: { level: Math.min(6, Math.max(1, block.level)) },
        content: inlineJSON(block.inline),
      };
    case "paragraph": {
      const content = inlineJSON(block.inline);
      return content.length > 0
        ? { type: "paragraph", content }
        : { type: "paragraph" };
    }
    case "rule":
      return { type: "horizontalRule" };
    case "code":
      return {
        type: "codeBlock",
        attrs: block.language ? { language: block.language } : {},
        content: block.text ? [{ type: "text", text: block.text }] : [],
      };
    case "quote":
      return { type: "blockquote", content: blocksJSON(block.blocks) };
    case "list": {
      // A list whose items are all checkboxes is a task list, which is a
      // different node in the editor.
      const tasks = block.items.every((item) => item.checked !== undefined);
      if (tasks && block.items.length > 0)
        return {
          type: "taskList",
          content: block.items.map((item) => ({
            type: "taskItem",
            attrs: { checked: Boolean(item.checked) },
            content: blocksJSON(item.blocks),
          })),
        };
      return {
        type: block.ordered ? "orderedList" : "bulletList",
        ...(block.ordered && block.start !== 1
          ? { attrs: { start: block.start } }
          : {}),
        content: block.items.map((item) => ({
          type: "listItem",
          content: blocksJSON(item.blocks),
        })),
      };
    }
    case "table": {
      const row = (cells: MDInline[][], header: boolean) => ({
        type: "tableRow",
        content: cells.map((cell) => ({
          type: header ? "tableHeader" : "tableCell",
          content: [{ type: "paragraph", content: inlineJSON(cell) }],
        })),
      });
      const width = block.header.length;
      const body = block.rows.map((cells) => {
        const padded = [...cells];
        while (padded.length < width) padded.push([]);
        return row(padded.slice(0, width), false);
      });
      return { type: "table", content: [row(block.header, true), ...body] };
    }
  }
}

function blocksJSON(blocks: MDBlock[]): EditorContent[] {
  const out: EditorContent[] = [];
  for (const block of blocks) {
    const converted = blockJSON(block);
    if (Array.isArray(converted)) out.push(...converted);
    else out.push(converted);
  }
  // An empty item still needs a node, or the editor rejects the insertion.
  return out.length > 0 ? out : [{ type: "paragraph" }];
}

/**
 * markdownToContent turns a model's answer into editor content.
 *
 * A single short line comes back as a plain string so that rewriting a few
 * words inside a sentence replaces the words rather than the paragraph — that
 * distinction is what keeps "다듬기" from flattening the surrounding formatting.
 */
export function markdownToContent(
  value: string,
  options: { forceBlocks?: boolean } = {},
): string | EditorContent | EditorContent[] {
  const source = value.trim();
  if (!source) return "";

  const blocks = parseMarkdown(source);
  if (!options.forceBlocks && blocks.length === 1) {
    const only = blocks[0];
    if (
      only?.type === "paragraph" &&
      !only.inline.some((piece) => piece.text.includes("\n"))
    ) {
      const inline = inlineJSON(only.inline);
      // No marks at all: hand back the string, which keeps the surrounding
      // formatting of the replaced range intact.
      if (inline.every((node) => !("marks" in node))) return source;
      return inline;
    }
  }
  return blocksJSON(blocks);
}

export { looksLikeMarkdown };
