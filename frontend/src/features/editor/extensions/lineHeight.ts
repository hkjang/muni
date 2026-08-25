import { Extension } from "@tiptap/core";

export type LineHeightOptions = {
  types: string[];
  values: string[];
  defaultValue: string;
};

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    muniLineHeight: {
      setLineHeight: (value: string) => ReturnType;
      unsetLineHeight: () => ReturnType;
    };
  }
}

/**
 * LineHeight puts line spacing on paragraphs and headings.
 *
 * It is one of the few things a Korean document routinely needs that a plain
 * rich-text editor leaves out: 160% is what most report templates ask for, and
 * without it the only way to get there is to leave blank paragraphs behind.
 */
export const LineHeight = Extension.create<LineHeightOptions>({
  name: "muniLineHeight",

  addOptions() {
    return {
      types: ["paragraph", "heading", "listItem", "taskItem"],
      values: ["1", "1.15", "1.5", "1.75", "2", "2.5"],
      defaultValue: "",
    };
  },

  addGlobalAttributes() {
    return [
      {
        types: this.options.types,
        attributes: {
          lineHeight: {
            default: this.options.defaultValue,
            parseHTML: (element) => element.style.lineHeight || this.options.defaultValue,
            renderHTML: (attributes) => {
              if (!attributes.lineHeight) return {};
              return { style: `line-height: ${attributes.lineHeight}` };
            },
          },
        },
      },
    ];
  },

  addCommands() {
    return {
      setLineHeight:
        (value) =>
        ({ commands }) =>
          this.options.types.every((type) =>
            commands.updateAttributes(type, { lineHeight: value }),
          ),
      unsetLineHeight:
        () =>
        ({ commands }) =>
          this.options.types.every((type) => commands.resetAttributes(type, "lineHeight")),
    };
  },
});
