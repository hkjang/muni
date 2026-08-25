import type { Editor } from "@tiptap/react";
import { blockIdAttribute } from "../extensions/blockId";
import { findMatches } from "../find/findMatches";

/**
 * Where a comment is attached.
 *
 * Positions alone do not survive editing: insert a paragraph above and every
 * offset below it moves, so a comment written on the third line ends up
 * pointing at the second. The block id is stable, and the text that was
 * selected locates the comment inside that block, so a comment keeps pointing
 * at what it was about.
 */
export type CommentAnchor = {
  from?: number;
  to?: number;
  blockId?: string;
  selectedText?: string;
};

export type AnchorRange = { from: number; to: number };

/**
 * resolveWithin locates the commented text inside a block.
 *
 * Returns the whole block when the text is no longer there — the comment is
 * still about this block, and taking the reader to it is more use than
 * refusing to move at all.
 */
export function resolveWithin(
  blockText: string,
  selected: string,
  blockFrom: number,
  blockTo: number,
): AnchorRange {
  const needle = selected.trim();
  if (!needle) return { from: blockFrom, to: blockTo };
  const found = findMatches(blockText, needle)[0];
  if (!found) return { from: blockFrom, to: blockTo };
  return { from: blockFrom + found.start, to: blockFrom + found.end };
}

/** readAnchor reads the stored shape, which older comments only half fill in. */
export function readAnchor(value: unknown): CommentAnchor {
  if (!value || typeof value !== "object") return {};
  const record = value as Record<string, unknown>;
  return {
    from: typeof record.from === "number" ? record.from : undefined,
    to: typeof record.to === "number" ? record.to : undefined,
    blockId: typeof record.blockId === "string" ? record.blockId : undefined,
    selectedText:
      typeof record.selectedText === "string" ? record.selectedText : undefined,
  };
}

/**
 * locateAnchor finds the range a comment points at in the document as it is
 * now, preferring the stable block id and falling back through the stored
 * text to the raw positions.
 */
export function locateAnchor(editor: Editor, anchor: CommentAnchor): AnchorRange | null {
  const doc = editor.state.doc;

  if (anchor.blockId) {
    let found: AnchorRange | null = null;
    doc.descendants((node, pos) => {
      if (found) return false;
      if (node.attrs?.[blockIdAttribute] === anchor.blockId) {
        // +1 steps inside the block, past its opening token.
        found = resolveWithin(
          node.textContent,
          anchor.selectedText ?? "",
          pos + 1,
          pos + 1 + node.content.size,
        );
        return false;
      }
      return true;
    });
    if (found) return found;
  }

  const selected = anchor.selectedText?.trim();
  if (selected) {
    // The block is gone, or there never was an id. The text itself is the next
    // best handle.
    const whole = doc.textBetween(0, doc.content.size, "\n", " ");
    const match = findMatches(whole, selected)[0];
    if (match) {
      const stored = storedRange(doc.content.size, anchor);
      if (stored && doc.textBetween(stored.from, stored.to, "\n", " ") === selected)
        return stored;
      // Positions in a flat string are not editor positions; walk to find it.
      return searchDocument(editor, selected);
    }
  }

  return storedRange(doc.content.size, anchor);
}

function storedRange(size: number, anchor: CommentAnchor): AnchorRange | null {
  if (
    typeof anchor.from !== "number" ||
    typeof anchor.to !== "number" ||
    anchor.from < 0 ||
    anchor.to > size ||
    anchor.from >= anchor.to
  )
    return null;
  return { from: anchor.from, to: anchor.to };
}

function searchDocument(editor: Editor, needle: string): AnchorRange | null {
  let result: AnchorRange | null = null;
  editor.state.doc.descendants((node, pos) => {
    if (result) return false;
    if (!node.isTextblock) return true;
    const index = node.textContent.indexOf(needle);
    if (index >= 0) {
      result = { from: pos + 1 + index, to: pos + 1 + index + needle.length };
      return false;
    }
    return true;
  });
  return result;
}

/** blockIdAt is the id of the block the caret sits in, if it has one. */
export function blockIdAt(editor: Editor, pos: number): string | undefined {
  const resolved = editor.state.doc.resolve(Math.min(pos, editor.state.doc.content.size));
  for (let depth = resolved.depth; depth > 0; depth -= 1) {
    const id = resolved.node(depth).attrs?.[blockIdAttribute];
    if (typeof id === "string" && id) return id;
  }
  return undefined;
}
