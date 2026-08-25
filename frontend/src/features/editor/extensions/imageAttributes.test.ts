import { describe, expect, it } from "vitest";
import { contentWidth, percentFor, pixelsFor } from "./imageAttributes";

describe("image sizing", () => {
  it("turns a preset into the width that is stored", () => {
    expect(pixelsFor(100)).toBe(contentWidth);
    expect(pixelsFor(50)).toBe(Math.round(contentWidth / 2));
  });

  it("reads a stored width back as its preset", () => {
    expect(percentFor(pixelsFor(25))).toBe(25);
    expect(percentFor(pixelsFor(75))).toBe(75);
  });

  it("treats an image with no width as full width", () => {
    expect(percentFor(null)).toBe(100);
  });

  it("reports nothing for a width that is not one of the presets", () => {
    expect(percentFor(123)).toBeNull();
  });

  it("tolerates rounding either way", () => {
    expect(percentFor(pixelsFor(50) + 2)).toBe(50);
  });
});
