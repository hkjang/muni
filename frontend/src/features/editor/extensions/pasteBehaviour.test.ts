import { describe, expect, it } from "vitest";
import { isPastableURL } from "./pasteBehaviour";

describe("isPastableURL", () => {
  it("recognises a web address", () => {
    expect(isPastableURL("https://example.com/보고서")).toBe(true);
    expect(isPastableURL("http://intranet/a")).toBe(true);
  });

  it("recognises an email link", () => {
    expect(isPastableURL("mailto:hong@example.com")).toBe(true);
  });

  it("ignores surrounding whitespace", () => {
    expect(isPastableURL("  https://example.com \n")).toBe(true);
  });

  it("is not a link when it is a sentence that mentions one", () => {
    expect(isPastableURL("자세한 내용은 https://example.com 을 보세요")).toBe(false);
  });

  it("is not a link when it is ordinary text", () => {
    expect(isPastableURL("회의 결과")).toBe(false);
    expect(isPastableURL("")).toBe(false);
  });

  it("refuses a scheme that would run code", () => {
    expect(isPastableURL("javascript:alert(1)")).toBe(false);
    expect(isPastableURL("data:text/html,<script>")).toBe(false);
    expect(isPastableURL("file:///etc/passwd")).toBe(false);
  });

  it("refuses something far too long to be an address anyone pasted", () => {
    expect(isPastableURL("https://example.com/" + "a".repeat(3000))).toBe(false);
  });
});
