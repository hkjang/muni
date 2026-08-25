import { describe, expect, it } from "vitest";
import {
  groupInsertCommands,
  insertCommands,
  matchInsertCommands,
  readSlashQuery,
  todayLabel,
} from "./slashCommands";

describe("readSlashQuery", () => {
  it("opens on a slash that starts the line", () => {
    expect(readSlashQuery("/")).toEqual({ query: "", length: 1 });
  });

  it("opens on a slash after a space", () => {
    expect(readSlashQuery("본문 /제목")).toEqual({ query: "제목", length: 3 });
  });

  it("does not open inside a date or a path", () => {
    expect(readSlashQuery("2026/08")).toBeNull();
    expect(readSlashQuery("docs/releases")).toBeNull();
  });

  it("closes when a second slash is typed", () => {
    expect(readSlashQuery("/제목/")).toBeNull();
  });

  it("gives up on a query nobody would type", () => {
    expect(readSlashQuery("/" + "가".repeat(30))).toBeNull();
  });

  it("is closed when there is no slash at all", () => {
    expect(readSlashQuery("보통 문장입니다")).toBeNull();
  });
});

describe("matchInsertCommands", () => {
  it("lists everything before anything is typed", () => {
    expect(matchInsertCommands(insertCommands, "")).toHaveLength(
      insertCommands.length,
    );
  });

  it("puts the exact label first", () => {
    const found = matchInsertCommands(insertCommands, "표");
    expect(found[0]?.id).toBe("table");
  });

  it("finds a command by its English name", () => {
    expect(matchInsertCommands(insertCommands, "table")[0]?.id).toBe("table");
    expect(matchInsertCommands(insertCommands, "page")[0]?.id).toBe("pageBreak");
  });

  it("matches a Korean label typed without its space", () => {
    expect(matchInsertCommands(insertCommands, "제목1")[0]?.id).toBe("h1");
  });

  it("finds nothing for a query that matches nothing", () => {
    expect(matchInsertCommands(insertCommands, "zzzz")).toEqual([]);
  });
});

describe("groupInsertCommands", () => {
  it("keeps 서식 before 삽입", () => {
    expect(groupInsertCommands(insertCommands).map((entry) => entry.group)).toEqual([
      "서식",
      "삽입",
    ]);
  });

  it("leaves out a group with nothing in it", () => {
    const onlyInsert = insertCommands.filter((command) => command.group === "삽입");
    expect(groupInsertCommands(onlyInsert).map((entry) => entry.group)).toEqual([
      "삽입",
    ]);
  });
});

describe("todayLabel", () => {
  it("writes the date the way a Korean document does", () => {
    expect(todayLabel(new Date(2026, 7, 26))).toBe("2026. 08. 26.");
  });
});
