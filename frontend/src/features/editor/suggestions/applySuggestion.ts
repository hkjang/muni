import type { Editor } from "@tiptap/react";
import type { Suggestion } from "../types";
import { blockIdAttribute } from "../extensions/blockId";

export type BlockLocation = { from: number; to: number };

/**
 * findBlock locates a block by its stable identity. A suggestion may sit in the
 * review queue while the document keeps changing, and by then a stored document
 * position points at whatever happens to be there now.
 */
export function findBlock(editor: Editor, blockId: string): BlockLocation | null {
  let found: BlockLocation | null = null;
  editor.state.doc.descendants((node, position) => {
    if (found) return false;
    if (node.attrs?.[blockIdAttribute] === blockId) {
      found = { from: position, to: position + node.nodeSize };
      return false;
    }
    return true;
  });
  return found;
}

export type ApplyOutcome =
  | { applied: true }
  | { applied: false; reason: "not-anchored" | "block-gone" | "no-text" };

/**
 * applySuggestion replaces the suggested block, preferring the block anchor and
 * falling back to the stored position range for suggestions written before
 * blocks carried an identity.
 */
export function applySuggestion(
  editor: Editor,
  suggestion: Suggestion,
): ApplyOutcome {
  if (typeof suggestion.newValue !== "string" || !suggestion.newValue.trim())
    return { applied: false, reason: "no-text" };

  const anchor = suggestion.blockId ?? suggestion.range?.blockId;
  if (anchor) {
    const location = findBlock(editor, anchor);
    if (!location) return { applied: false, reason: "block-gone" };
    editor
      .chain()
      .focus()
      .insertContentAt(location, {
        type: "paragraph",
        content: [{ type: "text", text: suggestion.newValue }],
      })
      .run();
    return { applied: true };
  }

  const { from, to } = suggestion.range ?? {};
  if (typeof from !== "number" || typeof to !== "number")
    return { applied: false, reason: "not-anchored" };
  const size = editor.state.doc.content.size;
  if (from > size || to > size) return { applied: false, reason: "block-gone" };
  editor.chain().focus().insertContentAt({ from, to }, suggestion.newValue).run();
  return { applied: true };
}
