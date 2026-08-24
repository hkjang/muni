import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RevisionDiffView } from "./RevisionDiffView";
import type { RevisionDiff } from "./revisionDiff";

function respondWith(diff: RevisionDiff) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      new Response(JSON.stringify({ data: diff }), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    ),
  );
}

function renderView() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <RevisionDiffView documentId="doc-1" from={2} to={3} />
    </QueryClientProvider>,
  );
}

const base: RevisionDiff = {
  from: {
    revision: 2,
    createdAt: "2026-08-24T10:00:00Z",
    reason: "초안",
    author: { id: "u1", displayName: "홍길동" },
  },
  to: {
    revision: 3,
    createdAt: "2026-08-24T11:00:00Z",
    reason: "수정",
    author: { id: "u2", displayName: "김철수" },
  },
  summary: { added: 1, removed: 1, changed: 1, moved: 0, unchanged: 2 },
  blocks: [],
};

afterEach(() => vi.unstubAllGlobals());

describe("RevisionDiffView", () => {
  it("summarises the change and shows each touched block", async () => {
    respondWith({
      ...base,
      blocks: [
        { status: "unchanged", type: "heading", blockId: "blk_h", after: "제목", fromIndex: 0, toIndex: 0 },
        { status: "added", type: "paragraph", blockId: "blk_new", after: "새로 추가된 문단", fromIndex: -1, toIndex: 1 },
        {
          status: "changed",
          type: "paragraph",
          blockId: "blk_b",
          before: "일정은 3월에 시작합니다",
          after: "일정은 5월에 시작합니다",
          fromIndex: 2,
          toIndex: 2,
          inline: [
            { op: "equal", text: "일정은 " },
            { op: "delete", text: "3" },
            { op: "insert", text: "5" },
            { op: "equal", text: "월에 시작합니다" },
          ],
        },
        { status: "removed", type: "paragraph", blockId: "blk_c", before: "삭제될 문단", fromIndex: 3, toIndex: -1 },
      ],
    });
    renderView();

    await waitFor(() =>
      expect(screen.getByText("Revision 2 → 3")).toBeDefined(),
    );
    expect(screen.getByText("추가 1 · 변경 1 · 삭제 1")).toBeDefined();

    expect(screen.getByText("새로 추가된 문단")).toBeDefined();
    expect(screen.getByText("삭제될 문단")).toBeDefined();
    // The inline breakdown is rendered piece by piece.
    expect(screen.getByText("3")).toBeDefined();
    expect(screen.getByText("5")).toBeDefined();

    // An unchanged block is counted, not listed.
    expect(screen.queryByText("제목")).toBeNull();
    expect(screen.getByText(/변경되지 않은 블록 2개/)).toBeDefined();
  });

  it("says so when the two revisions are identical", async () => {
    respondWith({
      ...base,
      summary: { added: 0, removed: 0, changed: 0, moved: 0, unchanged: 4 },
      blocks: [
        { status: "unchanged", type: "paragraph", after: "본문", fromIndex: 0, toIndex: 0 },
      ],
    });
    renderView();
    await waitFor(() =>
      expect(screen.getByText("두 버전의 내용이 같습니다.")).toBeDefined(),
    );
    expect(screen.getByText("변경 없음")).toBeDefined();
  });

  it("shows where a moved block went", async () => {
    respondWith({
      ...base,
      summary: { added: 0, removed: 0, changed: 0, moved: 1, unchanged: 1 },
      blocks: [
        { status: "moved", type: "paragraph", blockId: "blk_a", after: "옮겨진 문단", fromIndex: 0, toIndex: 2 },
      ],
    });
    renderView();
    await waitFor(() => expect(screen.getByText("옮겨진 문단")).toBeDefined());
    expect(screen.getByText("1번째 → 3번째")).toBeDefined();
  });

  it("reports a failed request instead of rendering nothing", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        new Response(JSON.stringify({ error: { code: "X", message: "비교하지 못했습니다." } }), {
          status: 500,
          headers: { "content-type": "application/json" },
        }),
      ),
    );
    renderView();
    await waitFor(() =>
      expect(screen.getByText("비교하지 못했습니다.")).toBeDefined(),
    );
  });
});
