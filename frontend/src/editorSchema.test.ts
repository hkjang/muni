import { describe, expect, it } from "vitest";
import { Editor } from "@tiptap/core";
import StarterKit from "@tiptap/starter-kit";
import Highlight from "@tiptap/extension-highlight";
import { TableKit } from "@tiptap/extension-table";
import TaskList from "@tiptap/extension-task-list";
import TaskItem from "@tiptap/extension-task-item";
import TextAlign from "@tiptap/extension-text-align";
import { TextStyleKit } from "@tiptap/extension-text-style";
import Superscript from "@tiptap/extension-superscript";
import Subscript from "@tiptap/extension-subscript";

/**
 * Every mark the server can put into a document must exist in the editor's
 * schema.
 *
 * ProseMirror does not ignore a mark it does not know — it discards the whole
 * paragraph the mark appears in, content and all. muni's .docx import produced
 * superscript and subscript marks, the editor had neither, and so a Word
 * document containing m², H₂O or a footnote number lost the entire paragraph
 * carrying it the moment somebody opened it. The next autosave wrote the loss
 * back, which made it permanent.
 *
 * This list is the server's vocabulary. It is spelled out here rather than
 * imported because the two halves are written in different languages and the
 * point is to notice when they drift apart. If the Go importer learns a new
 * mark, this test fails until the editor learns it too.
 */
const marksTheServerCanProduce = [
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

/** The same extension set the editor and the shared view are built with. */
function editorLike() {
  return new Editor({
    extensions: [
      StarterKit.configure({ undoRedo: false }),
      Highlight.configure({ multicolor: true }),
      TableKit.configure({ table: { resizable: true } }),
      TaskList,
      TaskItem.configure({ nested: true }),
      TextAlign.configure({ types: ["heading", "paragraph"] }),
      TextStyleKit,
      Superscript,
      Subscript,
    ],
  });
}

describe("the editor understands what the server sends", () => {
  it("has every mark the importer can produce", () => {
    const known = Object.keys(editorLike().schema.marks);
    const missing = marksTheServerCanProduce.filter((mark) => !known.includes(mark));
    expect(missing, "marks the server produces and the editor would discard").toEqual([]);
  });

  it("keeps the paragraph when one of them arrives", () => {
    // The failure was never subtle once you looked: the text went too, not
    // just its formatting.
    for (const mark of marksTheServerCanProduce) {
      const instance = editorLike();
      instance.commands.setContent({
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
      const json = JSON.stringify(instance.getJSON());
      expect(json, `paragraph text after loading a ${mark} mark`).toContain("면적 3m");
    }
  });
});
