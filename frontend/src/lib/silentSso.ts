import type { PublicSystem } from "../types";

// Somebody already signed in to Keycloak should open muni and see their
// documents, not a login screen. OIDC prompt=none asks the provider to answer
// from its existing session only: a code comes straight back, or
// error=login_required does. Neither draws anything.
//
// The whole of this module is making sure that happens at most once. A refusal
// that is retried bounces the browser between muni and the provider forever,
// and all the visitor sees is flicker. Three layers stop it: a once-per-tab
// flag, a suppression after a deliberate sign-out, and a marker the callback
// leaves in the address. Each survives the failure of another.

// sessionStorage rather than localStorage on purpose: a new tab should try
// again, a reload after a refusal should not.
const ATTEMPTED_KEY = "muni.sso.silentAttempted";
const SIGNED_OUT_KEY = "muni.sso.signedOut";

// Never here: the callback and login page are where loops start, and the
// rest are not screens at all.
const NEVER_ON = ["/login", "/api/", "/mcp", "/healthz", "/readyz", "/metrics"];

function readFlag(key: string): boolean {
  try {
    return window.sessionStorage.getItem(key) === "true";
  } catch {
    // Private modes and blocked site data throw. Reading that as "not yet
    // attempted" is exactly the loop, so a storage we cannot read counts as
    // one that says we already tried.
    return true;
  }
}

function writeFlag(key: string, value: boolean) {
  try {
    if (value) window.sessionStorage.setItem(key, "true");
    else window.sessionStorage.removeItem(key);
  } catch {
    // readFlag already fails closed; nothing more to do.
  }
}

/** The visitor signed out on purpose; signing them straight back in would look like sign-out is broken. */
export function markSignedOut() {
  writeFlag(SIGNED_OUT_KEY, true);
  writeFlag(ATTEMPTED_KEY, true);
}

/** A session exists again, so the next tab-session may try once more. */
export function clearSilentSsoState() {
  writeFlag(SIGNED_OUT_KEY, false);
  writeFlag(ATTEMPTED_KEY, false);
}

/** Only paths that stay on this site: begins with one slash, never two. */
export function safeReturnTo(value: string): string {
  return value.startsWith("/") && !value.startsWith("//") ? value : "/";
}

export function shouldAttemptSilentSso(
  system: Pick<PublicSystem, "oidcEnabled" | "oidcAutoLogin"> | null,
  location: Pick<Location, "pathname" | "search">,
): boolean {
  if (!system?.oidcEnabled || !system.oidcAutoLogin) return false;
  if (NEVER_ON.some((prefix) => location.pathname.startsWith(prefix)))
    return false;
  // The callback leaves this when the provider had no session; it holds even
  // when sessionStorage was cleared in between.
  const params = new URLSearchParams(location.search);
  if (params.has("sso") || params.has("error")) return false;
  if (readFlag(SIGNED_OUT_KEY)) return false;
  if (readFlag(ATTEMPTED_KEY)) return false;
  return true;
}

/**
 * Sends the whole window to the provider — not a hidden iframe, which
 * third-party-cookie blocking breaks and which needs the provider to allow
 * framing. The flag is written before leaving so that whatever comes back
 * cannot try again.
 */
export function beginSilentSso(
  returnTo: string,
  go: (url: string) => void = (url) => window.location.assign(url),
) {
  writeFlag(ATTEMPTED_KEY, true);
  go(
    `/api/v1/auth/oidc/start?prompt=none&return_to=${encodeURIComponent(safeReturnTo(returnTo))}`,
  );
}
