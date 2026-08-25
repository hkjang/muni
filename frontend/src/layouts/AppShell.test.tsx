import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { User } from "../types";

const useAuth = vi.fn();
vi.mock("../contexts/AuthContext", () => ({ useAuth: () => useAuth() }));

import { AppShell } from "./AppShell";

function renderShell(role: User["role"]) {
  useAuth.mockReturnValue({
    user: {
      id: "u1",
      email: "user@example.com",
      displayName: "홍길동",
      role,
    },
    build: null,
    logout: vi.fn(),
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <AppShell />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.stubGlobal(
    "matchMedia",
    vi.fn((query: string) => ({
      matches: true,
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  );
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify({ data: [] }), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
    ),
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("AppShell", () => {
  it("offers an administrator a way into service admin", async () => {
    renderShell("ADMIN");
    await screen.findByLabelText("주 메뉴");
    // The profile menu is closed, so this is the entry in the sidebar itself.
    expect(screen.getByText("서비스 관리")).toBeDefined();
  });

  it("hides it from everyone else", async () => {
    renderShell("USER");
    await screen.findByLabelText("주 메뉴");
    expect(screen.queryByText("서비스 관리")).toBeNull();
  });
});
