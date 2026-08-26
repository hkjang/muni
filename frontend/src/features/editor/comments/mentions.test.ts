import { describe, expect, it } from "vitest";
import { applyMention, readMention } from "./mentions";

describe("readMention", () => {
  it("opens on an @ that starts a word", () => {
    expect(readMention("@hon", 4)).toEqual({ start: 0, query: "hon" });
    // Indices are counted rather than written out: a Korean prefix makes them
    // easy to get wrong by hand and the test would then be testing itself.
    const value = "확인 부탁드립니다 @hon";
    expect(readMention(value, value.length)).toEqual({
      start: value.indexOf("@"),
      query: "hon",
    });
  });

  it("opens on a bare @ before anything is typed", () => {
    expect(readMention("@", 1)).toEqual({ start: 0, query: "" });
  });

  it("does not open inside an email address", () => {
    expect(readMention("hong@example.com", 16)).toBeNull();
  });

  it("closes once a space is typed", () => {
    expect(readMention("@hong 님 확인 부탁", 15)).toBeNull();
  });

  it("closes on a second @", () => {
    expect(readMention("@hong@", 6)).toBeNull();
  });

  it("gives up on something nobody would type as a name", () => {
    expect(readMention("@" + "a".repeat(40), 41)).toBeNull();
  });

  it("is closed when there is no @ at all", () => {
    expect(readMention("그냥 댓글입니다", 8)).toBeNull();
  });

  it("reads the mention the caret is in, not a later one", () => {
    const value = "@hon 님과 @kim";
    expect(readMention(value, 4)).toEqual({ start: 0, query: "hon" });
  });
});

describe("applyMention", () => {
  it("writes the name in and leaves a space after it", () => {
    const mention = readMention("@hon", 4)!;
    expect(applyMention("@hon", mention, "hong")).toEqual({
      value: "@hong ",
      caret: 6,
    });
  });

  it("keeps what came after the mention", () => {
    const value = "@hon 님 확인 부탁드립니다";
    const mention = readMention(value, 4)!;
    const result = applyMention(value, mention, "hong");
    expect(result.value).toBe("@hong 님 확인 부탁드립니다");
    // The caret lands right after the name, before the space that was already
    // there — not back at the @.
    expect(result.caret).toBe("@hong".length);
    expect(result.value.slice(result.caret)).toBe(" 님 확인 부탁드립니다");
  });

  it("does not double the space when one is already there", () => {
    const value = "@hon 님";
    const mention = readMention(value, 4)!;
    expect(applyMention(value, mention, "hong").value).toBe("@hong 님");
  });

  it("works when the mention is in the middle of a sentence", () => {
    const value = "오늘 @ki 확인해 주세요";
    // The caret has to be inside the mention; past the space it is closed.
    const caret = value.indexOf("@") + "@ki".length;
    const mention = readMention(value, caret)!;
    expect(applyMention(value, mention, "kim").value).toBe(
      "오늘 @kim 확인해 주세요",
    );
  });
});
