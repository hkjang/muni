import { describe, expect, it } from "vitest";
import * as Y from "yjs";
import { isEmptyUpdate, serverStateVector } from "./collabSync";

/** Records what a client would send, the way the socket handler does. */
function syncWithServer(serverUpdates: Uint8Array[], client: Y.Doc) {
  for (const update of serverUpdates) Y.applyUpdate(client, update, "remote");
  const missing = Y.encodeStateAsUpdate(client, serverStateVector(serverUpdates));
  return { missing, empty: isEmptyUpdate(missing) };
}

function text(doc: Y.Doc): string {
  return doc.getText("body").toString();
}

/** Rebuilds the document from what the server stored, as a joining client does. */
function replay(updates: Uint8Array[]): Y.Doc {
  const doc = new Y.Doc();
  for (const update of updates) Y.applyUpdate(doc, update, "remote");
  return doc;
}

describe("serverStateVector", () => {
  it("describes an empty server for a document with no history", () => {
    const fresh = new Y.Doc();
    expect(serverStateVector([])).toEqual(Y.encodeStateVector(fresh));
  });

  it("matches the state of a document built from the same updates", () => {
    const author = new Y.Doc();
    const updates: Uint8Array[] = [];
    author.on("update", (update: Uint8Array) => updates.push(update));
    author.getText("body").insert(0, "안녕하세요");
    author.getText("body").insert(5, " 반갑습니다");

    expect(serverStateVector(updates)).toEqual(
      Y.encodeStateVector(replay(updates)),
    );
  });
});

describe("connect handshake", () => {
  it("sends nothing back when the client has no local changes", () => {
    const author = new Y.Doc();
    const updates: Uint8Array[] = [];
    author.on("update", (update: Uint8Array) => updates.push(update));
    author.getText("body").insert(0, "서버에만 있는 내용");

    const { empty } = syncWithServer(updates, new Y.Doc());
    expect(empty).toBe(true);
  });

  it("sends offline edits the server has never seen", () => {
    const author = new Y.Doc();
    const serverUpdates: Uint8Array[] = [];
    author.on("update", (update: Uint8Array) => serverUpdates.push(update));
    author.getText("body").insert(0, "서버 내용");

    // A client that edited while offline, its work restored from IndexedDB.
    const offline = new Y.Doc();
    offline.getText("body").insert(0, "오프라인 내용");

    const { missing, empty } = syncWithServer(serverUpdates, offline);
    expect(empty).toBe(false);

    const server = replay(serverUpdates);
    Y.applyUpdate(server, missing, "remote");
    expect(text(server)).toBe(text(offline));
    expect(text(server)).toContain("서버 내용");
    expect(text(server)).toContain("오프라인 내용");
  });
});

describe("compaction", () => {
  it("replaces the history with one state that rebuilds the same document", () => {
    const author = new Y.Doc();
    const updates: Uint8Array[] = [];
    author.on("update", (update: Uint8Array) => updates.push(update));
    const body = author.getText("body");
    for (let index = 0; index < 50; index += 1) body.insert(body.length, `문장 ${index}. `);
    // Deletions must survive compaction too: the delete set is part of the state.
    body.delete(0, 5);
    expect(updates.length).toBeGreaterThan(50);

    // What the compacting client sends after applying everything.
    const compactor = replay(updates);
    const snapshot = Y.encodeStateAsUpdate(compactor);

    // A later client receives only the snapshot.
    expect(text(replay([snapshot]))).toBe(text(author));
    expect(snapshot.byteLength).toBeLessThan(
      updates.reduce((total, update) => total + update.byteLength, 0),
    );
  });

  it("keeps edits that land after the snapshot was taken", () => {
    const author = new Y.Doc();
    const updates: Uint8Array[] = [];
    author.on("update", (update: Uint8Array) => updates.push(update));
    author.getText("body").insert(0, "처음 내용 ");

    const snapshot = Y.encodeStateAsUpdate(replay(updates));
    const covered = updates.length;

    // Another client keeps typing while the snapshot is in flight; those
    // updates have a higher seq and are not deleted.
    author.getText("body").insert(author.getText("body").length, "나중 내용");
    const tail = updates.slice(covered);
    expect(tail.length).toBeGreaterThan(0);

    expect(text(replay([snapshot, ...tail]))).toBe(text(author));
  });

  it("is unharmed by replaying an update the snapshot already contains", () => {
    const author = new Y.Doc();
    const updates: Uint8Array[] = [];
    author.on("update", (update: Uint8Array) => updates.push(update));
    author.getText("body").insert(0, "내용");

    const snapshot = Y.encodeStateAsUpdate(replay(updates));
    // The server deletes updates up to base_seq only; a concurrent write can
    // leave an already-covered update behind. Yjs must treat it as a no-op.
    expect(text(replay([snapshot, ...updates]))).toBe(text(author));
  });

  it("survives concurrent edits from two clients", () => {
    const first = new Y.Doc();
    const second = new Y.Doc();
    const updates: Uint8Array[] = [];
    first.on("update", (update: Uint8Array) => updates.push(update));
    second.on("update", (update: Uint8Array) => updates.push(update));

    first.getText("body").insert(0, "가나다");
    second.getText("body").insert(0, "라마바");

    const merged = replay(updates);
    const snapshot = Y.encodeStateAsUpdate(merged);
    const rebuilt = replay([snapshot]);
    expect(text(rebuilt)).toBe(text(merged));
    expect(text(rebuilt)).toContain("가나다");
    expect(text(rebuilt)).toContain("라마바");
  });
});
