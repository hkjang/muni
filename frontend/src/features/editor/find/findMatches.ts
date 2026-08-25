/**
 * Finding text in the document.
 *
 * The browser's own find cannot see into a scrolled editor properly and cannot
 * replace, which is why Google Docs ships its own. The matching itself is a
 * plain string search over the document's text so it can be tested on its own;
 * the caller maps the offsets back to editor positions.
 */

export type MatchRange = { start: number; end: number };

export type FindOptions = {
  caseSensitive?: boolean;
  wholeWord?: boolean;
};

/** The most matches worth decorating; past this the page slows to a crawl. */
const maxMatches = 2000;

export function findMatches(
  haystack: string,
  needle: string,
  options: FindOptions = {},
): MatchRange[] {
  const query = needle;
  if (!query) return [];

  const text = options.caseSensitive ? haystack : haystack.toLowerCase();
  const target = options.caseSensitive ? query : query.toLowerCase();

  const out: MatchRange[] = [];
  let from = 0;
  for (;;) {
    const index = text.indexOf(target, from);
    if (index < 0) break;
    const end = index + target.length;
    if (!options.wholeWord || isWholeWord(haystack, index, end)) {
      out.push({ start: index, end });
      if (out.length >= maxMatches) break;
    }
    // Overlapping matches are not useful, but a query of one character must
    // still advance.
    from = index + Math.max(1, target.length);
  }
  return out;
}

/**
 * isWholeWord checks the characters on either side.
 *
 * Korean has no spaces inside a word the way English does, so a Hangul
 * character next to the match counts as part of the same word — otherwise
 * "회의" would match inside "회의록" even with whole-word asked for.
 */
function isWholeWord(text: string, start: number, end: number): boolean {
  return !isWordCharacter(text[start - 1]) && !isWordCharacter(text[end]);
}

function isWordCharacter(character: string | undefined): boolean {
  if (!character) return false;
  return /[\p{L}\p{N}_]/u.test(character);
}

/**
 * nextMatchIndex is the match to land on when the caret is where it is.
 *
 * Searching moves forward from the caret and wraps, which is what every editor
 * does and what people expect when they press Enter in the find box.
 */
export function nextMatchIndex(
  matches: MatchRange[],
  caret: number,
  direction: 1 | -1 = 1,
): number {
  if (matches.length === 0) return -1;
  if (direction === 1) {
    const found = matches.findIndex((match) => match.start >= caret);
    return found === -1 ? 0 : found;
  }
  for (let index = matches.length - 1; index >= 0; index -= 1) {
    const match = matches[index];
    if (match && match.end <= caret) return index;
  }
  return matches.length - 1;
}

/** step moves to the next or previous match, wrapping at both ends. */
export function step(current: number, total: number, direction: 1 | -1): number {
  if (total === 0) return -1;
  return (current + direction + total) % total;
}
