import { describe, expect, it } from "vitest";
import { humanBytes } from "./AdminOverviewPage";

describe("humanBytes", () => {
  it("reads a size the way an operator would say it", () => {
    expect(humanBytes(0)).toBe("0 B");
    expect(humanBytes(900)).toBe("900 B");
    expect(humanBytes(1536)).toBe("1.5 KB");
    expect(humanBytes(5 * 1024 * 1024)).toBe("5.0 MB");
    expect(humanBytes(2.5 * 1024 * 1024 * 1024)).toBe("2.5 GB");
  });

  it("drops the decimal once the number is large enough not to need it", () => {
    expect(humanBytes(150 * 1024)).toBe("150 KB");
  });

  it("stops at terabytes rather than inventing a unit", () => {
    expect(humanBytes(5 * 1024 ** 5)).toContain("TB");
  });
});
