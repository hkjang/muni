import { Extension } from "@tiptap/core";
import type { EditorState, Transaction } from "@tiptap/pm/state";

export type ParagraphIndentOptions = {
  types: string[];
  /** How far one step of indentation moves the text. */
  step: number;
  maxSteps: number;
};

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    muniIndent: {
      indentParagraph: () => ReturnType;
      outdentParagraph: () => ReturnType;
      toggleFirstLineIndent: () => ReturnType;
    };
  }
}

/**
 * Paragraph indentation.
 *
 * A Korean document asks for two things a plain rich-text editor does not
 * offer: a whole paragraph moved in from the margin, and a first line indented
 * by one character. Without them the only ways to get there are leading spaces
 * — which vanish on export — or a bullet list with the bullet hidden.
 *
 * Indentation is stored as a count of steps rather than a length, so the
 * document does not carry a pixel measurement that means something different
 * at another zoom or on paper.
 */
export const ParagraphIndent = Extension.create<ParagraphIndentOptions>({
  name: "muniIndent",

  addOptions() {
    return {
      types: ["paragraph", "heading", "blockquote"],
      step: 2,
      maxSteps: 8,
    };
  },

  addGlobalAttributes() {
    return [
      {
        types: this.options.types,
        attributes: {
          indent: {
            default: 0,
            parseHTML: (element) => {
              const value = Number.parseFloat(element.style.marginInlineStart || "0");
              return Math.round(value / this.options.step) || 0;
            },
            renderHTML: (attributes) => {
              const steps = Number(attributes.indent ?? 0);
              if (!steps) return {};
              return {
                style: `margin-inline-start: ${steps * this.options.step}em`,
              };
            },
          },
          firstLine: {
            default: false,
            parseHTML: (element) => Boolean(element.style.textIndent),
            renderHTML: (attributes) =>
              attributes.firstLine ? { style: "text-indent: 1em" } : {},
          },
        },
      },
    ];
  },

  addCommands() {
    const types = this.options.types;
    const maxSteps = this.options.maxSteps;

    /** shift moves every block in the selection one step in or out. */
    const shift =
      (direction: 1 | -1) =>
      () =>
      ({ state, tr, dispatch }: { state: EditorState; tr: Transaction; dispatch?: (tr: Transaction) => void }) => {
        const { from, to } = state.selection;
        let changed = false;
        state.doc.nodesBetween(from, to, (node, pos) => {
          if (!types.includes(node.type.name)) return true;
          const current = Number(node.attrs.indent ?? 0);
          const next = Math.min(maxSteps, Math.max(0, current + direction));
          if (next !== current) {
            tr.setNodeMarkup(pos, undefined, { ...node.attrs, indent: next });
            changed = true;
          }
          return false;
        });
        if (changed && dispatch) dispatch(tr);
        return changed;
      };

    return {
      indentParagraph: shift(1),
      outdentParagraph: shift(-1),
      toggleFirstLineIndent:
        () =>
        ({ state, tr, dispatch }) => {
          const { from, to } = state.selection;
          const targets: { pos: number; attrs: Record<string, unknown> }[] = [];
          let anyOff = false;
          state.doc.nodesBetween(from, to, (node, pos) => {
            if (!types.includes(node.type.name)) return true;
            targets.push({ pos, attrs: node.attrs });
            if (!node.attrs.firstLine) anyOff = true;
            return false;
          });
          if (targets.length === 0) return false;
          for (const target of targets)
            tr.setNodeMarkup(target.pos, undefined, {
              ...target.attrs,
              firstLine: anyOff,
            });
          if (dispatch) dispatch(tr);
          return true;
        },
    };
  },

  addKeyboardShortcuts() {
    return {
      // Inside a list Tab already means "one level deeper"; outside one it
      // means what it means in every word processor.
      Tab: ({ editor }) => {
        if (editor.isActive("listItem") || editor.isActive("taskItem")) return false;
        if (editor.isActive("codeBlock") || editor.isActive("table")) return false;
        return editor.commands.indentParagraph();
      },
      "Shift-Tab": ({ editor }) => {
        if (editor.isActive("listItem") || editor.isActive("taskItem")) return false;
        if (editor.isActive("codeBlock") || editor.isActive("table")) return false;
        return editor.commands.outdentParagraph();
      },
    };
  },
});
