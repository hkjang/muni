import { describe, expect, it } from "vitest";
import { splitReasoning } from "./reasoning";

describe("splitReasoning", () => {
  // What the Qwen family sends when reasoning was never asked for.
  it("drops working that ends with a closing tag it never opened", () => {
    const raw =
      "사용자가 요약을 원한다. 문서는 세 항목이다.\n</think>\n요약: 세 항목입니다.";
    expect(splitReasoning(raw)).toEqual({
      text: "요약: 세 항목입니다.",
      thinking: false,
      reasoning: "사용자가 요약을 원한다. 문서는 세 항목이다.",
    });
  });

  it("removes a complete block", () => {
    expect(splitReasoning("<think>먼저 생각</think>답변").text).toBe("답변");
  });

  it("reports that the model has not started answering yet", () => {
    expect(splitReasoning("<think>아직 생각 중")).toEqual({
      text: "",
      thinking: true,
      reasoning: "아직 생각 중",
    });
  });

  it("leaves an ordinary answer alone", () => {
    expect(splitReasoning("그냥 답변입니다.")).toEqual({
      text: "그냥 답변입니다.",
      thinking: false,
      reasoning: "",
    });
  });

  it("keeps only what follows the last closing tag", () => {
    expect(splitReasoning("생각1</think>생각2</think>답변").text).toBe("답변");
  });

  it("ignores the case of the tag", () => {
    expect(splitReasoning("생각\n</THINK>\n답변").text).toBe("답변");
  });

  // The reasoning arrives before the answer, so a partial stream must never
  // leave the working in place once the boundary appears.
  it("cleans up as the stream advances", () => {
    const chunks = [
      "사용자가",
      " 요약을 원한다.",
      "\n</think>\n",
      "요약:",
      " 세 항목",
    ];
    let raw = "";
    const seen: string[] = [];
    for (const chunk of chunks) {
      raw += chunk;
      seen.push(splitReasoning(raw).text);
    }
    expect(seen[seen.length - 1]).toBe("요약: 세 항목");
    // Once the boundary is passed the working is gone for good.
    expect(seen[seen.length - 1]).not.toContain("사용자가");
  });

  // The panel shows the working while it arrives and folds it away after.
  it("keeps the working so it can be shown", () => {
    const split = splitReasoning("<think>세 항목을 세어본다</think>답변");
    expect(split.reasoning).toBe("세 항목을 세어본다");
    expect(split.text).toBe("답변");
  });

  it("handles an empty stream", () => {
    expect(splitReasoning("")).toEqual({
      text: "",
      thinking: false,
      reasoning: "",
    });
  });
});
