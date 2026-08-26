import { describe, expect, it } from "vitest";

/**
 * Every button a person can press has to say what it does.
 *
 * An icon-only button with no accessible name is announced as "버튼" and
 * nothing else, which makes the editor toolbar — almost entirely icon buttons —
 * unusable with a screen reader. Korean public-sector and enterprise
 * procurement asks for this specifically (컨트롤의 명칭 제공).
 *
 * MUI's Tooltip supplies the name when the button is its direct child. It does
 * not when a <span> sits between them, which is the shape used so a tooltip
 * still appears over a disabled button: the label lands on the span, and the
 * button underneath keeps none. That was verified against the rendered
 * accessibility tree, not assumed — it is the reason this check exists rather
 * than a rule about tooltips.
 */

/** Returns the attribute text of every <IconButton ...> in the source. */
function iconButtonAttributes(source: string): { attrs: string; at: number }[] {
  const found: { attrs: string; at: number }[] = [];
  const tag = "<IconButton";
  let index = source.indexOf(tag);
  while (index !== -1) {
    // Scan to the '>' that closes the tag, ignoring the ones inside braces —
    // an arrow function in onClick contains '>' and stopping there would read
    // only half the attributes.
    let depth = 0;
    let cursor = index + tag.length;
    while (cursor < source.length) {
      const character = source[cursor];
      if (character === "{") depth += 1;
      else if (character === "}") depth -= 1;
      else if (character === ">" && depth === 0) break;
      cursor += 1;
    }
    found.push({ attrs: source.slice(index + tag.length, cursor), at: index });
    index = source.indexOf(tag, cursor);
  }
  return found;
}

// Vite reads the sources at build time, so the check needs no filesystem
// access and runs the same way in CI as it does here.
const sources = import.meta.glob("./**/*.tsx", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

describe("icon buttons have accessible names", () => {
  it("every IconButton is labelled, by aria-label or by a tooltip on itself", () => {
    const unlabelled: string[] = [];
    for (const [file, source] of Object.entries(sources)) {
      for (const { attrs, at } of iconButtonAttributes(source)) {
        if (attrs.includes("aria-label")) continue;
        // A Tooltip immediately around the button labels it. A Tooltip with a
        // <span> in between does not.
        const preceding = source.slice(Math.max(0, at - 200), at);
        if (/<Tooltip[^>]*>\s*$/.test(preceding)) continue;
        const line = source.slice(0, at).split("\n").length;
        unlabelled.push(`${file}:${line}`);
      }
    }
    expect(unlabelled).toEqual([]);
  });
});
