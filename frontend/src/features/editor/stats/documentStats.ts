/**
 * How long the document is.
 *
 * Google Docs puts this a keystroke away because it is what people are asked
 * for — a report with a length limit, a summary that has to fit on a page.
 */

export type DocumentStats = {
  words: number;
  characters: number;
  charactersNoSpaces: number;
  paragraphs: number;
  /** Minutes to read at 500 characters a minute, never less than one. */
  readingMinutes: number;
};

/**
 * countWords counts whitespace-separated runs.
 *
 * Korean marks word boundaries with spaces, so this is the same count a Korean
 * reader would arrive at by hand. A run of CJK with no spaces at all counts as
 * one word, which is what every editor does and what the character count is
 * there to cover.
 */
export function countWords(text: string): number {
  const trimmed = text.trim();
  if (!trimmed) return 0;
  return trimmed.split(/\s+/u).length;
}

/** Korean reads at roughly this speed; the figure is a hint, not a promise. */
const charactersPerMinute = 500;

export function documentStats(text: string): DocumentStats {
  const characters = [...text].length;
  const charactersNoSpaces = [...text.replace(/\s/gu, "")].length;
  const paragraphs = text
    .split(/\n+/u)
    .filter((line) => line.trim().length > 0).length;
  return {
    words: countWords(text),
    characters,
    charactersNoSpaces,
    paragraphs,
    readingMinutes: charactersNoSpaces === 0
      ? 0
      : Math.max(1, Math.round(charactersNoSpaces / charactersPerMinute)),
  };
}
