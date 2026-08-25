import { describe, expect, it } from "vitest";
import { buildThreads, sortThreads } from "./threads";
import type { CommentItem } from "../../../types";

function comment(
  id: string,
  createdAt: string,
  extra: Partial<CommentItem> = {},
): CommentItem {
  return {
    id,
    author: { id: "u1", displayName: "홍길동" },
    body: id,
    createdAt,
    updatedAt: createdAt,
    ...extra,
  };
}

describe("buildThreads", () => {
  it("puts a reply under the comment it answers", () => {
    const threads = buildThreads([
      comment("a", "2026-08-01T10:00:00Z"),
      comment("b", "2026-08-01T11:00:00Z", { parentId: "a" }),
    ]);
    expect(threads).toHaveLength(1);
    expect(threads[0]?.replies.map((reply) => reply.id)).toEqual(["b"]);
  });

  it("keeps a reply to a reply in the same thread", () => {
    const threads = buildThreads([
      comment("a", "2026-08-01T10:00:00Z"),
      comment("b", "2026-08-01T11:00:00Z", { parentId: "a" }),
      comment("c", "2026-08-01T12:00:00Z", { parentId: "b" }),
    ]);
    expect(threads).toHaveLength(1);
    expect(threads[0]?.replies.map((reply) => reply.id)).toEqual(["b", "c"]);
  });

  it("orders replies oldest first", () => {
    const threads = buildThreads([
      comment("a", "2026-08-01T10:00:00Z"),
      comment("late", "2026-08-01T13:00:00Z", { parentId: "a" }),
      comment("early", "2026-08-01T11:00:00Z", { parentId: "a" }),
    ]);
    expect(threads[0]?.replies.map((reply) => reply.id)).toEqual([
      "early",
      "late",
    ]);
  });

  it("keeps a reply whose parent is gone rather than dropping it", () => {
    const threads = buildThreads([
      comment("orphan", "2026-08-01T10:00:00Z", { parentId: "deleted" }),
    ]);
    expect(threads).toHaveLength(1);
    expect(threads[0]?.root.id).toBe("orphan");
  });

  it("does not hang on a cycle", () => {
    const threads = buildThreads([
      comment("a", "2026-08-01T10:00:00Z", { parentId: "b" }),
      comment("b", "2026-08-01T11:00:00Z", { parentId: "a" }),
    ]);
    expect(threads.length).toBeGreaterThan(0);
  });

  it("has nothing for a document without comments", () => {
    expect(buildThreads([])).toEqual([]);
  });
});

describe("sortThreads", () => {
  it("puts open threads before resolved ones", () => {
    const threads = buildThreads([
      comment("resolved", "2026-08-02T10:00:00Z", {
        resolvedAt: "2026-08-02T12:00:00Z",
      }),
      comment("open", "2026-08-01T10:00:00Z"),
    ]);
    expect(sortThreads(threads).map((thread) => thread.root.id)).toEqual([
      "open",
      "resolved",
    ]);
  });

  it("puts the newest open thread first", () => {
    const threads = buildThreads([
      comment("older", "2026-08-01T10:00:00Z"),
      comment("newer", "2026-08-03T10:00:00Z"),
    ]);
    expect(sortThreads(threads).map((thread) => thread.root.id)).toEqual([
      "newer",
      "older",
    ]);
  });
});
