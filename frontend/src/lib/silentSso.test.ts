import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  beginSilentSso,
  clearSilentSsoState,
  markSignedOut,
  safeReturnTo,
  shouldAttemptSilentSso,
} from "./silentSso";

const on = { oidcEnabled: true, oidcAutoLogin: true };
const home = { pathname: "/", search: "" };

// Every one of these is a way the browser could end up bouncing between muni
// and the provider forever. The rule is that no combination of them retries.
describe("shouldAttemptSilentSso", () => {
  beforeEach(() => window.sessionStorage.clear());
  afterEach(() => vi.restoreAllMocks());

  it("tries once when SSO and auto-login are on and nothing has happened yet", () => {
    expect(shouldAttemptSilentSso(on, home)).toBe(true);
  });

  it("does nothing while auto-login is off, which is the default", () => {
    expect(
      shouldAttemptSilentSso({ oidcEnabled: true, oidcAutoLogin: false }, home),
    ).toBe(false);
    expect(
      shouldAttemptSilentSso({ oidcEnabled: false, oidcAutoLogin: true }, home),
    ).toBe(false);
    expect(shouldAttemptSilentSso(null, home)).toBe(false);
  });

  it("tries at most once per tab session", () => {
    const go = vi.fn();
    beginSilentSso("/docs/1", go);
    expect(go).toHaveBeenCalledWith(
      "/api/v1/auth/oidc/start?prompt=none&return_to=%2Fdocs%2F1",
    );
    expect(shouldAttemptSilentSso(on, home)).toBe(false);
  });

  it("does not try after a deliberate sign-out, until a session exists again", () => {
    markSignedOut();
    expect(shouldAttemptSilentSso(on, home)).toBe(false);
    clearSilentSsoState();
    expect(shouldAttemptSilentSso(on, home)).toBe(true);
  });

  it("honours the marker the callback leaves in the address", () => {
    // sessionStorage is empty here, as if it had been cleared: the address
    // alone must be enough.
    expect(shouldAttemptSilentSso(on, { pathname: "/", search: "?sso=none" })).toBe(false);
    expect(
      shouldAttemptSilentSso(on, { pathname: "/", search: "?error=login_required" }),
    ).toBe(false);
  });

  it("never tries on the login, callback, or non-screen paths", () => {
    for (const pathname of [
      "/login",
      "/api/v1/auth/oidc/callback",
      "/mcp",
      "/healthz",
      "/readyz",
      "/metrics",
    ]) {
      expect(shouldAttemptSilentSso(on, { pathname, search: "" })).toBe(false);
    }
    expect(shouldAttemptSilentSso(on, { pathname: "/docs/1", search: "" })).toBe(true);
  });

  it("treats storage it cannot read as already attempted", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    expect(shouldAttemptSilentSso(on, home)).toBe(false);
  });
});

describe("beginSilentSso", () => {
  beforeEach(() => window.sessionStorage.clear());

  it("carries the deep link so the visitor lands where they were going", () => {
    const go = vi.fn<(url: string) => void>();
    beginSilentSso("/workspace/7?tab=recent", go);
    expect(go).toHaveBeenCalledWith(
      expect.stringContaining(
        "return_to=" + encodeURIComponent("/workspace/7?tab=recent"),
      ),
    );
  });

  it("refuses a return path that leaves the site", () => {
    expect(safeReturnTo("//evil.example/x")).toBe("/");
    expect(safeReturnTo("https://evil.example/")).toBe("/");
    expect(safeReturnTo("/docs/1")).toBe("/docs/1");
    const go = vi.fn<(url: string) => void>();
    beginSilentSso("//evil.example/x", go);
    expect(go).toHaveBeenCalledWith(
      "/api/v1/auth/oidc/start?prompt=none&return_to=%2F",
    );
  });

  it("marks the attempt before leaving, so a refusal cannot restart it", () => {
    beginSilentSso("/", () => {
      // What the callback does on refusal is land here; by then the flag
      // must already be set.
      expect(shouldAttemptSilentSso(on, home)).toBe(false);
    });
  });
});
