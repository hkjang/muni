/**
 * Where the reader was in each document.
 *
 * Reopening a long document at the top means scrolling back to where you were
 * every single time. Google Docs returns you to the last edit, and for a
 * report that runs to twenty pages that is the difference between picking work
 * back up and hunting for it.
 *
 * This is a per-browser convenience, not document state — it never leaves the
 * machine it was written on.
 */

const storageKey = "muni:editor:positions";

/** How many documents to remember before dropping the oldest. */
const capacity = 50;

type Positions = Record<string, number>;

type Store = Pick<Storage, "getItem" | "setItem">;

export function rememberPosition(
  store: Store,
  documentId: string,
  position: number,
): void {
  if (!documentId || position < 1) return;
  const current = readAll(store);
  delete current[documentId];
  const entries = Object.entries(current);
  // Insertion order is the recency order, so trimming the front drops the
  // documents that have not been opened in the longest time.
  const kept = entries.slice(Math.max(0, entries.length - (capacity - 1)));
  const next: Positions = Object.fromEntries(kept);
  next[documentId] = Math.round(position);
  write(store, next);
}

export function recallPosition(store: Store, documentId: string): number | null {
  const value = readAll(store)[documentId];
  return typeof value === "number" && value > 0 ? value : null;
}

function readAll(store: Store): Positions {
  try {
    const raw = store.getItem(storageKey);
    if (!raw) return {};
    const parsed: unknown = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object") return {};
    const out: Positions = {};
    for (const [key, value] of Object.entries(parsed as Record<string, unknown>))
      if (typeof value === "number" && Number.isFinite(value) && value > 0)
        out[key] = value;
    return out;
  } catch {
    // Storage that is unavailable or holds something else just means the
    // document opens at the top.
    return {};
  }
}

function write(store: Store, value: Positions): void {
  try {
    store.setItem(storageKey, JSON.stringify(value));
  } catch {
    /* A full or disabled store simply forgets. */
  }
}
