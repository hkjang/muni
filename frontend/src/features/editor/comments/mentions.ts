/**
 * Reading and writing @mentions in a comment box.
 *
 * A mention has always become a notification, and the only guidance was
 * "@아이디로 멘션할 수 있습니다" — which asks the writer to have memorised
 * somebody's username.
 */

export type MentionContext = {
  /** Where the "@" sits. */
  start: number;
  /** What has been typed after it, which is what the list is filtered by. */
  query: string;
};

/**
 * readMention decides whether the caret is inside a mention being typed.
 *
 * The "@" has to start a word, or every email address in a comment would open
 * a list of people. The query stops at whitespace, because a mention is one
 * name.
 */
export function readMention(value: string, caret: number): MentionContext | null {
  const position = Math.max(0, Math.min(caret, value.length));
  const before = value.slice(0, position);
  const at = before.lastIndexOf("@");
  if (at < 0) return null;

  const preceding = before[at - 1];
  if (preceding !== undefined && !/\s/u.test(preceding)) return null;

  const query = before.slice(at + 1);
  // A name has no spaces and nobody types thirty characters of one.
  if (/[\s@]/u.test(query) || query.length > 30) return null;
  return { start: at, query };
}

/** applyMention writes the chosen name in and reports where the caret goes. */
export function applyMention(
  value: string,
  mention: MentionContext,
  username: string,
): { value: string; caret: number } {
  const after = value.slice(mention.start + 1 + mention.query.length);
  // A trailing space so the next word is not stuck to the name, unless one is
  // already there.
  const spacer = after.startsWith(" ") ? "" : " ";
  const inserted = `@${username}${spacer}`;
  return {
    value: value.slice(0, mention.start) + inserted + after,
    caret: mention.start + inserted.length,
  };
}
