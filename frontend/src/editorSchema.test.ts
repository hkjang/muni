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
      const missingMarks = marksTheServerProduces.filter(
        (m) => !(m in editor.schema.marks),
      );
      const missingNodes = nodesTheServerProduces.filter(
        (n) => !(n in editor.schema.nodes),
      );
      expect(
        missingMarks,
        "marks the server produces and the screen would discard",
      ).toEqual([]);
      expect(
        missingNodes,
        "nodes the server produces and the screen would discard",
      ).toEqual([]);
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

/**
 * The attributes the server puts on nodes, and the node each one belongs to.
 *
 * ProseMirror treats an unknown attribute more gently than an unknown mark —
 * it drops the attribute and keeps the content — so this loses formatting
 * rather than text. Quietly, though: a table cell arrives without its shading,
 * a paragraph without its line spacing, and nothing anywhere says so.
 */
const attributesTheServerProduces: {
  node: string;
  attrs: Record<string, unknown>;
}[] = [
  {
    node: "paragraph",
    attrs: {
      textAlign: "justify",
      lineHeight: "1.6",
      firstLine: true,
      indent: 2,
    },
  },
  { node: "heading", attrs: { level: 2 } },
  { node: "orderedList", attrs: { start: 3 } },
  { node: "codeBlock", attrs: { language: "go" } },
];

describe("node attributes survive a load", () => {
  it.each(attributesTheServerProduces)(
    "$node keeps what the server set",
    ({ node, attrs }) => {
      const editor = new Editor({ extensions: documentExtensions() });
      try {
        const child =
          node === "orderedList"
            ? {
                type: node,
                attrs,
                content: [
                  {
                    type: "listItem",
                    content: [
                      {
                        type: "paragraph",
                        content: [{ type: "text", text: "항목" }],
                      },
                    ],
                  },
                ],
              }
            : { type: node, attrs, content: [{ type: "text", text: "내용" }] };
        editor.commands.setContent({ type: "doc", content: [child] });

        const loaded = (editor.getJSON().content ?? [])[0] as
          { attrs?: Record<string, unknown> } | undefined;
        for (const [key, value] of Object.entries(attrs)) {
          expect(loaded?.attrs?.[key], `${node}.${key}`).toBe(value);
        }
      } finally {
        editor.destroy();
      }
    },
  );

  // The editor draws an image as a block of its own, so the server lifts a
  // picture out of the paragraph that held it before sending the document
  // (richdoc.LiftImages). A paragraph with an image still inside is not a
  // document the editor loses formatting from — it is one setContent throws on.
  it("an image keeps its source, size and description", () => {
    const editor = new Editor({ extensions: documentExtensions() });
    try {
      editor.commands.setContent({
        type: "doc",
        content: [
          {
            type: "image",
            attrs: {
              src: "/api/v1/attachments/x",
              alt: "표 이미지",
              width: 320,
            },
          },
        ],
      });
      const json = JSON.stringify(editor.getJSON());
      for (const expected of ["/api/v1/attachments/x", "표 이미지", "320"]) {
        expect(json, "image attributes").toContain(expected);
      }
    } finally {
      editor.destroy();
    }
  });

  it("refuses a picture left inside a paragraph", () => {
    // If this ever stops throwing, the editor has learned inline images and
    // richdoc.LiftImages can go. Until then it is the reason that code exists.
    const editor = new Editor({ extensions: documentExtensions() });
    try {
      expect(() =>
        editor.commands.setContent({
          type: "doc",
          content: [
            {
              type: "paragraph",
              content: [
                { type: "text", text: "사진 앞" },
                { type: "image", attrs: { src: "/api/v1/attachments/x" } },
              ],
            },
          ],
        }),
      ).toThrow();
    } finally {
      editor.destroy();
    }
  });

  it("a table cell keeps its span and shading", () => {
    // 병합된 셀과 음영은 한국 공문서 표의 기본이고, 조용히 사라지면
    // 표가 다른 표가 됩니다.
    const editor = new Editor({ extensions: documentExtensions() });
    try {
      editor.commands.setContent({
        type: "doc",
        content: [
          {
            type: "table",
            content: [
              {
                type: "tableRow",
                content: [
                  {
                    type: "tableCell",
                    attrs: {
                      colspan: 2,
                      rowspan: 1,
                      backgroundColor: "#d9e2f3",
                    },
                    content: [
                      {
                        type: "paragraph",
                        content: [{ type: "text", text: "병합" }],
                      },
                    ],
                  },
                ],
              },
            ],
          },
        ],
      });
      const json = JSON.stringify(editor.getJSON());
      expect(json, "colspan").toContain('"colspan":2');
      expect(json, "cell shading").toContain("#d9e2f3");
    } finally {
      editor.destroy();
    }
  });

  it("a task item remembers whether it was ticked", () => {
    const editor = new Editor({ extensions: documentExtensions() });
    try {
      editor.commands.setContent({
        type: "doc",
        content: [
          {
            type: "taskList",
            content: [
              {
                type: "taskItem",
                attrs: { checked: true },
                content: [
                  {
                    type: "paragraph",
                    content: [{ type: "text", text: "끝난 일" }],
                  },
                ],
              },
            ],
          },
        ],
      });
      expect(JSON.stringify(editor.getJSON()), "checked").toContain(
        '"checked":true',
      );
    } finally {
      editor.destroy();
    }
  });
});
