import { describe, expect, it } from "vitest";
import { folderPaths } from "./folderTree";
import type { Folder } from "../../types";

function folder(id: string, name: string, parentId?: string): Folder {
  return { id, name, parentId, workspaceId: "w" } as Folder;
}

describe("folderPaths", () => {
  it("shows where a folder sits, not just its name", () => {
    const paths = folderPaths([
      folder("a", "2026"),
      folder("b", "상반기", "a"),
      folder("c", "1분기", "b"),
    ]);
    expect(paths.map((entry) => entry.path)).toEqual([
      "2026",
      "2026 / 상반기",
      "2026 / 상반기 / 1분기",
    ]);
  });

  it("tells two folders of the same name apart", () => {
    const paths = folderPaths([
      folder("a", "2025"),
      folder("b", "2026"),
      folder("c", "상반기", "a"),
      folder("d", "상반기", "b"),
    ]);
    expect(paths.map((entry) => entry.path)).toContain("2025 / 상반기");
    expect(paths.map((entry) => entry.path)).toContain("2026 / 상반기");
  });

  it("keeps a folder whose parent is not in the list", () => {
    const paths = folderPaths([folder("orphan", "떨어진 폴더", "gone")]);
    expect(paths).toHaveLength(1);
    expect(paths[0]?.path).toBe("떨어진 폴더");
  });

  it("does not hang on a cycle", () => {
    const paths = folderPaths([folder("a", "가", "b"), folder("b", "나", "a")]);
    expect(paths.length).toBeGreaterThan(0);
  });

  it("has nothing to show for a workspace without folders", () => {
    expect(folderPaths([])).toEqual([]);
  });

  it("sorts folders at each level by name", () => {
    const paths = folderPaths([folder("b", "나"), folder("a", "가")]);
    expect(paths.map((entry) => entry.path)).toEqual(["가", "나"]);
  });
});
