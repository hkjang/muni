import * as Y from "yjs";

/**
 * serverStateVector describes what the server already holds, so the client can
 * send only the part it is missing. Merging the updates first is cheaper than
 * replaying them into a throwaway document.
 */
export function serverStateVector(updates: Uint8Array[]): Uint8Array {
  if (updates.length === 0) return Y.encodeStateVector(new Y.Doc());
  const merged = updates.length === 1 ? updates[0]! : Y.mergeUpdates(updates);
  return Y.encodeStateVectorFromUpdate(merged);
}

/**
 * isEmptyUpdate recognises the update Yjs produces when there is nothing to
 * send: an empty struct list followed by an empty delete set.
 */
export function isEmptyUpdate(update: Uint8Array): boolean {
  return update.byteLength <= 2;
}
