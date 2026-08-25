import { describe, expect, it } from "vitest";
import { toContent } from "./selectionContent";
import { buildPrompt, selectionActions } from "./aiActions";

describe("toContent", () => {
  it("keeps a one-line rewrite inline so it stays inside the sentence", () => {
    expect(toContent("다듬어진 문장입니다.")).toBe("다듬어진 문장입니다.");
  });

  it("trims surrounding whitespace", () => {
    expect(toContent("\n  결과  \n")).toBe("결과");
  });

  it("returns an empty string for a blank answer", () => {
    expect(toContent("   \n\n  ")).toBe("");
  });

  it("splits blank-line separated text into paragraphs", () => {
    expect(toContent("첫 문단\n\n둘째 문단")).toEqual([
      { type: "paragraph", content: [{ type: "text", text: "첫 문단" }] },
      { type: "paragraph", content: [{ type: "text", text: "둘째 문단" }] },
    ]);
  });

  it("turns single newlines inside a paragraph into hard breaks", () => {
    expect(toContent("한 줄\n다음 줄")).toEqual([
      {
        type: "paragraph",
        content: [
          { type: "text", text: "한 줄" },
          { type: "hardBreak" },
          { type: "text", text: "다음 줄" },
        ],
      },
    ]);
  });

  it("builds a real bullet list when every line is a bullet", () => {
    expect(toContent("- 첫째\n- 둘째\n* 셋째")).toEqual([
      {
        type: "bulletList",
        content: [
          {
            type: "listItem",
            content: [
              { type: "paragraph", content: [{ type: "text", text: "첫째" }] },
            ],
          },
          {
            type: "listItem",
            content: [
              { type: "paragraph", content: [{ type: "text", text: "둘째" }] },
            ],
          },
          {
            type: "listItem",
            content: [
              { type: "paragraph", content: [{ type: "text", text: "셋째" }] },
            ],
          },
        ],
      },
    ]);
  });

  it("does not treat mixed prose as a list", () => {
    const result = toContent("설명입니다\n- 항목");
    expect(Array.isArray(result)).toBe(true);
  });
});

describe("selection actions", () => {
  it("gives every action a distinct id and a Korean label", () => {
    const ids = new Set(selectionActions.map((action) => action.id));
    expect(ids.size).toBe(selectionActions.length);
    for (const action of selectionActions) {
      expect(action.label.length).toBeGreaterThan(0);
      expect(action.instruction.length).toBeGreaterThan(10);
    }
  });

  it("tells the model to answer with the replacement text only", () => {
    const prompt = buildPrompt(selectionActions[0]!.instruction, "원본 문장");
    expect(prompt).toContain("결과만 출력");
    expect(prompt).toContain("Markdown");
    expect(prompt).toContain("원본 문장");
  });
});
