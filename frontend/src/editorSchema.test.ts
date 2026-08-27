import { describe, expect, it } from "vitest";
import { Editor } from "@tiptap/core";
import { documentExtensions } from "./features/editor/documentExtensions";

/**
 * Everything the server can put into a document must exist in the schema of
 * every screen that renders one.
 *
 * ProseMirror does not ignore what it does not recognise. An unknown mark
 * takes the whole paragraph it sits in — text and all — and an unknown node
 * takes itself and its contents. muni's .docx import produced superscript and
 * subscript marks that the editor had never heard of, so a Word document
 * containing m², H₂O or a footnote number lost the paragraph carrying it the
 * moment somebody opened it, and the next autosave made that permanent.
 *
 * The vocabularies below are written out rather than imported from the Go
 * side. The two halves are in different languages, and noticing when they
 * drift apart is the entire point: teach the importer a new mark and this
 * fails until the editor learns it too.
 *
 * The schema itself comes from documentExtensions, the one list both screens
 * build from — checking a copy of that list would prove only that the copy
 * agrees with itself, which is how the share view came to be missing the
 * contents-list node in the first place.
 */
const marksTheServerProduces = [
  "bold",
  "code",
  "highlight",
  "italic",
  "link",
  "strike",
  "subscript",
  "superscript",
  "textStyle",
  "underline",
];

const nodesTheServerProduces = [
  "blockquote",
  "bulletList",
  "codeBlock",
  "footnote",
  "hardBreak",
  "heading",
  "horizontalRule",
  "image",
  "listItem",
  "orderedList",
  "pageBreak",
  "paragraph",
  "table",
  "tableCell",
  "tableHeader",
  "tableRow",
  "tableOfContents",
  "taskItem",
  "taskList",
  "text",
];

describe("every screen understands what the server sends", () => {
  const extensions = documentExtensions();

  // A Tiptap editor keeps a ProseMirror view with a timer on it. Left alive,
  // it fires after the test environment is gone and takes the run down with
  // "document is not defined" — every assertion passing and the process
  // exiting 1.
  function withEditor(use: (editor: Editor) => void) {
    const editor = new Editor({ extensions });
    try {
      use(editor);
    } finally {
      editor.destroy();
    }
  }

  it("has every mark and node the server can produce", () => {
    withEditor((editor) => {
      const missingMarks = marksTheServerProduces.filter((m) => !(m in editor.schema.marks));
      const missingNodes = nodesTheServerProduces.filter((n) => !(n in editor.schema.nodes));
      expect(missingMarks, "marks the server produces and the screen would discard").toEqual([]);
      expect(missingNodes, "nodes the server produces and the screen would discard").toEqual([]);
    });
  });

  it("keeps the paragraph when each mark arrives", () => {
    // The failure was never subtle once you looked: the text went too, not
    // just its formatting.
    for (const mark of marksTheServerProduces) {
      withEditor((editor) => {
        editor.commands.setContent({
          type: "doc",
          content: [
            {
              type: "paragraph",
              content: [
                { type: "text", text: "면적 3m" },
                { type: "text", marks: [{ type: mark }], text: "2" },
              ],
            },
          ],
        });
        expect(
          JSON.stringify(editor.getJSON()),
          `paragraph text after loading a ${mark} mark`,
        ).toContain("면적 3m");
      });
    }
  });
});
