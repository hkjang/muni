import { Extension } from "@tiptap/core";

/**
 * Where a cell's text sits between the top and the bottom of its row.
 *
 * Word writes this per cell and defaults to the top. muni had no place to keep
 * it, and wrote every exported cell as centred — so a table with a tall 비고
 * column, whose text the author had left at the top, came back centred. muni
 * was not losing the alignment so much as replacing it.
 */
export const cellAlignments = ["top", "middle", "bottom"] as const;

export type CellAlignment = (typeof cellAlignments)[number];

/** normalizeAlignment keeps anything muni cannot draw out. */
export function normalizeAlignment(value: unknown): CellAlignment | null {
  if (typeof value !== "string") return null;
  const trimmed = value.trim().toLowerCase();
  return (cellAlignments as readonly string[]).includes(trimmed)
    ? (trimmed as CellAlignment)
    : null;
}

/**
 * CellVerticalAlign holds what Word said, so muni can say it back.
 *
 * There is no toolbar control yet. The point is that a document muni did not
 * write keeps its own shape: an attribute the schema does not declare is
 * dropped on load without a word, and the next save makes that final.
 */
export const CellVerticalAlign = Extension.create({
  name: "muniCellVerticalAlign",

  addGlobalAttributes() {
    return [
      {
        types: ["tableCell", "tableHeader"],
        attributes: {
          verticalAlign: {
            default: null,
            parseHTML: (element) =>
              normalizeAlignment(
                element.getAttribute("data-vertical-align") ||
                  element.style.verticalAlign,
              ),
            renderHTML: (attributes) => {
              const alignment = normalizeAlignment(attributes.verticalAlign);
              if (!alignment) return {};
              return {
                "data-vertical-align": alignment,
                style: `vertical-align: ${alignment}`,
              };
            },
          },
        },
      },
    ];
  },
});
