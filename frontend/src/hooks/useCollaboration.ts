import { useEffect, useMemo, useState } from "react";
import * as Y from "yjs";
import {
  Awareness,
  applyAwarenessUpdate,
  encodeAwarenessUpdate,
} from "y-protocols/awareness";
import { IndexeddbPersistence } from "y-indexeddb";
import type { User } from "../types";

export type CollaborationStatus = "offline" | "connecting" | "synced";
export type AwarenessUser = {
  clientId: number;
  id?: string;
  name?: string;
  color?: string;
};

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

export function useCollaboration(documentId: string, user: User | null) {
  const ydoc = useMemo(() => new Y.Doc(), [documentId]);
  const awareness = useMemo(() => new Awareness(ydoc), [ydoc]);
  const provider = useMemo(() => ({ awareness }), [awareness]);
  const [status, setStatus] = useState<CollaborationStatus>("connecting");
  const [syncedAt, setSyncedAt] = useState(0);
  const [users, setUsers] = useState<AwarenessUser[]>([]);

  useEffect(() => {
    if (!documentId || !user) return;
    let disposed = false;
    let socket: WebSocket | null = null;
    let reconnectTimer: number | undefined;
    let reconnectAttempt = 0;
    const persistence = new IndexeddbPersistence(
      `muni:document:${documentId}`,
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
      if (origin !== "remote") send(0, update);
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
          1,
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
              updates?: string[];
            };
            if (message.type === "sync") {
              for (const encoded of message.updates ?? [])
                Y.applyUpdate(ydoc, decodeBase64(encoded), "remote");
              send(0, Y.encodeStateAsUpdate(ydoc));
              setStatus("synced");
              setSyncedAt(Date.now());
              const clients = Array.from(awareness.getStates().keys());
              send(1, encodeAwarenessUpdate(awareness, clients));
            }
          } catch {
            /* Ignore unknown ephemeral text events. */
          }
          return;
        }
        const payload = new Uint8Array(event.data as ArrayBuffer);
        if (payload.length < 2) return;
        if (payload[0] === 0)
          Y.applyUpdate(ydoc, payload.subarray(1), "remote");
        if (payload[0] === 1)
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
  }, [awareness, documentId, user, ydoc]);

  useEffect(
    () => () => {
      awareness.destroy();
      ydoc.destroy();
    },
    [awareness, ydoc],
  );
  return { ydoc, awareness, provider, status, syncedAt, users };
}
