/**
 * The document outline, built from the headings in the document.
 *
 * Google Docs keeps this beside the page and it is the fastest way to move
 * around a long document — far faster than scrolling. Everything here works on
 * plain data so the shape of the list can be tested without an editor.
 */

/** A heading as it was found in the document. */
export type RawHeading = {
  level: number;
  text: string;
  /** Where the heading starts, in editor positions. */
  pos: number;
};

export type OutlineItem = RawHeading & {
  /** How far to indent, counting from the shallowest heading present. */
  depth: number;
};

const maxLabel = 90;

/**
 * buildOutline turns the headings into the list the panel draws.
 *
 * Indentation counts from the shallowest heading actually used rather than
 * from `h1`, so a document written entirely in `h2` and `h3` is not drawn
 * pushed off to the right. A deeper heading never indents more than one step
 * past the one above it, which keeps a document that skips from `h1` to `h4`
 * readable.
 */
export function buildOutline(headings: RawHeading[]): OutlineItem[] {
  const named = headings
    .map((heading) => ({ ...heading, text: heading.text.trim() }))
    .filter((heading) => heading.text.length > 0);
  if (named.length === 0) return [];

  const shallowest = Math.min(...named.map((heading) => heading.level));
  const out: OutlineItem[] = [];
  let previousDepth = -1;
  let previousLevel = shallowest;

  for (const heading of named) {
    let depth: number;
    if (heading.level <= shallowest) depth = 0;
    else if (heading.level > previousLevel) depth = previousDepth + 1;
    else if (heading.level === previousLevel) depth = previousDepth;
    else depth = Math.max(0, previousDepth - (previousLevel - heading.level));
    out.push({
      ...heading,
      text: heading.text.length > maxLabel ? heading.text.slice(0, maxLabel) + "…" : heading.text,
      depth,
    });
    previousDepth = depth;
    previousLevel = heading.level;
  }
  return out;
}

/**
 * currentOutlineIndex is the section the caret is in: the last heading at or
 * before it. Returns -1 when the caret sits above every heading.
 */
export function currentOutlineIndex(items: OutlineItem[], caret: number): number {
  let found = -1;
  for (let index = 0; index < items.length; index += 1) {
    const item = items[index];
    if (item && item.pos <= caret) found = index;
    else break;
  }
  return found;
}
