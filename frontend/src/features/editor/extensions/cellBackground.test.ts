import { describe, expect, it } from "vitest";
import { cellShades, normalizeShade, rgbToHex } from "./cellBackground";

describe("normalizeShade", () => {
  it("accepts a plain hex colour", () => {
    expect(normalizeShade("#E8F0FE")).toBe("#e8f0fe");
  });

  it("treats no shade as no shade", () => {
    expect(normalizeShade("")).toBe("");
    expect(normalizeShade(null)).toBe("");
    expect(normalizeShade(undefined)).toBe("");
  });

  it("refuses anything that is not a colour", () => {
    expect(normalizeShade("url(javascript:alert(1))")).toBe("");
    expect(normalizeShade("red; background-image: url(x)")).toBe("");
    expect(normalizeShade("#fff")).toBe("");
  });
});

describe("rgbToHex", () => {
  it("reads back what a browser reports", () => {
    expect(rgbToHex("rgb(232, 240, 254)")).toBe("#e8f0fe");
    expect(rgbToHex("rgba(232, 240, 254, 1)")).toBe("#e8f0fe");
  });

  it("leaves a value it does not recognise alone", () => {
    expect(rgbToHex("#e8f0fe")).toBe("#e8f0fe");
  });
});

describe("cellShades", () => {
  it("offers a clearing option first", () => {
    expect(cellShades[0]?.value).toBe("");
  });

  it("offers only shades that survive the normaliser", () => {
    for (const shade of cellShades.slice(1))
      expect(normalizeShade(shade.value)).toBe(shade.value);
  });
});
