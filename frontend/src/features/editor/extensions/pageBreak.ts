import { Node, mergeAttributes } from "@tiptap/core";

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    muniPageBreak: {
      setPageBreak: () => ReturnType;
    };
  }
}

/**
 * A page break.
 *
 * The editor has no pages, so the break shows as a labelled line: without a
 * mark on screen there is no way to tell a document that will print on three
 * pages from one that will print on one. It is a real node rather than an
 * empty paragraph so it survives the round trip through DOCX, where Word
 * writes it as `<w:br w:type="page"/>`, and through the PDF export, which
 * prints the same HTML the browser shows.
 */
export const PageBreak = Node.create({
  name: "pageBreak",
  group: "block",
  atom: true,
  selectable: true,
  draggable: false,

  parseHTML() {
    return [
      { tag: "div[data-page-break]" },
      { tag: "div.muni-page-break" },
      // What the Markdown export writes, so a re-import keeps it.
      {
        tag: "div",
        getAttrs: (element) =>
          (element as HTMLElement).style.pageBreakAfter === "always" ? {} : false,
      },
    ];
  },

  renderHTML({ HTMLAttributes }) {
    return [
      "div",
      mergeAttributes(HTMLAttributes, {
        "data-page-break": "true",
        class: "muni-page-break",
      }),
    ];
  },

  addCommands() {
    return {
      setPageBreak:
        () =>
        ({ chain }) =>
          chain()
            .insertContent({ type: this.name })
            // A break with nothing after it leaves no way to keep writing.
            .command(({ tr, state, dispatch }) => {
              const paragraph = state.schema.nodes.paragraph;
              const end = tr.selection.to;
              if (!paragraph || end < state.doc.content.size - 1) return true;
              if (dispatch) dispatch(tr.insert(end, paragraph.create()));
              return true;
            })
            .run(),
    };
  },

  addKeyboardShortcuts() {
    return {
      "Mod-Enter": () => this.editor.commands.setPageBreak(),
    };
  },
});
