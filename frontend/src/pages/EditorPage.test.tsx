import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  RouterProvider,
  createMemoryRouter,
  useRouteError,
} from "react-router-dom";

vi.mock("../contexts/AuthContext", () => ({
  useAuth: () => ({
    user: {
      id: "u1",
      email: "user@example.com",
      displayName: "홍길동",
      role: "USER",
    },
    build: null,
    logout: vi.fn(),
  }),
}));

// The real hook opens a socket and IndexedDB. This one keeps the shape — a
// fresh Y.Doc per document, an awareness for the caret extension — and
// reports itself synced at once with this client chosen to seed, so the
// stored content goes into the editor the way it does on a first open.
vi.mock("../hooks/useCollaboration", async () => {
  const { useMemo } = await import("react");
  const Y = await import("yjs");
  const { Awareness } = await import("y-protocols/awareness");
  return {
    useCollaboration: (documentId: string) => {
      const ydoc = useMemo(() => new Y.Doc(), [documentId]);
      const awareness = useMemo(() => new Awareness(ydoc), [ydoc]);
      const provider = useMemo(() => ({ awareness }), [awareness]);
      return {
        ydoc,
        awareness,
        provider,
        status: "connected",
        syncedAt: 1,
        users: [],
        maySeed: true,
      };
    },
  };
});

import { EditorPage } from "./EditorPage";

const documents: Record<string, object> = {
  a: {
    id: "a",
    workspaceId: "w1",
    title: "첫째 문서",
    permission: "OWNER",
    revision: 1,
    crdtGeneration: 1,
    headingNumbering: "decimal",
    content: {
      type: "doc",
      content: [
        { type: "heading", attrs: { level: 1 }, content: [{ type: "text", text: "개요" }] },
        { type: "paragraph", content: [{ type: "text", text: "본문" }] },
      ],
    },
  },
  b: {
    id: "b",
    workspaceId: "w1",
    title: "둘째 문서",
    permission: "OWNER",
    revision: 1,
    crdtGeneration: 1,
    headingNumbering: "none",
    content: { type: "doc", content: [{ type: "paragraph" }] },
  },
};

const capabilities = {
  workflowEnabled: false,
  aiEnabled: false,
  pdfExport: false,
  docxExport: false,
  presentations: false,
  maxAiTokens: 0,
};

function json(data: unknown, status = 200) {
  return new Response(JSON.stringify({ data }), {
    status,
    headers: { "content-type": "application/json" },
  });
}

function fakeFetch(input: RequestInfo | URL) {
  const path = new URL(String(input), "http://muni.test").pathname;
  const document = path.match(/^\/api\/v1\/documents\/([^/]+)$/);
  if (document?.[1]) {
    const found = documents[document[1]];
    return Promise.resolve(
      found ? json(found) : json({ code: "NOT_FOUND", message: "없음" }, 404),
    );
  }
  if (path === "/api/v1/system/capabilities") return Promise.resolve(json(capabilities));
  if (path === "/api/v1/notifications") return Promise.resolve(json({ items: [], unread: 0 }));
  if (path.endsWith("/tags")) return Promise.resolve(json({ tags: [] }));
  return Promise.resolve(json([]));
}

// The page has no boundary of its own; the app's shows "문제가 생겼습니다".
// A crash in one of the page's effects is caught by the route and shown
// here, so the test reads the message instead of timing out on a title
// that never comes.
function Failed() {
  const error = useRouteError() as Error;
  return <p>문제가 생겼습니다: {error.message}</p>;
}

function open(path: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const router = createMemoryRouter(
    [
      {
        path: "/docs/:documentId",
        element: <EditorPage />,
        errorElement: <Failed />,
      },
    ],
    { initialEntries: [path] },
  );
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return router;
}

async function editorShowing(title: string) {
  const title_ = screen.findByLabelText("문서 제목", {}, { timeout: 3000 });
  const failed = screen.findByText(/문제가 생겼습니다/, {}, { timeout: 3000 });
  // Whichever shows first decides; the other keeps looking until it gives up,
  // and its giving up is not a failure of anything.
  title_.catch(() => undefined);
  failed.catch(() => undefined);
  const field = await Promise.race([
    title_,
    failed.then((element) => {
      throw new Error(element.textContent ?? "");
    }),
  ]);
  expect((field as HTMLInputElement).value).toBe(title);
  const editor = document.querySelector(".tiptap");
  expect(editor).not.toBeNull();
  return editor as HTMLElement;
}

beforeEach(() => {
  vi.stubGlobal(
    "matchMedia",
    vi.fn((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  );
  vi.stubGlobal("fetch", vi.fn(fakeFetch));
  vi.spyOn(console, "error").mockImplementation(() => {});
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("EditorPage", () => {
  it("opens a document arrived at directly and numbers its headings", async () => {
    open("/docs/a");
    const editor = await editorShowing("첫째 문서");
    // The scheme reaches the editor through an effect; the heading it
    // numbers is the one the document carries.
    expect(editor.querySelector(".muni-heading-number")?.textContent).toBe("1. ");
    expect(screen.queryByText(/문제가 생겼습니다/)).toBeNull();
  });

  it("moves to another document without touching the editor it replaced", async () => {
    const router = open("/docs/a");
    await editorShowing("첫째 문서");
    // Same page, new document: useEditor destroys the old editor in the very
    // commit whose effects still hold it, and the scheme changes with the
    // document — the scheme effect used to reach a destroyed editor here.
    await act(async () => {
      await router.navigate("/docs/b");
    });
    const editor = await editorShowing("둘째 문서");
    expect(screen.queryByText(/문제가 생겼습니다/)).toBeNull();
    expect(editor.querySelector(".muni-heading-number")).toBeNull();
  });
});
