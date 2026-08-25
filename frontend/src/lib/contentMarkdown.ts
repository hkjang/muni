import type { EditorContent } from "./markdownContent";

/**
 * contentToMarkdown writes editor content back out as Markdown.
 *
 * This is what an AI rewrite sends: handing the model plain text throws away
 * the bold runs, links and list structure of the selection, and whatever the
 * model returns then replaces formatted text with unformatted text. Markdown
 * survives the round trip in both directions.
 */
export function contentToMarkdown(nodes: EditorContent[]): string {
  return blocks(nodes)
    .join("\n\n")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
}

function blocks(nodes: EditorContent[]): string[] {
  const out: string[] = [];
  for (const node of nodes) {
    const text = block(node);
    if (text !== null) out.push(text);
  }
  return out;
}

function children(node: EditorContent): EditorContent[] {
  const content = node.content;
  return Array.isArray(content) ? (content as EditorContent[]) : [];
}

function block(node: EditorContent, depth = 0): string | null {
  const type = String(node.type ?? "");
  const attrs = (node.attrs ?? {}) as Record<string, unknown>;
  switch (type) {
    case "paragraph":
      return inline(children(node));
    case "heading": {
      const level = Math.min(6, Math.max(1, Number(attrs.level ?? 1)));
      return "#".repeat(level) + " " + inline(children(node));
    }
    case "horizontalRule":
      return "---";
    case "codeBlock": {
      const language = typeof attrs.language === "string" ? attrs.language : "";
      return "```" + language + "\n" + plain(children(node)) + "\n```";
    }
    case "blockquote":
      return blocks(children(node))
        .join("\n\n")
        .split("\n")
        .map((line) => (line ? "> " + line : ">"))
        .join("\n");
    case "bulletList":
    case "orderedList": {
      const ordered = type === "orderedList";
      const start = Number(attrs.start ?? 1) || 1;
      return children(node)
        .map((item, index) =>
          listItem(item, ordered ? `${start + index}. ` : "- ", depth),
        )
        .join("\n");
    }
    case "taskList":
      return children(node)
        .map((item) => {
          const checked = Boolean(
            (item.attrs as Record<string, unknown>)?.checked,
          );
          return listItem(item, checked ? "- [x] " : "- [ ] ", depth);
        })
        .join("\n");
    case "table":
      return table(node);
    case "image": {
      const src = typeof attrs.src === "string" ? attrs.src : "";
      const alt = typeof attrs.alt === "string" ? attrs.alt : "";
      return src ? `![${alt}](${src})` : null;
    }
    default: {
      const nested = children(node);
      if (nested.length > 0) return blocks(nested).join("\n\n");
      if (typeof node.text === "string") return node.text;
      return null;
    }
  }
}

function listItem(item: EditorContent, marker: string, depth: number): string {
  const inner = children(item)
    .map((child) => block(child, depth + 1) ?? "")
    .filter((value) => value !== "");
  const indent = "  ".repeat(depth);
  const body = inner.join("\n\n");
  return body
    .split("\n")
    .map((line, index) =>
      index === 0 ? indent + marker + line : indent + "  " + line,
    )
    .join("\n");
}

function table(node: EditorContent): string {
  const rows = children(node).map((row) =>
    children(row).map((cell) =>
      inline(cellInline(cell)).replace(/\n+/g, " ").replace(/\|/g, "\\|"),
    ),
  );
  if (rows.length === 0) return "";
  const width = Math.max(...rows.map((row) => row.length));
  const line = (cells: string[]) => {
    const padded = [...cells];
    while (padded.length < width) padded.push("");
    return "| " + padded.join(" | ") + " |";
  };
  const [header, ...body] = rows;
  return [
    line(header ?? []),
    "| " + Array.from({ length: width }, () => "---").join(" | ") + " |",
    ...body.map(line),
  ].join("\n");
}

function cellInline(cell: EditorContent): EditorContent[] {
  // A cell holds paragraphs; Markdown tables hold one line per cell.
  return children(cell).flatMap((child) => children(child));
}

function plain(nodes: EditorContent[]): string {
  return nodes
    .map((node) => (typeof node.text === "string" ? node.text : ""))
    .join("");
}

const wrappers: Record<string, string> = {
  bold: "**",
  strong: "**",
  italic: "*",
  em: "*",
  strike: "~~",
  code: "`",
};

function inline(nodes: EditorContent[]): string {
  return nodes
    .map((node) => {
      const type = String(node.type ?? "");
      if (type === "hardBreak") return "\n";
      if (type !== "text") {
        const nested = block(node);
        return nested ?? "";
      }
      let text = String(node.text ?? "");
      const marks = Array.isArray(node.marks)
        ? (node.marks as EditorContent[])
        : [];
      // Code is applied innermost: `**a**` inside a code span is literal.
      const ordered = [...marks].sort(
        (left, right) =>
          Number(String(right.type) === "code") -
          Number(String(left.type) === "code"),
      );
      let href = "";
      for (const mark of ordered) {
        const name = String(mark.type ?? "");
        if (name === "link") {
          const attrs = (mark.attrs ?? {}) as Record<string, unknown>;
          href = typeof attrs.href === "string" ? attrs.href : "";
          continue;
        }
        const wrapper = wrappers[name];
        if (wrapper && text.trim()) text = wrapper + text + wrapper;
      }
      if (href) text = `[${text}](${href})`;
      return text;
    })
    .join("");
}
