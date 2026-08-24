export type BlockStatus = "unchanged" | "added" | "removed" | "changed" | "moved";

export type InlineOp = {
  op: "equal" | "insert" | "delete";
  text: string;
};

export type BlockDiff = {
  status: BlockStatus;
  blockId?: string;
  type: string;
  before?: string;
  after?: string;
  inline?: InlineOp[];
  fromIndex: number;
  toIndex: number;
};

export type RevisionRef = {
  revision: number;
  createdAt: string;
  reason?: string | null;
  name?: string | null;
  author: { id: string; displayName: string };
};

export type RevisionDiff = {
  from: RevisionRef;
  to: RevisionRef;
  summary: {
    added: number;
    removed: number;
    changed: number;
    moved: number;
    unchanged: number;
  };
  blocks: BlockDiff[];
};

/** Korean labels for the block kinds the diff reports. */
export function blockTypeLabel(type: string): string {
  switch (type) {
    case "heading":
      return "제목";
    case "paragraph":
      return "문단";
    case "codeBlock":
      return "코드";
    case "table":
      return "표";
    case "image":
      return "그림";
    case "horizontalRule":
      return "구분선";
    default:
      return type;
  }
}

export function blockStatusLabel(status: BlockStatus): string {
  switch (status) {
    case "added":
      return "추가";
    case "removed":
      return "삭제";
    case "changed":
      return "변경";
    case "moved":
      return "이동";
    default:
      return "유지";
  }
}

/**
 * describeChanges turns the summary into the one line a reader wants before
 * they decide whether to look at the detail.
 */
export function describeChanges(summary: RevisionDiff["summary"]): string {
  const parts: string[] = [];
  if (summary.added) parts.push(`추가 ${summary.added}`);
  if (summary.changed) parts.push(`변경 ${summary.changed}`);
  if (summary.removed) parts.push(`삭제 ${summary.removed}`);
  if (summary.moved) parts.push(`이동 ${summary.moved}`);
  if (parts.length === 0) return "변경 없음";
  return parts.join(" · ");
}

/** Blocks that carry no change are hidden unless the reader asks for them. */
export function hasChange(block: BlockDiff): boolean {
  return block.status !== "unchanged";
}
