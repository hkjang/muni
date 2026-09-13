import { ApiError } from "./api";

/**
 * Sending a document to another service without a download (HANDOFF-STANDARD).
 *
 * muni issues a claim — a single-use, five-minute token for one rendering of
 * the document — and the other service's /handoff page fetches the document
 * from muni with it. The person sees a new tab open on the other service
 * with the document already there.
 */

export type HandoffFormat = "markdown" | "docx";

/** A service the administrator listed that can receive what muni sends. */
export type HandoffTarget = {
  origin: string;
  name: string;
  formats: HandoffFormat[];
};

/** What POST /api/v1/handoff/claims answers; the shape is the standard's,
 * not muni's usual `{data}` envelope. */
export type HandoffClaim = {
  claim: string;
  source: string;
  filename: string;
  content_type: string;
  bytes: number;
  expires_at: string;
};

export const formatLabels: Record<HandoffFormat, string> = {
  markdown: "Markdown",
  docx: "DOCX",
};

/** handoffUrl is where the receiving service opens the document. */
export function handoffUrl(
  target: Pick<HandoffTarget, "origin">,
  claim: Pick<HandoffClaim, "claim" | "source">,
): string {
  const params = new URLSearchParams({
    source: claim.source,
    claim: claim.claim,
  });
  return `${target.origin}/handoff?${params.toString()}`;
}

export async function issueClaim(
  documentId: string,
  format: HandoffFormat,
): Promise<HandoffClaim> {
  const response = await fetch("/api/v1/handoff/claims", {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify({ resource: documentId, format }),
  });
  const body = (await response.json().catch(() => ({}))) as
    HandoffClaim | { error?: { code: string; message: string } };
  if (!response.ok || "error" in body) {
    const error = "error" in body ? body.error : undefined;
    throw new ApiError(
      response.status,
      error?.code ?? "HTTP_ERROR",
      error?.message ?? `요청에 실패했습니다 (${response.status}).`,
    );
  }
  return body as HandoffClaim;
}

/**
 * sendToService opens the other service on this document. The tab is opened
 * before the claim is asked for, because a window opened after an await is
 * not the click any more and the browser blocks it as a pop-up; it is pointed
 * at the destination once the claim arrives, and closed if it never does.
 */
export async function sendToService(
  documentId: string,
  target: HandoffTarget,
  format: HandoffFormat,
): Promise<void> {
  // Not opened with "noopener": that makes window.open return null, and
  // the handle is what the destination is written into. The opener is cut
  // by hand instead, so the other service cannot navigate this tab.
  const tab = window.open("", "_blank");
  if (tab) tab.opener = null;
  try {
    const claim = await issueClaim(documentId, format);
    const url = handoffUrl(target, claim);
    if (tab) tab.location.href = url;
    else window.open(url, "_blank", "noopener,noreferrer");
  } catch (cause) {
    tab?.close();
    throw cause;
  }
}
