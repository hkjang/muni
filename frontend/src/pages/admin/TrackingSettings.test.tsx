import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { TrackingSettings as TrackingValue, TrackingViolation } from "../../types";
import { MAX_SNIPPET_BYTES, TrackingSettings, snippetBytes } from "./TrackingSettings";

const off: TrackingValue = {
  enabled: false,
  provider: "none",
  momentoUrl: "",
  momentoSiteId: "",
  momentoProxy: true,
  measurementId: "",
  matomoUrl: "",
  matomoSiteId: "",
  customSnippet: "",
  allowedHosts: "",
  includeAdmin: false,
  placement: "head",
};

const fetchMock = vi.fn();

function renderTab(value: TrackingValue, violations: TrackingViolation[]) {
  fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith("/tracking/violations") && (!init?.method || init.method === "GET")) {
      return new Response(JSON.stringify({ data: violations }), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }
    if (url.endsWith("/tracking/allow")) {
      const body = JSON.parse(String(init?.body)) as { origin: string };
      return new Response(
        JSON.stringify({ data: { allowedHosts: `https://old.example, ${body.origin}` } }),
        { status: 200, headers: { "content-type": "application/json" } },
      );
    }
    return new Response(null, { status: 204 });
  });
  const onChange = vi.fn();
  const onAllowedHosts = vi.fn();
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <TrackingSettings value={value} onChange={onChange} onAllowedHosts={onAllowedHosts} />
    </QueryClientProvider>,
  );
  return { onChange, onAllowedHosts };
}

beforeEach(() => vi.stubGlobal("fetch", fetchMock));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  fetchMock.mockReset();
});

describe("TrackingSettings", () => {
  it("shows the collector fields for momento and nothing for the others", () => {
    renderTab({ ...off, provider: "momento" }, []);
    expect(screen.getByLabelText("Momento 수집기 주소")).toBeTruthy();
    expect(screen.getByLabelText(/같은 오리진 프록시/)).toBeTruthy();
    expect(screen.queryByLabelText("추적 코드")).toBeNull();
  });

  it("lists what the policy blocked and allows one origin with a click", async () => {
    const { onAllowedHosts } = renderTab({ ...off, enabled: true, provider: "custom", customSnippet: "<script></script>" }, [
      { origin: "https://momento.corp.example", directive: "connect-src", page: "/", count: 5, firstSeen: "", lastSeen: "", allowed: false },
      { origin: "https://cdn.corp.example", directive: "script-src-elem", page: "/", count: 1, firstSeen: "", lastSeen: "", allowed: true },
    ]);
    await screen.findByText("https://momento.corp.example");
    expect(screen.getByText("허용됨")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "허용" }));
    await waitFor(() =>
      expect(onAllowedHosts).toHaveBeenCalledWith("https://old.example, https://momento.corp.example"),
    );
    const call = fetchMock.mock.calls.find(([url]) => String(url).endsWith("/tracking/allow"));
    expect(call?.[1]?.method).toBe("POST");
  });

  it("warns before the server refuses an oversized snippet", () => {
    const snippet = "x".repeat(MAX_SNIPPET_BYTES + 1);
    renderTab({ ...off, provider: "custom", customSnippet: snippet }, []);
    expect(screen.getByText(/8,193 \/ 8,192 바이트/)).toBeTruthy();
    expect(snippetBytes("한")).toBe(3);
  });
});
