import { describe, expect, it } from "vitest";
import { groupCommands, rank, score, type QuickCommand } from "./commands";

function doc(label: string, detail = ""): QuickCommand {
  return { id: label, label, detail, group: "문서" };
}

describe("score", () => {
  it("puts an exact title first, then a prefix, then a contained match", () => {
    const exact = score(doc("추진 계획"), "추진 계획");
    const prefix = score(doc("추진 계획서 초안"), "추진 계획");
    const contains = score(doc("2027 추진 계획 검토"), "추진 계획");
    expect(exact).toBeGreaterThan(prefix);
    expect(prefix).toBeGreaterThan(contains);
  });

  it("matches the start of any word, not just the title", () => {
    expect(score(doc("2027 사업 계획"), "계획")).toBeGreaterThan(
      score(doc("사업계획서"), "계획"),
    );
  });

  it("ranks a title match above a detail match", () => {
    expect(score(doc("예산 계획"), "예산")).toBeGreaterThan(
      score(doc("추진 계획", "예산 워크스페이스"), "예산"),
    );
  });

  it("finds letters typed in order", () => {
    expect(score(doc("추진 계획"), "추계")).toBeGreaterThan(0);
    expect(score(doc("추진 계획"), "계추")).toBe(0);
  });

  it("keeps everything when nothing has been typed", () => {
    expect(score(doc("아무 문서"), "  ")).toBe(1);
  });

  it("drops rows that do not match at all", () => {
    expect(score(doc("추진 계획"), "회의록")).toBe(0);
  });
});

describe("rank", () => {
  it("orders by how well each row matches and caps the list", () => {
    const commands = [
      doc("2027 추진 계획 검토"),
      doc("추진 계획"),
      doc("추진 계획서 초안"),
      doc("회의록"),
    ];
    const ranked = rank(commands, "추진 계획", 2);
    expect(ranked).toHaveLength(2);
    expect(ranked[0]!.label).toBe("추진 계획");
    expect(ranked[1]!.label).toBe("추진 계획서 초안");
  });
});

describe("groupCommands", () => {
  it("keeps a stable section order whatever the scores were", () => {
    const grouped = groupCommands([
      { id: "a", label: "동작", group: "동작" },
      { id: "b", label: "문서", group: "문서" },
      { id: "c", label: "이동", group: "이동" },
    ]);
    expect(grouped.map((entry) => entry.group)).toEqual([
      "문서",
      "이동",
      "동작",
    ]);
  });

  it("leaves out sections that have nothing in them", () => {
    const grouped = groupCommands([{ id: "a", label: "문서", group: "문서" }]);
    expect(grouped).toHaveLength(1);
  });
});
