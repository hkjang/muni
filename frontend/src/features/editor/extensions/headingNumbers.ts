import { Extension } from "@tiptap/core";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";
import { buildOutline, type RawHeading } from "../outline/outline";
import { headingNumbers, validScheme, type NumberingScheme } from "../outline/numbering";

export const headingNumberKey = new PluginKey("muniHeadingNumbers");

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    muniHeadingNumbers: {
      setHeadingNumbering: (scheme: NumberingScheme) => ReturnType;
    };
  }
}

export type HeadingNumberOptions = { scheme: NumberingScheme };

/**
 * HeadingNumbers draws the section number in front of each heading.
 *
 * The numbers are decorations, not text: written into the document they would
 * be wrong the moment a section moved, and a collaborator's renumbering would
 * arrive as an edit to every heading below the change. Drawing them means the
 * document holds only what an author actually wrote, and the server puts the
 * same numbers into the export.
 */
export const HeadingNumbers = Extension.create<HeadingNumberOptions>({
  name: "muniHeadingNumbers",

  addOptions() {
    return { scheme: "none" };
  },

  addCommands() {
    return {
      setHeadingNumbering:
        (scheme) =>
        ({ dispatch, tr }) => {
          if (dispatch) dispatch(tr.setMeta(headingNumberKey, validScheme(scheme)));
          return true;
        },
    };
  },

  addProseMirrorPlugins() {
    const initial = validScheme(this.options.scheme);
    return [
      new Plugin({
        key: headingNumberKey,
        state: {
          init: () => initial,
          apply(transaction, value) {
            const meta = transaction.getMeta(headingNumberKey) as
              | NumberingScheme
              | undefined;
            return meta ?? value;
          },
        },
        props: {
          decorations(state) {
            const scheme = headingNumberKey.getState(state) as NumberingScheme;
            if (!scheme || scheme === "none") return DecorationSet.empty;

            const headings: RawHeading[] = [];
            state.doc.descendants((node, pos) => {
              if (node.type.name !== "heading") return true;
              headings.push({
                level: Number(node.attrs.level ?? 1),
                text: node.textContent,
                pos,
              });
              return false;
            });
            // buildOutline drops empty headings, which must not take a number
            // — the same rule the server follows.
            const outline = buildOutline(headings);
            if (outline.length === 0) return DecorationSet.empty;

            const labels = headingNumbers(
              outline.map((item) => item.depth),
              scheme,
            );
            return DecorationSet.create(
              state.doc,
              outline.map((item, index) =>
                Decoration.widget(
                  item.pos + 1,
                  () => {
                    const span = document.createElement("span");
                    span.className = "muni-heading-number";
                    span.textContent = (labels[index] ?? "") + " ";
                    // The number is drawn, not written, so it must never be
                    // selectable or land in a copy of the text.
                    span.contentEditable = "false";
                    return span;
                  },
                  { side: -1, marks: [] },
                ),
              ),
            );
          },
        },
      }),
    ];
  },
});
