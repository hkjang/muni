import type { Folder } from "../../types";

export type FolderPath = { id: string; path: string; depth: number };

/**
 * folderPaths turns a flat list of folders into the paths a person recognises.
 *
 * Folders nest, and a picker that shows only leaf names cannot tell 2026 /
 * 상반기 from 2025 / 상반기. The full path is what makes the choice
 * unambiguous.
 */
export function folderPaths(folders: Folder[]): FolderPath[] {
  const byParent = new Map<string, Folder[]>();
  for (const folder of folders) {
    const key = folder.parentId ?? "";
    byParent.set(key, [...(byParent.get(key) ?? []), folder]);
  }
  for (const list of byParent.values())
    list.sort((left, right) => left.name.localeCompare(right.name, "ko"));

  const out: FolderPath[] = [];
  const seen = new Set<string>();
  const walk = (parent: string, prefix: string, depth: number) => {
    for (const folder of byParent.get(parent) ?? []) {
      // A parent that points into a cycle would otherwise recurse forever.
      if (seen.has(folder.id)) continue;
      seen.add(folder.id);
      const path = prefix ? `${prefix} / ${folder.name}` : folder.name;
      out.push({ id: folder.id, path, depth });
      walk(folder.id, path, depth + 1);
    }
  };
  walk("", "", 0);

  // A folder whose parent is missing from the list still has to be reachable.
  for (const folder of folders) {
    if (!seen.has(folder.id)) {
      seen.add(folder.id);
      out.push({ id: folder.id, path: folder.name, depth: 0 });
    }
  }
  return out;
}
