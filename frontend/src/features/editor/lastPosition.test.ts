import { beforeEach, describe, expect, it } from "vitest";
import { recallPosition, rememberPosition } from "./lastPosition";

function memoryStore() {
  const data = new Map<string, string>();
  return {
    getItem: (key: string) => data.get(key) ?? null,
    setItem: (key: string, value: string) => void data.set(key, value),
    size: () => data.size,
    raw: () => data,
  };
}

let store: ReturnType<typeof memoryStore>;
beforeEach(() => {
  store = memoryStore();
});

describe("last position", () => {
  it("brings the reader back to where they were", () => {
    rememberPosition(store, "doc-1", 420);
    expect(recallPosition(store, "doc-1")).toBe(420);
  });

  it("knows nothing about a document not opened before", () => {
    expect(recallPosition(store, "doc-2")).toBeNull();
  });

  it("keeps documents apart", () => {
    rememberPosition(store, "doc-1", 10);
    rememberPosition(store, "doc-2", 99);
    expect(recallPosition(store, "doc-1")).toBe(10);
    expect(recallPosition(store, "doc-2")).toBe(99);
  });

  it("replaces the position rather than accumulating them", () => {
    rememberPosition(store, "doc-1", 10);
    rememberPosition(store, "doc-1", 40);
    expect(recallPosition(store, "doc-1")).toBe(40);
  });

  it("forgets the documents opened longest ago", () => {
    for (let index = 0; index < 60; index += 1)
      rememberPosition(store, `doc-${index}`, index + 1);
    expect(recallPosition(store, "doc-0")).toBeNull();
    expect(recallPosition(store, "doc-59")).toBe(60);
  });

  it("ignores a position at the very start", () => {
    rememberPosition(store, "doc-1", 0);
    expect(recallPosition(store, "doc-1")).toBeNull();
  });

  it("survives storage holding something else entirely", () => {
    store.setItem("muni:editor:positions", "not json");
    expect(recallPosition(store, "doc-1")).toBeNull();
    rememberPosition(store, "doc-1", 5);
    expect(recallPosition(store, "doc-1")).toBe(5);
  });

  it("works when storage throws", () => {
    const hostile = {
      getItem: () => {
        throw new Error("blocked");
      },
      setItem: () => {
        throw new Error("blocked");
      },
    };
    expect(() => rememberPosition(hostile, "doc-1", 5)).not.toThrow();
    expect(recallPosition(hostile, "doc-1")).toBeNull();
  });
});
