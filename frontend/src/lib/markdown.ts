/**
 * A small Markdown reader.
 *
 * The model answers in Markdown whether or not it is asked to, so muni has to
 * understand it in two places: the agent panel renders it, and an AI rewrite is
 * inserted back into the document as real formatting rather than as the literal
 * characters `**중요**`.
 *
 * It covers what a language model actually produces — headings, lists, quotes,
 * fenced code, tables, emphasis, inline code and links — and deliberately not
 * the rest of CommonMark. Raw HTML is never interpreted; it stays text.
 */

export type MDMark =
  | { type: "bold" }
  | { type: "italic" }
  | { type: "code" }
  | { type: "strike" }
  | { type: "link"; href: string };

export type MDInline = { text: string; marks: MDMark[] };

export type MDListItem = { blocks: MDBlock[]; checked?: boolean };

export type MDBlock =
  | { type: "heading"; level: number; inline: MDInline[] }
  | { type: "paragraph"; inline: MDInline[] }
  | { type: "list"; ordered: boolean; start: number; items: MDListItem[] }
  | { type: "quote"; blocks: MDBlock[] }
  | { type: "code"; language: string; text: string }
  | { type: "rule" }
  | { type: "table"; header: MDInline[][]; rows: MDInline[][][] };

const bulletPattern = /^([-*+])\s+(.*)$/;
const orderedPattern = /^(\d{1,9})[.)]\s+(.*)$/;
const taskPattern = /^\[([ xX])\]\s+(.*)$/;

/** parseMarkdown turns a Markdown document into blocks. */
export function parseMarkdown(source: string): MDBlock[] {
  const lines = source.replace(/\r\n?/g, "\n").split("\n");
  return parseBlocks(lines);
}

function parseBlocks(lines: string[]): MDBlock[] {
  const blocks: MDBlock[] = [];
  let index = 0;

  while (index < lines.length) {
    const line = lines[index] ?? "";
    const trimmed = line.trim();

    if (!trimmed) {
      index += 1;
      continue;
    }

    const fence = /^(```|~~~)\s*([\w+-]*)\s*$/.exec(trimmed);
    if (fence) {
      const marker = fence[1] ?? "```";
      const body: string[] = [];
      index += 1;
      while (
        index < lines.length &&
        !(lines[index] ?? "").trim().startsWith(marker)
      ) {
        body.push(lines[index] ?? "");
        index += 1;
      }
      index += 1; // The closing fence, or the end of the input.
      blocks.push({
        type: "code",
        language: fence[2] ?? "",
        text: body.join("\n"),
      });
      continue;
    }

    const heading = /^(#{1,6})\s+(.*)$/.exec(trimmed);
    if (heading) {
      blocks.push({
        type: "heading",
        level: (heading[1] ?? "#").length,
        inline: parseInline((heading[2] ?? "").replace(/\s+#+\s*$/, "")),
      });
      index += 1;
      continue;
    }

    if (
      /^(\*\s*){3,}$/.test(trimmed) ||
      /^(-\s*){3,}$/.test(trimmed) ||
      /^(_\s*){3,}$/.test(trimmed)
    ) {
      blocks.push({ type: "rule" });
      index += 1;
      continue;
    }

    if (trimmed.startsWith(">")) {
      const body: string[] = [];
      while (
        index < lines.length &&
        (lines[index] ?? "").trim().startsWith(">")
      ) {
        body.push((lines[index] ?? "").trim().replace(/^>\s?/, ""));
        index += 1;
      }
      blocks.push({ type: "quote", blocks: parseBlocks(body) });
      continue;
    }

    if (isTableRow(line) && isTableDivider(lines[index + 1] ?? "")) {
      const header = splitTableRow(line).map((cell) => parseInline(cell));
      index += 2;
      const rows: MDInline[][][] = [];
      while (index < lines.length && isTableRow(lines[index] ?? "")) {
        rows.push(
          splitTableRow(lines[index] ?? "").map((cell) => parseInline(cell)),
        );
        index += 1;
      }
      blocks.push({ type: "table", header, rows });
      continue;
    }

    if (bulletPattern.test(trimmed) || orderedPattern.test(trimmed)) {
      const [list, next] = parseList(lines, index);
      blocks.push(list);
      index = next;
      continue;
    }

    // A paragraph runs until a blank line or the start of another block.
    const paragraph: string[] = [];
    while (index < lines.length) {
      const current = lines[index] ?? "";
      if (!current.trim()) break;
      if (paragraph.length > 0 && startsBlock(current)) break;
      paragraph.push(current.trim());
      index += 1;
    }
    blocks.push({
      type: "paragraph",
      inline: parseInline(paragraph.join("\n")),
    });
  }

  return blocks;
}

function startsBlock(line: string): boolean {
  const trimmed = line.trim();
  return (
    /^(```|~~~)/.test(trimmed) ||
    /^#{1,6}\s/.test(trimmed) ||
    trimmed.startsWith(">") ||
    bulletPattern.test(trimmed) ||
    orderedPattern.test(trimmed)
  );
}

function indentOf(line: string): number {
  const match = /^[ \t]*/.exec(line);
  return (match?.[0] ?? "").replace(/\t/g, "    ").length;
}

/** parseList consumes one list, including anything nested inside its items. */
function parseList(lines: string[], start: number): [MDBlock, number] {
  const first = lines[start] ?? "";
  const baseIndent = indentOf(first);
  const ordered = orderedPattern.test(first.trim());
  const startNumber = ordered
    ? Number(orderedPattern.exec(first.trim())?.[1] ?? 1)
    : 1;
  const items: MDListItem[] = [];
  let index = start;

  while (index < lines.length) {
    const line = lines[index] ?? "";
    const trimmed = line.trim();
    if (!trimmed) {
      // A blank line ends the list unless the next line continues it.
      const following = lines[index + 1] ?? "";
      if (!following.trim() || indentOf(following) < baseIndent) break;
      if (!startsBlock(following) && indentOf(following) <= baseIndent) break;
      index += 1;
      continue;
    }
    if (indentOf(line) < baseIndent) break;

    const match = bulletPattern.exec(trimmed) ?? orderedPattern.exec(trimmed);
    if (!match || indentOf(line) > baseIndent) break;
    if (ordered !== orderedPattern.test(trimmed)) break;

    let content = match[2] ?? "";
    let checked: boolean | undefined;
    const task = taskPattern.exec(content);
    if (task) {
      checked = (task[1] ?? " ").toLowerCase() === "x";
      content = task[2] ?? "";
    }

    // Everything indented under the marker belongs to this item.
    const body = [content];
    index += 1;
    while (index < lines.length) {
      const next = lines[index] ?? "";
      if (!next.trim()) {
        const following = lines[index + 1] ?? "";
        if (indentOf(following) > baseIndent && following.trim()) {
          body.push("");
          index += 1;
          continue;
        }
        break;
      }
      if (indentOf(next) <= baseIndent) break;
      body.push(next.slice(Math.min(indentOf(next), baseIndent + 2)));
      index += 1;
    }

    items.push({ blocks: parseBlocks(body), checked });
  }

  return [{ type: "list", ordered, start: startNumber, items }, index];
}

function isTableRow(line: string): boolean {
  const trimmed = line.trim();
  return trimmed.startsWith("|") && trimmed.endsWith("|") && trimmed.length > 2;
}

function isTableDivider(line: string): boolean {
  const trimmed = line.trim();
  if (!isTableRow(trimmed)) return false;
  return splitTableRow(trimmed).every((cell) =>
    /^:?-{1,}:?$/.test(cell.trim()),
  );
}

function splitTableRow(line: string): string[] {
  const trimmed = line.trim().replace(/^\|/, "").replace(/\|$/, "");
  const cells: string[] = [];
  let current = "";
  for (let index = 0; index < trimmed.length; index += 1) {
    const character = trimmed[index];
    if (character === "\\" && trimmed[index + 1] === "|") {
      current += "|";
      index += 1;
      continue;
    }
    if (character === "|") {
      cells.push(current.trim());
      current = "";
      continue;
    }
    current += character;
  }
  cells.push(current.trim());
  return cells;
}

type Delimiter = { token: string; mark: MDMark };

const delimiters: Delimiter[] = [
  { token: "**", mark: { type: "bold" } },
  { token: "__", mark: { type: "bold" } },
  { token: "~~", mark: { type: "strike" } },
  { token: "*", mark: { type: "italic" } },
  { token: "_", mark: { type: "italic" } },
];

/**
 * parseInline reads emphasis, inline code and links.
 *
 * Code spans are read first and never looked inside, so `**` in a code span
 * stays literal — which matters because that is exactly what a model writes
 * when it is explaining Markdown.
 */
export function parseInline(source: string, marks: MDMark[] = []): MDInline[] {
  const out: MDInline[] = [];
  let plain = "";

  const flush = () => {
    if (plain) out.push({ text: plain, marks });
    plain = "";
  };

  let index = 0;
  while (index < source.length) {
    const rest = source.slice(index);

    if (rest.startsWith("\\") && rest.length > 1) {
      plain += rest[1];
      index += 2;
      continue;
    }

    if (rest.startsWith("`")) {
      const fence = /^(`+)/.exec(rest)?.[1] ?? "`";
      const close = source.indexOf(fence, index + fence.length);
      if (close > -1) {
        flush();
        out.push({
          text: source.slice(index + fence.length, close).trim(),
          marks: [...marks, { type: "code" }],
        });
        index = close + fence.length;
        continue;
      }
    }

    const link = /^!?\[([^\]]*)\]\(([^)\s]+)(?:\s+"[^"]*")?\)/.exec(rest);
    if (link) {
      flush();
      const label = link[1] ?? "";
      const href = link[2] ?? "";
      out.push(
        ...parseInline(label || href, [...marks, { type: "link", href }]),
      );
      index += link[0].length;
      continue;
    }

    const auto = /^<((?:https?|mailto):[^>\s]+)>/.exec(rest);
    if (auto) {
      flush();
      const href = auto[1] ?? "";
      out.push({ text: href, marks: [...marks, { type: "link", href }] });
      index += auto[0].length;
      continue;
    }

    const delimiter = delimiters.find((candidate) =>
      rest.startsWith(candidate.token),
    );
    if (delimiter && !marks.some((mark) => mark.type === delimiter.mark.type)) {
      const closing = findClosing(
        source,
        index + delimiter.token.length,
        delimiter.token,
      );
      if (closing > -1) {
        const inner = source.slice(index + delimiter.token.length, closing);
        if (inner.trim()) {
          flush();
          out.push(...parseInline(inner, [...marks, delimiter.mark]));
          index = closing + delimiter.token.length;
          continue;
        }
      }
    }

    plain += source[index];
    index += 1;
  }

  flush();
  return out;
}

/** findClosing skips over code spans so a delimiter inside one is ignored. */
function findClosing(source: string, from: number, token: string): number {
  let index = from;
  while (index < source.length) {
    if (source[index] === "\\") {
      index += 2;
      continue;
    }
    if (source[index] === "`") {
      const fence = /^(`+)/.exec(source.slice(index))?.[1] ?? "`";
      const close = source.indexOf(fence, index + fence.length);
      if (close === -1) return -1;
      index = close + fence.length;
      continue;
    }
    if (source.startsWith(token, index)) {
      // `*` must not match the first half of a `**`.
      if (token.length === 1 && source[index + 1] === token) {
        index += 2;
        continue;
      }
      return index;
    }
    index += 1;
  }
  return -1;
}

/** looksLikeMarkdown reports whether formatting would be lost by ignoring it. */
export function looksLikeMarkdown(value: string): boolean {
  return (
    /(^|\n)\s{0,3}#{1,6}\s/.test(value) ||
    /(^|\n)\s*([-*+]|\d{1,9}[.)])\s+/.test(value) ||
    /(^|\n)\s*>/.test(value) ||
    /(^|\n)\s*\|.*\|\s*(\n|$)/.test(value) ||
    /```/.test(value) ||
    /\*\*[^*\n]+\*\*/.test(value) ||
    /~~[^~\n]+~~/.test(value) ||
    /`[^`\n]+`/.test(value) ||
    /\[[^\]\n]+\]\([^)\s]+\)/.test(value)
  );
}
