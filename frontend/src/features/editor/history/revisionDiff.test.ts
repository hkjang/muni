import { describe, expect, it } from "vitest";
import {
  blockStatusLabel,
  blockTypeLabel,
  describeChanges,
  hasChange,
} from "./revisionDiff";

describe("describeChanges", () => {
  it("says so plainly when nothing changed", () => {
    expect(
      describeChanges({ added: 0, removed: 0, changed: 0, moved: 0, unchanged: 9 }),
    ).toBe("변경 없음");
  });

  it("lists only the kinds of change that occurred", () => {
    expect(
      describeChanges({ added: 2, removed: 0, changed: 1, moved: 0, unchanged: 5 }),
    ).toBe("추가 2 · 변경 1");
  });

  it("keeps a stable order so the line does not jump around", () => {
    expect(
      describeChanges({ added: 1, removed: 1, changed: 1, moved: 1, unchanged: 0 }),
    ).toBe("추가 1 · 변경 1 · 삭제 1 · 이동 1");
  });
});

describe("labels", () => {
  it("names the block kinds the diff reports", () => {
    expect(blockTypeLabel("heading")).toBe("제목");
    expect(blockTypeLabel("table")).toBe("표");
    expect(blockTypeLabel("somethingNew")).toBe("somethingNew");
  });

  it("names every status", () => {
    for (const status of ["added", "removed", "changed", "moved", "unchanged"] as const) {
      expect(blockStatusLabel(status).length).toBeGreaterThan(0);
    }
  });
});

describe("hasChange", () => {
  it("hides only the untouched blocks", () => {
    expect(hasChange({ status: "unchanged", type: "paragraph", fromIndex: 0, toIndex: 0 })).toBe(false);
    expect(hasChange({ status: "moved", type: "paragraph", fromIndex: 0, toIndex: 2 })).toBe(true);
  });
});
