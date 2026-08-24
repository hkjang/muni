import { describe, expect, it } from "vitest";
import { isBusy, statusLabel, suggestSlideCount } from "./types";

describe("suggestSlideCount", () => {
  it("follows the time available, not the length of the document", () => {
    expect(suggestSlideCount(10, "간결")).toBe(8);
    expect(suggestSlideCount(30, "보통")).toBe(29);
    expect(suggestSlideCount(60, "상세")).toBe(50);
  });

  it("stays inside what a deck can hold", () => {
    expect(suggestSlideCount(1, "간결")).toBeGreaterThanOrEqual(3);
    expect(suggestSlideCount(600, "상세")).toBeLessThanOrEqual(50);
  });
});

describe("status", () => {
  it("names every state in Korean", () => {
    for (const status of ["pending", "draft", "queued", "generating", "completed", "failed"] as const) {
      expect(statusLabel(status).length).toBeGreaterThan(0);
    }
  });

  it("keeps polling only while generation is unfinished", () => {
    expect(isBusy("queued")).toBe(true);
    expect(isBusy("generating")).toBe(true);
    expect(isBusy("completed")).toBe(false);
    expect(isBusy("failed")).toBe(false);
  });
});
