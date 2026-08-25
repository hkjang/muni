import type { Editor } from "@tiptap/react";
import { contentToMarkdown } from "../../../lib/contentMarkdown";
import type { EditorContent } from "../../../lib/markdownContent";
import { markdownToContent } from "../../../lib/markdownContent";

/**
 * selectionMarkdown is what the model is shown: the selected text with its
 * formatting written out as Markdown.
 *
 * Sending plain text is what made "다듬기" strip formatting — bold runs, links
 * and list structure never reached the model, so its answer could not contain
 * them either, and the plain answer replaced the formatted original.
 */
export function selectionMarkdown(
  editor: Editor,
  from: number,
  to: number,
): string {
  const slice = editor.state.doc.slice(from, to);
  const nodes = slice.content.toJSON();
  if (!Array.isArray(nodes))
    return editor.state.doc.textBetween(from, to, "\n", " ").trim();
  return contentToMarkdown(nodes as EditorContent[]).trim();
}

/**
 * toContent turns the model's answer into editor content, reading it as the
 * Markdown that models write whether or not they were asked to.
 *
 * A selection that stayed within one paragraph is written back as inline
 * content so the rewrite replaces the words, not the block they sit in.
 */
export function toContent(
  value: string,
  options: { forceBlocks?: boolean } = {},
): string | EditorContent | EditorContent[] {
  return markdownToContent(value, options);
}

/**
 * SelectionShape records what was selected, so the answer can be written back
 * in the same shape: words inside a sentence stay words, and a heading that is
 * rewritten stays a heading.
 */
export type SelectionShape = {
  /** The selection stayed inside one block, so the answer is inline content. */
  inline: boolean;
  /** The block type the selection covered whole, if it covered exactly one. */
  blockType?: string;
  blockAttrs?: Record<string, unknown>;
};

export function selectionShape(
  editor: Editor,
  from: number,
  to: number,
): SelectionShape {
  const slice = editor.state.doc.slice(from, to);
  const inline =
    slice.content.childCount === 1 && slice.openStart > 0 && slice.openEnd > 0;
  if (inline) return { inline: true };
  if (slice.content.childCount === 1) {
    const only = slice.content.firstChild;
    if (only && only.isTextblock)
      return {
        inline: false,
        blockType: only.type.name,
        blockAttrs: { ...only.attrs },
      };
  }
  return { inline: false };
}

/**
 * resultContent converts the model's answer for the range it replaces.
 *
 * Reshaping matters for the block case: a rewritten heading comes back as an
 * ordinary paragraph unless the original type is put back, which is how an AI
 * edit used to quietly demote headings to body text.
 */
export function resultContent(
  value: string,
  shape: SelectionShape,
): string | EditorContent | EditorContent[] {
  const content = toContent(value, { forceBlocks: !shape.inline });
  if (shape.inline || !shape.blockType || shape.blockType === "paragraph")
    return content;
  if (!Array.isArray(content) || content.length !== 1) return content;
  const only = content[0];
  if (!only || only.type !== "paragraph") return content;
  return [{ ...only, type: shape.blockType, attrs: shape.blockAttrs ?? {} }];
}
