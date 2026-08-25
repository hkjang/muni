import { describe, expect, it } from "vitest";
import { readAnchor, resolveWithin } from "./anchor";

describe("resolveWithin", () => {
  it("finds the commented text inside the block", () => {
    // The block starts at editor position 10.
    expect(resolveWithin("예산은 3억원입니다", "3억원", 10, 29)).toEqual({
      from: 14,
      to: 17,
    });
  });

  it("falls back to the whole block when the text is gone", () => {
    expect(resolveWithin("문장이 완전히 바뀌었습니다", "3억원", 10, 29)).toEqual({
      from: 10,
      to: 29,
    });
  });

  it("uses the whole block when nothing was selected", () => {
    expect(resolveWithin("본문", "  ", 4, 10)).toEqual({ from: 4, to: 10 });
  });
});

describe("readAnchor", () => {
  it("reads a comment written before block ids existed", () => {
    expect(readAnchor({ from: 3, to: 9, selectedText: "옛 댓글" })).toEqual({
      from: 3,
      to: 9,
      blockId: undefined,
      selectedText: "옛 댓글",
    });
  });

  it("ignores a value of the wrong shape", () => {
    expect(readAnchor("nonsense")).toEqual({});
    expect(readAnchor(null)).toEqual({});
    expect(readAnchor({ from: "3" })).toEqual({
      from: undefined,
      to: undefined,
      blockId: undefined,
      selectedText: undefined,
    });
  });
});
