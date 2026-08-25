import type { CommentItem } from "../../../types";

export type CommentThread = {
  root: CommentItem;
  replies: CommentItem[];
};

/**
 * buildThreads groups comments into the conversations they belong to.
 *
 * A comment is a discussion, not a note: someone answers, and the answer
 * belongs under the question. The server has always stored a parent; the panel
 * listed every comment flat, so a reply appeared as an unrelated remark
 * somewhere further down.
 */
export function buildThreads(comments: CommentItem[]): CommentThread[] {
  const byId = new Map(comments.map((comment) => [comment.id, comment]));
  const threads = new Map<string, CommentThread>();
  const order: string[] = [];

  for (const comment of comments) {
    if (comment.parentId && byId.has(comment.parentId)) continue;
    threads.set(comment.id, { root: comment, replies: [] });
    order.push(comment.id);
  }

  for (const comment of comments) {
    if (!comment.parentId) continue;
    // A reply to a reply still belongs to the thread it started in.
    const rootId = findRoot(comment, byId);
    const thread = rootId ? threads.get(rootId) : undefined;
    if (thread) thread.replies.push(comment);
    else if (!threads.has(comment.id)) {
      // The parent is gone; the reply is a remark of its own rather than lost.
      threads.set(comment.id, { root: comment, replies: [] });
      order.push(comment.id);
    }
  }

  for (const thread of threads.values())
    thread.replies.sort((left, right) =>
      left.createdAt.localeCompare(right.createdAt),
    );

  return order.map((id) => threads.get(id)!).filter(Boolean);
}

function findRoot(
  comment: CommentItem,
  byId: Map<string, CommentItem>,
): string | undefined {
  const seen = new Set<string>();
  let current: CommentItem | undefined = comment;
  while (current?.parentId) {
    if (seen.has(current.id)) return undefined; // A cycle cannot be a thread.
    seen.add(current.id);
    current = byId.get(current.parentId);
  }
  return current?.id;
}

/** A thread counts as resolved when the comment that started it is. */
export function isResolved(thread: CommentThread): boolean {
  return Boolean(thread.root.resolvedAt);
}

/** Open threads come first: they are the ones still asking for something. */
export function sortThreads(threads: CommentThread[]): CommentThread[] {
  return [...threads].sort((left, right) => {
    const difference = Number(isResolved(left)) - Number(isResolved(right));
    if (difference !== 0) return difference;
    return right.root.createdAt.localeCompare(left.root.createdAt);
  });
}
