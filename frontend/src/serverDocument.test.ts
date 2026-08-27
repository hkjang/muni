import { Editor } from "@tiptap/core";
import { describe, expect, it } from "vitest";
import everyNodeFixture from "../testdata/every-node.json";
import { documentExtensions } from "./features/editor/documentExtensions";

/**
 * The server's own coverage fixture, loaded into the editor that has to open
 * it. testdata/every-node.json is the same file internal/httpapi and
 * internal/docx read; keeping one copy is the point, because the copy that
 * drifts is the one that stops finding things.
 *
 * editorSchema.test.ts checks vocabulary — every mark, node and attribute the
 * server can name. This checks something vocabulary cannot: whether the
 * *shape* is one the schema can hold. A picture left inside the paragraph that
 * held it used only known nodes and known attributes, carried every phrase
 * through every export, and still threw the moment the editor opened it.
 */
const everyNode = everyNodeFixture as {
  type: string;
  content: { type: string }[];
};

/** The phrases the Go tests follow through each export format. */
const carriedPhrases = [
  "제목입니다",
  "평문과",
  "굵은글씨",
  "기울임",
  "밑줄",
  "취소선",
  "코드조각",
  "링크글자",
  "위첨자표시",
  "아래첨자표시",
  "형광펜표시",
  "글자서식표시",
  "각주내용입니다",
  "줄바꿈뒤문장",
  "인용문입니다",
  "글머리항목",
  "하위항목",
  "셋째항목",
  "번호항목",
  "할일항목",
  "코드블록내용",
  "표머리글",
  "표셀내용",
  "마지막문단",
];

describe("the editor opens what the server sends", () => {
  it("loads every kind of content without refusing the document", () => {
    const editor = new Editor({ extensions: documentExtensions() });
    try {
      // A shape the schema cannot hold throws here rather than degrading, so
      // the document does not open at all.
      expect(() => editor.commands.setContent(everyNode)).not.toThrow();
    } finally {
      editor.destroy();
    }
  });

  it("keeps every phrase the export formats carry", () => {
    const editor = new Editor({ extensions: documentExtensions() });
    try {
      editor.commands.setContent(everyNode);
      const loaded = JSON.stringify(editor.getJSON());
      for (const phrase of carriedPhrases) {
        expect(loaded, `${phrase} 가 편집기에서 사라졌습니다`).toContain(
          phrase,
        );
      }
    } finally {
      editor.destroy();
    }
  });

  it("holds a document the schema calls valid", () => {
    // setContent does not validate: ProseMirror builds the node from JSON and
    // only complains later, on the first edit that touches the bad part. A
    // document can therefore load, save, and be unusable — which is what a
    // hardBreak inside a footnote did.
    const editor = new Editor({ extensions: documentExtensions() });
    try {
      editor.commands.setContent(everyNode);
      expect(() => editor.state.doc.check()).not.toThrow();
    } finally {
      editor.destroy();
    }
  });

  it("keeps every kind of block the fixture names", () => {
    const editor = new Editor({ extensions: documentExtensions() });
    try {
      editor.commands.setContent(everyNode);
      const loaded = JSON.stringify(editor.getJSON());
      for (const block of everyNode.content as { type: string }[]) {
        expect(loaded, `${block.type} 블록이 사라졌습니다`).toContain(
          `"${block.type}"`,
        );
      }
    } finally {
      editor.destroy();
    }
  });
});
