import { describe, expect, it } from "vitest";
import { countWords, documentStats } from "./documentStats";

describe("countWords", () => {
  it("counts Korean words by their spaces", () => {
    expect(countWords("오늘 회의 결과를 정리합니다")).toBe(4);
  });

  it("counts nothing for an empty document", () => {
    expect(countWords("   \n\n ")).toBe(0);
  });

  it("does not count a line break as a word", () => {
    expect(countWords("첫 줄\n둘째 줄")).toBe(4);
  });
});

describe("documentStats", () => {
  it("counts characters with and without spaces", () => {
    const stats = documentStats("회의 결과");
    expect(stats.characters).toBe(5);
    expect(stats.charactersNoSpaces).toBe(4);
  });

  it("counts paragraphs, ignoring blank lines", () => {
    expect(documentStats("첫 문단\n\n둘째 문단\n\n\n").paragraphs).toBe(2);
  });

  it("gives an empty document no reading time", () => {
    expect(documentStats("").readingMinutes).toBe(0);
  });

  it("rounds a short document up to one minute", () => {
    expect(documentStats("짧은 글").readingMinutes).toBe(1);
  });

  it("counts an emoji as one character", () => {
    expect(documentStats("👍").characters).toBe(1);
  });
});
