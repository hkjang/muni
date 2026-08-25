import { describe, expect, it } from "vitest";
import { buildOutline, currentOutlineIndex } from "./outline";

describe("buildOutline", () => {
  it("indents each level one step further", () => {
    const outline = buildOutline([
      { level: 1, text: "개요", pos: 1 },
      { level: 2, text: "배경", pos: 20 },
      { level: 3, text: "선행 사례", pos: 40 },
      { level: 2, text: "목표", pos: 60 },
    ]);
    expect(outline.map((item) => item.depth)).toEqual([0, 1, 2, 1]);
  });

  it("does not push a document written in h2 and h3 off to the right", () => {
    const outline = buildOutline([
      { level: 2, text: "첫 절", pos: 1 },
      { level: 3, text: "세부", pos: 20 },
    ]);
    expect(outline.map((item) => item.depth)).toEqual([0, 1]);
  });

  it("indents a skipped level by one step, not by the gap", () => {
    const outline = buildOutline([
      { level: 1, text: "제목", pos: 1 },
      { level: 4, text: "갑자기 깊은 제목", pos: 20 },
    ]);
    expect(outline.map((item) => item.depth)).toEqual([0, 1]);
  });

  it("comes back out when the level rises again", () => {
    const outline = buildOutline([
      { level: 1, text: "1장", pos: 1 },
      { level: 2, text: "1.1", pos: 10 },
      { level: 3, text: "1.1.1", pos: 20 },
      { level: 1, text: "2장", pos: 30 },
    ]);
    expect(outline.map((item) => item.depth)).toEqual([0, 1, 2, 0]);
  });

  it("leaves out an empty heading", () => {
    const outline = buildOutline([
      { level: 1, text: "  ", pos: 1 },
      { level: 1, text: "실제 제목", pos: 10 },
    ]);
    expect(outline).toHaveLength(1);
  });

  it("shortens a heading that would not fit", () => {
    const outline = buildOutline([{ level: 1, text: "가".repeat(200), pos: 1 }]);
    expect(outline[0]?.text.endsWith("…")).toBe(true);
    expect(outline[0]?.text.length).toBeLessThan(100);
  });

  it("has nothing to show for a document without headings", () => {
    expect(buildOutline([])).toEqual([]);
  });
});

describe("currentOutlineIndex", () => {
  const outline = buildOutline([
    { level: 1, text: "개요", pos: 1 },
    { level: 1, text: "본론", pos: 100 },
    { level: 1, text: "결론", pos: 250 },
  ]);

  it("is the last heading at or before the caret", () => {
    expect(currentOutlineIndex(outline, 150)).toBe(1);
    expect(currentOutlineIndex(outline, 250)).toBe(2);
  });

  it("is nothing when the caret sits above every heading", () => {
    expect(currentOutlineIndex(outline, 0)).toBe(-1);
  });
});
