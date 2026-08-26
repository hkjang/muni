import { Extension } from "@tiptap/core";

/**
 * The shades offered for a table cell.
 *
 * A short list of muted tones rather than a colour picker: a table in a report
 * is read for its numbers, and the shading is there to group rows, not to
 * decorate them. Every one of these stays legible under black text in print.
 */
export const cellShades = [
  { value: "", label: "없음" },
  { value: "#f3f4fa", label: "회색" },
  { value: "#e8f0fe", label: "파랑" },
  { value: "#e6f4ea", label: "초록" },
  { value: "#fef7e0", label: "노랑" },
  { value: "#fce8e6", label: "빨강" },
] as const;

/** normalizeShade keeps anything that is not a plain hex colour out. */
export function normalizeShade(value: unknown): string {
  if (typeof value !== "string") return "";
  const trimmed = value.trim();
  return /^#[0-9a-f]{6}$/i.test(trimmed) ? trimmed.toLowerCase() : "";
}

/**
 * CellBackground shades table cells.
 *
 * A header row is the only part of a table muni could set apart, so grouping
 * rows — a totals line, a section — had to be done with words.
 */
export const CellBackground = Extension.create({
  name: "muniCellBackground",

  addGlobalAttributes() {
    return [
      {
        types: ["tableCell", "tableHeader"],
        attributes: {
          backgroundColor: {
            default: null,
            parseHTML: (element) =>
              normalizeShade(
                element.getAttribute("data-background") ||
                  rgbToHex(element.style.backgroundColor),
              ) || null,
            renderHTML: (attributes) => {
              const shade = normalizeShade(attributes.backgroundColor);
              if (!shade) return {};
              return {
                "data-background": shade,
                style: `background-color: ${shade}`,
              };
            },
          },
        },
      },
    ];
  },
});

/** rgbToHex reads back what the browser reports for a style it parsed. */
export function rgbToHex(value: string): string {
  const match = /^rgba?\((\d+),\s*(\d+),\s*(\d+)/i.exec(value.trim());
  if (!match) return value;
  const parts = match.slice(1, 4).map((part) => Number(part));
  if (parts.some((part) => Number.isNaN(part) || part < 0 || part > 255)) return "";
  return (
    "#" + parts.map((part) => part.toString(16).padStart(2, "0")).join("")
  );
}
