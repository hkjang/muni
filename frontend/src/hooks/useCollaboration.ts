import { useEffect, useMemo, useState } from "react";
import * as Y from "yjs";
import {
  Awareness,
  applyAwarenessUpdate,
  encodeAwarenessUpdate,
} from "y-protocols/awareness";
import { IndexeddbPersistence } from "y-indexeddb";
import type { User } from "../types";
import { isEmptyUpdate, serverStateVector } from "./collabSync";

export type CollaborationStatus = "offline" | "connecting" | "synced";
export type AwarenessUser = {
  clientId: number;
  id?: string;
  name?: string;
  color?: string;
};

/** Binary frames carry a one byte channel prefix. */
const CHANNEL_UPDATE = 0;
const CHANNEL_AWARENESS = 1;
const CHANNEL_SNAPSHOT = 2;

function decodeBase64(value: string) {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1)
    bytes[index] = binary.charCodeAt(index);
  return bytes;
}

function userColor(id: string) {
  const colors = [
    "#5151c6",
    "#167d70",
    "#b05d22",
    "#a13e78",
    "#3977a8",
    "#6b7132",
    "#9254b0",
  ];
  let hash = 0;
  for (const char of id) hash = ((hash << 5) - hash + char.charCodeAt(0)) | 0;
  return colors[Math.abs(hash) % colors.length] ?? colors[0];
}

/**
 * offlineName is where the browser keeps its copy of the shared document.
 *
 * The generation is part of the name on purpose. When a version is restored the
 * server replaces the shared state and counts the generation up; a browser that
 * still had the old state in IndexedDB would otherwise reconnect, find the
 * server empty, and helpfully push the pre-restore document straight back.
 */
function offlineName(documentId: string, generation: number) {
  return `muni:document:${documentId}:g${generation}`;
}

/** Old generations are dead weight, and one of them is the state we replaced. */
function dropOldOfflineCopies(documentId: string, generation: number) {
  const keep = offlineName(documentId, generation);
  const prefix = `muni:document:${documentId}`;
  const remove = (name: string) => {
    if (name === keep) return;
    try {
      indexedDB.deleteDatabase(name);
    } catch {
      /* A browser that refuses to enumerate or delete simply keeps them. */
    }
  };
  try {
    const list = indexedDB.databases?.();
    if (list) {
      void list
        .then((entries) => {
          for (const entry of entries) {
            const name = entry.name ?? "";
            if (name === prefix || name.startsWith(prefix + ":")) remove(name);
          }
        })
        .catch(() => undefined);
      return;
    }
  } catch {
    /* Fall through to the blind removal below. */
  }
  // Safari has no databases(); delete the unversioned name this used to use
  // and the generations immediately behind us, which covers a restore.
  remove(prefix);
  for (
    let previous = generation - 1;
    previous >= 1 && previous > generation - 8;
    previous -= 1
  )
    remove(offlineName(documentId, previous));
}

export function useCollaboration(
  documentId: string,
  user: User | null,
  generation = 0,
) {
  const ydoc = useMemo(() => new Y.Doc(), [documentId]);
  const awareness = useMemo(() => new Awareness(ydoc), [ydoc]);
  const provider = useMemo(() => ({ awareness }), [awareness]);
  const [status, setStatus] = useState<CollaborationStatus>("connecting");
  const [syncedAt, setSyncedAt] = useState(0);
  // The server picks one client to write the starting content into an empty
  // document. A reader who cannot write seeds its own copy for display only.
  const [maySeed, setMaySeed] = useState(false);
  const [users, setUsers] = useState<AwarenessUser[]>([]);

  useEffect(() => {
    // Waiting for the generation costs one render and keeps the offline copy
    // from being opened under a name that is about to change.
    if (!documentId || !user || generation < 1) return;
    let disposed = false;
    let socket: WebSocket | null = null;
    let reconnectTimer: number | undefined;
    let reconnectAttempt = 0;
    dropOldOfflineCopies(documentId, generation);
    const persistence = new IndexeddbPersistence(
      offlineName(documentId, generation),
      ydoc,
    );

    const send = (channel: number, value: Uint8Array) => {
      if (socket?.readyState !== WebSocket.OPEN) return;
      const payload = new Uint8Array(value.length + 1);
      payload[0] = channel;
      payload.set(value, 1);
      socket.send(payload);
    };
    const onDocumentUpdate = (update: Uint8Array, origin: unknown) => {
      if (origin !== "remote") send(CHANNEL_UPDATE, update);
    };
    const onAwarenessUpdate = (
      {
        added,
        updated,
        removed,
      }: { added: number[]; updated: number[]; removed: number[] },
      origin: unknown,
    ) => {
      const next = Array.from(awareness.getStates().entries()).map(
        ([clientId, state]) => ({
          clientId,
          ...(state.user as Omit<AwarenessUser, "clientId"> | undefined),
        }),
      );
      setUsers(next);
      if (origin !== "remote")
        send(
          CHANNEL_AWARENESS,
          encodeAwarenessUpdate(awareness, [...added, ...updated, ...removed]),
        );
    };

    ydoc.on("update", onDocumentUpdate);
    awareness.on("update", onAwarenessUpdate);
    awareness.setLocalStateField("user", {
      id: user.id,
      name: user.displayName,
      color: userColor(user.id),
    });

    const connect = () => {
      if (disposed) return;
      setStatus("connecting");
      const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
      socket = new WebSocket(
        `${protocol}//${window.location.host}/api/v1/collab/${documentId}`,
      );
      socket.binaryType = "arraybuffer";
      socket.onopen = () => {
        reconnectAttempt = 0;
      };
      socket.onmessage = (event) => {
        if (typeof event.data === "string") {
          try {
            const message = JSON.parse(event.data) as {
              type?: string;
              generation?: number;
              snapshot?: string;
              updates?: string[];
              compact?: boolean;
              seed?: boolean;
              writeAllowed?: boolean;
            };
            if (message.type === "sync") {
              // A version was restored after this page loaded. Nothing local is
              // worth keeping, and pushing it would undo the restore.
              if (message.generation && message.generation !== generation) {
                socket?.close();
                window.location.reload();
                return;
              }
              const serverUpdates = [
                ...(message.snapshot ? [decodeBase64(message.snapshot)] : []),
                ...(message.updates ?? []).map(decodeBase64),
              ];
              for (const update of serverUpdates)
                Y.applyUpdate(ydoc, update, "remote");

              // Send only what the server is missing — offline edits held in
              // IndexedDB, say — instead of the whole document. Pushing a full
              // state on every connect is what made the update log grow by a
              // document per open.
              const missing = Y.encodeStateAsUpdate(
                ydoc,
                serverStateVector(serverUpdates),
              );
              if (!isEmptyUpdate(missing)) send(CHANNEL_UPDATE, missing);

              // The server asks one writer to fold the history back into a
              // single state when the tail has grown.
              if (message.compact && message.writeAllowed)
                send(CHANNEL_SNAPSHOT, Y.encodeStateAsUpdate(ydoc));

              setMaySeed(Boolean(message.seed) || !message.writeAllowed);
              setStatus("synced");
              setSyncedAt(Date.now());
              const clients = Array.from(awareness.getStates().keys());
              send(
                CHANNEL_AWARENESS,
                encodeAwarenessUpdate(awareness, clients),
              );
            }
          } catch {
            /* Ignore unknown ephemeral text events. */
          }
          return;
        }
        const payload = new Uint8Array(event.data as ArrayBuffer);
        if (payload.length < 2) return;
        if (payload[0] === CHANNEL_UPDATE)
          Y.applyUpdate(ydoc, payload.subarray(1), "remote");
        if (payload[0] === CHANNEL_AWARENESS)
          applyAwarenessUpdate(awareness, payload.subarray(1), "remote");
      };
      socket.onclose = () => {
        if (disposed) return;
        setStatus("offline");
        const wait = Math.min(20_000, 750 * 2 ** reconnectAttempt);
        reconnectAttempt += 1;
        reconnectTimer = window.setTimeout(connect, wait);
      };
      socket.onerror = () => socket?.close();
    };

    void persistence.whenSynced.then(connect);
    return () => {
      disposed = true;
      if (reconnectTimer) window.clearTimeout(reconnectTimer);
      socket?.close();
      awareness.setLocalState(null);
      awareness.off("update", onAwarenessUpdate);
      ydoc.off("update", onDocumentUpdate);
      persistence.destroy();
    };
  }, [awareness, documentId, generation, user, ydoc]);

  useEffect(
    () => () => {
      awareness.destroy();
      ydoc.destroy();
    },
    [awareness, ydoc],
  );
  return { ydoc, awareness, provider, status, syncedAt, users, maySeed };
}
