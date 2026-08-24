import { describe, expect, it } from "vitest";
import { Editor } from "@tiptap/core";
import Document from "@tiptap/extension-document";
import Paragraph from "@tiptap/extension-paragraph";
import Text from "@tiptap/extension-text";
import { BlockId, blockIdAttribute } from "../extensions/blockId";
import { applySuggestion, findBlock } from "./applySuggestion";
import type { Suggestion } from "../types";

function editorWith(paragraphs: Array<{ id?: string; text: string }>): Editor {
  return new Editor({
    extensions: [Document, Paragraph, Text, BlockId],
    content: {
      type: "doc",
      content: paragraphs.map((item) => ({
        type: "paragraph",
        attrs: item.id ? { [blockIdAttribute]: item.id } : {},
        content: [{ type: "text", text: item.text }],
      })),
    },
  });
}

function suggestion(overrides: Partial<Suggestion>): Suggestion {
  return {
    id: "s1",
    author: { id: "u1", displayName: "홍길동" },
    range: {},
    newValue: "새 문장",
    status: "PENDING",
    createdAt: "2026-08-24T00:00:00Z",
    ...overrides,
  };
}

describe("findBlock", () => {
  it("locates a block by its identity", () => {
    const editor = editorWith([
      { id: "blk_a", text: "첫 문단" },
      { id: "blk_b", text: "둘째 문단" },
    ]);
    const location = findBlock(editor, "blk_b");
    expect(location).not.toBeNull();
    expect(editor.state.doc.textBetween(location!.from, location!.to, " ")).toContain("둘째");
    editor.destroy();
  });

  it("returns nothing when the block is gone", () => {
    const editor = editorWith([{ id: "blk_a", text: "첫 문단" }]);
    expect(findBlock(editor, "blk_missing")).toBeNull();
    editor.destroy();
  });
});

describe("applySuggestion", () => {
  it("replaces the anchored block even after text was inserted above it", () => {
    const editor = editorWith([
      { id: "blk_a", text: "첫 문단" },
      { id: "blk_b", text: "고칠 문단" },
    ]);
    // The document moves on while the suggestion waits for review.
    editor.commands.insertContentAt(0, {
      type: "paragraph",
      content: [{ type: "text", text: "나중에 앞에 끼워 넣은 문단" }],
    });

    const outcome = applySuggestion(
      editor,
      suggestion({ blockId: "blk_b", newValue: "고쳐진 문단" }),
    );
    expect(outcome.applied).toBe(true);
    const text = editor.getText();
    expect(text).toContain("고쳐진 문단");
    expect(text).not.toContain("고칠 문단");
    // The block above must be untouched.
    expect(text).toContain("첫 문단");
    editor.destroy();
  });

  it("refuses when the anchored block no longer exists", () => {
    const editor = editorWith([{ id: "blk_a", text: "남은 문단" }]);
    const outcome = applySuggestion(
      editor,
      suggestion({ blockId: "blk_gone", newValue: "무엇이든" }),
    );
    expect(outcome).toEqual({ applied: false, reason: "block-gone" });
    expect(editor.getText()).toContain("남은 문단");
    editor.destroy();
  });

  it("reads the anchor out of the stored range as well", () => {
    const editor = editorWith([{ id: "blk_a", text: "원래 문단" }]);
    const outcome = applySuggestion(
      editor,
      suggestion({ range: { blockId: "blk_a" }, newValue: "바뀐 문단" }),
    );
    expect(outcome.applied).toBe(true);
    expect(editor.getText()).toContain("바뀐 문단");
    editor.destroy();
  });

  it("falls back to the stored position for suggestions made before block ids", () => {
    const editor = editorWith([{ text: "예전 방식 문단" }]);
    const outcome = applySuggestion(
      editor,
      suggestion({ range: { from: 1, to: 8 }, newValue: "교체" }),
    );
    expect(outcome.applied).toBe(true);
    expect(editor.getText()).toContain("교체");
    editor.destroy();
  });

  it("refuses a stored position that is past the end of the document", () => {
    const editor = editorWith([{ text: "짧은 문단" }]);
    const outcome = applySuggestion(
      editor,
      suggestion({ range: { from: 900, to: 950 }, newValue: "교체" }),
    );
    expect(outcome).toEqual({ applied: false, reason: "block-gone" });
    editor.destroy();
  });

  it("refuses a suggestion with no replacement text", () => {
    const editor = editorWith([{ id: "blk_a", text: "문단" }]);
    expect(
      applySuggestion(editor, suggestion({ blockId: "blk_a", newValue: "   " })),
    ).toEqual({ applied: false, reason: "no-text" });
    editor.destroy();
  });
});
