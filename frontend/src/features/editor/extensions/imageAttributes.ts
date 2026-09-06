import Image from "@tiptap/extension-image";

/**
 * The width the presets are a percentage of: the page is 860px wide with 72px
 * of margin on each side, which is what an image at 100% fills.
 */
export const contentWidth = 716;

export const widthPresets = [25, 50, 75, 100] as const;

/** pixelsFor turns a preset into the pixel width that is actually stored. */
export function pixelsFor(percent: number): number {
  return Math.round((contentWidth * percent) / 100);
}

/** percentFor reports which preset a stored width corresponds to, if any. */
export function percentFor(width: number | null | undefined): number | null {
  if (!width) return 100;
  const percent = Math.round((width / contentWidth) * 100);
  const nearest = widthPresets.find((preset) => Math.abs(preset - percent) <= 3);
  return nearest ?? null;
}

/**
 * heightFor keeps a picture's shape while its width changes.
 *
 * A document can draw a picture in a box that is not the shape of its bytes —
 * a 한글 header stamp squashed to fit a line — and that shape is stored as a
 * height beside the width. Moving the width to a preset without moving the
 * height with it would stretch the picture, so the two travel together.
 */
export function heightFor(
  width: number | null | undefined,
  height: number | null | undefined,
  nextWidth: number,
): number | null {
  if (!width || !height || width <= 0 || height <= 0) return null;
  return Math.max(1, Math.round((height * nextWidth) / width));
}

/**
 * SizedImage adds a width, a drawn height and an alignment to an image.
 *
 * Images went in at whatever size they happened to be and always sat on the
 * left. The width is stored in pixels rather than as a percentage because that
 * is what both exporters already understand: the DOCX writer scales the
 * picture to it and the HTML writer puts it on the tag.
 */
export const SizedImage = Image.extend({
  addAttributes() {
    return {
      ...this.parent?.(),
      width: {
        default: null,
        parseHTML: (element) => {
          const attribute = element.getAttribute("width");
          if (attribute) return Number.parseInt(attribute, 10) || null;
          const style = Number.parseInt(element.style.width || "", 10);
          return Number.isNaN(style) ? null : style;
        },
        renderHTML: (attributes) => {
          const width = Number(attributes.width ?? 0);
          if (!width) return {};
          return { width: String(width), style: `width:${width}px` };
        },
      },
      height: {
        default: null,
        parseHTML: (element) => {
          const attribute = element.getAttribute("height");
          if (attribute) return Number.parseInt(attribute, 10) || null;
          const style = Number.parseInt(element.style.height || "", 10);
          return Number.isNaN(style) ? null : style;
        },
        renderHTML: (attributes) => {
          const width = Number(attributes.width ?? 0);
          const height = Number(attributes.height ?? 0);
          if (!width || !height) return {};
          // The stylesheet gives every picture height:auto so a wide one
          // shrinks to the page instead of spilling off it, which also
          // flattens a squashed picture back to the shape of its bytes. The
          // ratio survives that shrinking; a fixed height would not.
          return { height: String(height), style: `aspect-ratio:${width}/${height}` };
        },
      },
      textAlign: {
        default: null,
        parseHTML: (element) => element.style.textAlign || null,
        renderHTML: (attributes) => {
          if (!attributes.textAlign) return {};
          // An inline-block image is moved by aligning it, and margin:auto is
          // what actually centres one.
          if (attributes.textAlign === "center")
            return { style: "display:block;margin-inline:auto" };
          if (attributes.textAlign === "right")
            return { style: "display:block;margin-inline-start:auto" };
          return {};
        },
      },
    };
  },
});
