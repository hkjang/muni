import { describe, expect, it } from "vitest";
import { headingNumbers, validScheme } from "./numbering";

describe("headingNumbers", () => {
  const depths = [0, 1, 1, 2, 0];

  it("numbers sections in decimal", () => {
    expect(headingNumbers(depths, "decimal")).toEqual([
      "1.",
      "1.1.",
      "1.2.",
      "1.2.1.",
      "2.",
    ]);
  });

  it("numbers sections the way a Korean report does", () => {
    expect(headingNumbers(depths, "korean")).toEqual([
      "I.",
      "1.",
      "2.",
      "가.",
      "II.",
    ]);
  });

  it("starts the next chapter's sections at one again", () => {
    expect(headingNumbers([0, 1, 0, 1], "decimal")).toEqual([
      "1.",
      "1.1.",
      "2.",
      "2.1.",
    ]);
  });

  it("labels nothing when numbering is off", () => {
    expect(headingNumbers(depths, "none")).toEqual(["", "", "", "", ""]);
  });

  it("keeps going past the letters it has names for", () => {
    const many = Array.from({ length: 16 }, () => 2);
    const labels = headingNumbers([0, ...many], "korean");
    expect(labels[1]).toBe("가.");
    // 가나다 runs to 하, and past that it counts instead of inventing letters.
    expect(labels[14]).toBe("하.");
    expect(labels[15]).toBe("15.");
  });

  it("has nothing to number in a document without headings", () => {
    expect(headingNumbers([], "decimal")).toEqual([]);
  });

  it("matches what the server writes into the export", () => {
    // The two implementations have to agree or the screen and the exported
    // file disagree about what section 2.1 is.
    expect(headingNumbers([0, 1, 1, 2, 0], "decimal")).toEqual([
      "1.",
      "1.1.",
      "1.2.",
      "1.2.1.",
      "2.",
    ]);
  });
});

describe("validScheme", () => {
  it("accepts the two schemes", () => {
    expect(validScheme("decimal")).toBe("decimal");
    expect(validScheme("korean")).toBe("korean");
  });

  it("treats anything else as no numbering", () => {
    expect(validScheme("nonsense")).toBe("none");
    expect(validScheme(undefined)).toBe("none");
    expect(validScheme(null)).toBe("none");
  });
});
