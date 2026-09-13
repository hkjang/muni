import { afterEach, describe, expect, it, vi } from "vitest";
import { handoffUrl, issueClaim, sendToService } from "./handoff";

const target = {
  origin: "https://ptium.intra",
  name: "ptium",
  formats: ["markdown" as const],
};
const claim = {
  claim: "AbC-123_xyz",
  source: "https://muni.intra",
  filename: "개편안.md",
  content_type: "text/markdown; charset=utf-8",
  bytes: 10,
  expires_at: "2026-09-13T00:05:00+09:00",
};

afterEach(() => vi.unstubAllGlobals());

describe("handoffUrl", () => {
  it("opens the peer's /handoff with the source and the claim, encoded", () => {
    expect(handoffUrl(target, claim)).toBe(
      "https://ptium.intra/handoff?source=https%3A%2F%2Fmuni.intra&claim=AbC-123_xyz",
    );
  });
});

describe("issueClaim", () => {
  it("posts the standard's request and reads the standard's reply, not an envelope", async () => {
    const fetchMock = vi.fn(
      async (_url: RequestInfo | URL, init?: RequestInit) => {
        expect(JSON.parse(String(init?.body))).toEqual({
          resource: "doc-1",
          format: "markdown",
        });
        return new Response(JSON.stringify(claim), {
          status: 201,
          headers: { "content-type": "application/json" },
        });
      },
    );
    vi.stubGlobal("fetch", fetchMock);
    await expect(issueClaim("doc-1", "markdown")).resolves.toEqual(claim);
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/v1/handoff/claims");
  });

  it("turns the server's refusal into an error with its message", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              error: { code: "DOCX_EXPORT_DISABLED", message: "DOCX 꺼짐" },
            }),
            {
              status: 403,
              headers: { "content-type": "application/json" },
            },
          ),
      ),
    );
    await expect(issueClaim("doc-1", "docx")).rejects.toThrow("DOCX 꺼짐");
  });
});

describe("sendToService", () => {
  it("opens the tab on the click and points it at the peer once the claim arrives", async () => {
    const tab = {
      location: { href: "" },
      close: vi.fn(),
      opener: {} as unknown,
    };
    const open = vi.fn(() => tab);
    vi.stubGlobal("open", open);
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify(claim), {
            status: 201,
            headers: { "content-type": "application/json" },
          }),
      ),
    );
    await sendToService("doc-1", target, "markdown");
    expect(open).toHaveBeenCalledWith("", "_blank");
    expect(tab.opener).toBeNull();
    expect(tab.location.href).toBe(handoffUrl(target, claim));
    expect(tab.close).not.toHaveBeenCalled();
  });

  it("closes the tab again when no claim comes", async () => {
    const tab = {
      location: { href: "" },
      close: vi.fn(),
      opener: {} as unknown,
    };
    vi.stubGlobal(
      "open",
      vi.fn(() => tab),
    );
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response("{}", {
            status: 500,
            headers: { "content-type": "application/json" },
          }),
      ),
    );
    await expect(sendToService("doc-1", target, "markdown")).rejects.toThrow();
    expect(tab.close).toHaveBeenCalled();
    expect(tab.location.href).toBe("");
  });
});
