export type QuickCommand = {
  id: string;
  /** What the row says. */
  label: string;
  /** The line under it: a workspace name, an owner, a snippet. */
  detail?: string;
  group: "문서" | "워크스페이스" | "이동" | "동작";
  /** Where to go, or what to do. */
  to?: string;
  run?: () => void;
  /** Extra words the query may match, such as a slug or an owner's name. */
  keywords?: string;
};

/**
 * score ranks a command against what has been typed.
 *
 * A quick switcher is only quick if the thing you want is first, so a title
 * that starts with the query beats one that merely contains it, and a match in
 * the title beats a match in the detail. Zero means the row does not belong in
 * the list at all.
 */
export function score(command: QuickCommand, query: string): number {
  const needle = query.trim().toLowerCase();
  if (!needle) return 1;

  const label = command.label.toLowerCase();
  const detail = (command.detail ?? "").toLowerCase();
  const keywords = (command.keywords ?? "").toLowerCase();

  if (label === needle) return 100;
  if (label.startsWith(needle)) return 80;
  if (wordStartsWith(label, needle)) return 60;
  if (label.includes(needle)) return 40;
  if (keywords.includes(needle)) return 25;
  if (detail.includes(needle)) return 15;
  // Letters in order but not adjacent — how someone types "추계" for
  // "추진 계획".
  if (subsequence(label, needle)) return 10;
  return 0;
}

function wordStartsWith(value: string, needle: string): boolean {
  return value.split(/[\s·/,()[\]-]+/).some((word) => word.startsWith(needle));
}

function subsequence(value: string, needle: string): boolean {
  let index = 0;
  for (const character of value) {
    if (character === needle[index]) index += 1;
    if (index === needle.length) return true;
  }
  return needle.length === 0;
}

/** rank keeps the best matches, in order, and drops the rest. */
export function rank(commands: QuickCommand[], query: string, limit = 12): QuickCommand[] {
  return commands
    .map((command) => ({ command, value: score(command, query) }))
    .filter((entry) => entry.value > 0)
    .sort((left, right) => right.value - left.value)
    .slice(0, limit)
    .map((entry) => entry.command);
}

/** groupOrder is the order the sections appear in, whatever the scores say. */
export const groupOrder: QuickCommand["group"][] = [
  "문서",
  "워크스페이스",
  "이동",
  "동작",
];

export function groupCommands(commands: QuickCommand[]) {
  const groups = new Map<QuickCommand["group"], QuickCommand[]>();
  for (const command of commands) {
    const existing = groups.get(command.group) ?? [];
    existing.push(command);
    groups.set(command.group, existing);
  }
  return groupOrder
    .filter((group) => groups.has(group))
    .map((group) => ({ group, commands: groups.get(group)! }));
}
