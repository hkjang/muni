import { describe, expect, it } from "vitest";
import { normalizeHref } from "./LinkMenu";

describe("normalizeHref", () => {
  it("keeps a full address as it is", () => {
    expect(normalizeHref("https://example.com/a")).toBe("https://example.com/a");
    expect(normalizeHref("http://intranet/report")).toBe("http://intranet/report");
  });

  it("assumes https for a bare host", () => {
    expect(normalizeHref("example.com/보고서")).toBe("https://example.com/보고서");
  });

  it("turns an address that is an email into a mailto link", () => {
    expect(normalizeHref("hong@example.com")).toBe("mailto:hong@example.com");
  });

  it("keeps a mailto link written out in full", () => {
    expect(normalizeHref("mailto:hong@example.com")).toBe("mailto:hong@example.com");
  });

  it("refuses a scheme that would run code", () => {
    expect(normalizeHref("javascript:alert(1)")).toBe("");
    expect(normalizeHref("data:text/html,<script>")).toBe("");
    expect(normalizeHref("  JavaScript:alert(1)  ")).toBe("");
  });

  it("treats an empty value as removing the link", () => {
    expect(normalizeHref("   ")).toBe("");
  });
});
