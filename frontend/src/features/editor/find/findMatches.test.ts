import { describe, expect, it } from "vitest";
import { findMatches, nextMatchIndex, step } from "./findMatches";

describe("findMatches", () => {
  it("finds every occurrence", () => {
    expect(findMatches("회의 자료와 회의 결과", "회의")).toEqual([
      { start: 0, end: 2 },
      { start: 7, end: 9 },
    ]);
  });

  it("ignores case unless asked", () => {
    expect(findMatches("Muni and MUNI", "muni")).toHaveLength(2);
    expect(findMatches("Muni and MUNI", "muni", { caseSensitive: true })).toHaveLength(0);
  });

  it("matches a whole word only when asked", () => {
    const text = "회의 회의록";
    expect(findMatches(text, "회의")).toHaveLength(2);
    // 회의록 is one word in Korean, so 회의 is not a whole word inside it.
    expect(findMatches(text, "회의", { wholeWord: true })).toEqual([
      { start: 0, end: 2 },
    ]);
  });

  it("treats punctuation as a word boundary", () => {
    expect(findMatches("계획(안)", "계획", { wholeWord: true })).toHaveLength(1);
  });

  it("finds nothing for an empty query", () => {
    expect(findMatches("본문", "")).toEqual([]);
  });

  it("does not loop forever on a one-character query", () => {
    expect(findMatches("aaa", "a")).toHaveLength(3);
  });

  it("does not return overlapping matches", () => {
    expect(findMatches("aaaa", "aa")).toEqual([
      { start: 0, end: 2 },
      { start: 2, end: 4 },
    ]);
  });
});

describe("nextMatchIndex", () => {
  const matches = [
    { start: 10, end: 12 },
    { start: 40, end: 42 },
    { start: 90, end: 92 },
  ];

  it("moves forward from the caret", () => {
    expect(nextMatchIndex(matches, 20)).toBe(1);
  });

  it("wraps to the first match past the last one", () => {
    expect(nextMatchIndex(matches, 200)).toBe(0);
  });

  it("moves backwards to the match before the caret", () => {
    expect(nextMatchIndex(matches, 50, -1)).toBe(1);
  });

  it("wraps to the last match when searching back from the top", () => {
    expect(nextMatchIndex(matches, 0, -1)).toBe(2);
  });

  it("has nothing to land on when there are no matches", () => {
    expect(nextMatchIndex([], 5)).toBe(-1);
  });
});

describe("step", () => {
  it("wraps at both ends", () => {
    expect(step(2, 3, 1)).toBe(0);
    expect(step(0, 3, -1)).toBe(2);
  });

  it("stays at nothing when there is nothing", () => {
    expect(step(-1, 0, 1)).toBe(-1);
  });
});
