/**
 * The insert menu that opens when a line starts with "/".
 *
 * Reaching a toolbar means leaving the keyboard, and the toolbar cannot hold
 * everything anyway. Typing what you want and pressing Enter is how Google
 * Docs' "@" menu works and it is the fastest path to a heading, a table or a
 * page break.
 */

export type InsertCommand = {
  id: string;
  label: string;
  /** Words that should also find it, including the English name. */
  keywords: string;
  group: "서식" | "삽입";
  hint?: string;
};

export const insertCommands: InsertCommand[] = [
  {
    id: "h1",
    label: "제목 1",
    keywords: "heading title h1 제목",
    group: "서식",
    hint: "# ",
  },
  {
    id: "h2",
    label: "제목 2",
    keywords: "heading h2 제목",
    group: "서식",
    hint: "## ",
  },
  {
    id: "h3",
    label: "제목 3",
    keywords: "heading h3 제목",
    group: "서식",
    hint: "### ",
  },
  {
    id: "paragraph",
    label: "본문",
    keywords: "paragraph text body 본문",
    group: "서식",
  },
  {
    id: "bulletList",
    label: "글머리 기호 목록",
    keywords: "bullet list ul 목록",
    group: "서식",
    hint: "- ",
  },
  {
    id: "orderedList",
    label: "번호 매기기 목록",
    keywords: "ordered list ol number 목록 번호",
    group: "서식",
    hint: "1. ",
  },
  {
    id: "taskList",
    label: "체크리스트",
    keywords: "task todo checklist 체크 할일",
    group: "서식",
  },
  {
    id: "blockquote",
    label: "인용",
    keywords: "quote blockquote 인용",
    group: "서식",
    hint: "> ",
  },
  {
    id: "codeBlock",
    label: "코드 블록",
    keywords: "code block 코드",
    group: "서식",
    hint: "```",
  },
  {
    id: "mermaid",
    label: "도형 (mermaid)",
    keywords: "mermaid diagram 도형 다이어그램 순서도 flowchart",
    group: "삽입",
    hint: "```mermaid",
  },
  { id: "table", label: "표", keywords: "table grid 표", group: "삽입" },
  {
    id: "image",
    label: "이미지",
    keywords: "image picture photo 이미지 그림 사진",
    group: "삽입",
  },
  {
    id: "horizontalRule",
    label: "가로 구분선",
    keywords: "divider rule hr 구분선",
    group: "삽입",
  },
  {
    id: "pageBreak",
    label: "페이지 나누기",
    keywords: "page break 페이지 나누기 인쇄",
    group: "삽입",
  },
  {
    id: "tableOfContents",
    label: "목차",
    keywords: "toc table of contents 목차 차례",
    group: "삽입",
  },
  {
    id: "date",
    label: "오늘 날짜",
    keywords: "date today 날짜 오늘",
    group: "삽입",
  },
];

/**
 * readSlashQuery decides whether the menu should be open.
 *
 * The trigger has to start a word — otherwise every date written 2026/08 and
 * every path in a sentence would open a menu. The query stops at the first
 * character that cannot be part of a command name so a stray slash mid-typing
 * closes the menu rather than leaving it stuck open.
 */
export function readSlashQuery(
  textBefore: string,
): { query: string; length: number } | null {
  const slash = textBefore.lastIndexOf("/");
  if (slash < 0) return null;
  const before = textBefore[slash - 1];
  if (before !== undefined && !/\s/u.test(before)) return null;
  const query = textBefore.slice(slash + 1);
  if (query.length > 20) return null;
  if (/[/\n\t]/u.test(query)) return null;
  return { query, length: query.length + 1 };
}

/**
 * matchInsertCommands ranks the commands against what has been typed. An empty
 * query lists everything, which is what makes the menu discoverable.
 */
export function matchInsertCommands(
  commands: InsertCommand[],
  query: string,
): InsertCommand[] {
  const needle = query.trim().toLowerCase();
  if (!needle) return commands;
  return commands
    .map((command) => ({ command, score: scoreCommand(command, needle) }))
    .filter((entry) => entry.score > 0)
    .sort((left, right) => right.score - left.score)
    .map((entry) => entry.command);
}

function scoreCommand(command: InsertCommand, needle: string): number {
  const label = command.label.toLowerCase();
  if (label === needle) return 100;
  if (label.startsWith(needle)) return 80;
  if (label.replace(/\s/gu, "").startsWith(needle)) return 70;
  if (label.includes(needle)) return 50;
  const keywords = command.keywords.toLowerCase();
  if (keywords.split(/\s+/u).some((word) => word.startsWith(needle))) return 40;
  if (keywords.includes(needle)) return 20;
  return 0;
}

/** groupInsertCommands keeps the two sections in a fixed order. */
export function groupInsertCommands(commands: InsertCommand[]) {
  const groups: InsertCommand["group"][] = ["서식", "삽입"];
  return groups
    .map((group) => ({
      group,
      commands: commands.filter((command) => command.group === group),
    }))
    .filter((entry) => entry.commands.length > 0);
}

/** todayLabel is the date in the form Korean documents are written in. */
export function todayLabel(now: Date): string {
  const year = now.getFullYear();
  const month = String(now.getMonth() + 1).padStart(2, "0");
  const day = String(now.getDate()).padStart(2, "0");
  return `${year}. ${month}. ${day}.`;
}
