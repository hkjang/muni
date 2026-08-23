export class ApiError extends Error {
  status: number;
  code: string;
  details?: Record<string, unknown>;

  constructor(
    status: number,
    code: string,
    message: string,
    details?: Record<string, unknown>,
  ) {
    super(message);
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

type Envelope<T> =
  | { data: T; error?: never }
  | {
      data?: never;
      error: {
        code: string;
        message: string;
        details?: Record<string, unknown>;
      };
    };

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (
    init.body &&
    !(init.body instanceof FormData) &&
    !headers.has("Content-Type")
  ) {
    headers.set("Content-Type", "application/json");
  }
  headers.set("Accept", "application/json");
  const response = await fetch(path, {
    ...init,
    headers,
    credentials: "same-origin",
  });
  if (response.status === 204) return undefined as T;
  const contentType = response.headers.get("content-type") ?? "";
  if (!contentType.includes("application/json")) {
    if (!response.ok)
      throw new ApiError(
        response.status,
        "HTTP_ERROR",
        `요청에 실패했습니다 (${response.status}).`,
      );
    return (await response.text()) as T;
  }
  const envelope = (await response.json()) as Envelope<T>;
  if (!response.ok || envelope.error) {
    const error = envelope.error ?? {
      code: "HTTP_ERROR",
      message: `요청에 실패했습니다 (${response.status}).`,
    };
    throw new ApiError(
      response.status,
      error.code,
      error.message,
      error.details,
    );
  }
  return envelope.data as T;
}

export const jsonBody = (value: unknown): Pick<RequestInit, "body"> => ({
  body: JSON.stringify(value),
});

export function formatDate(value?: string | null, withTime = true) {
  if (!value) return "—";
  return new Intl.DateTimeFormat("ko-KR", {
    year: "numeric",
    month: "short",
    day: "numeric",
    ...(withTime ? { hour: "2-digit", minute: "2-digit" } : {}),
  }).format(new Date(value));
}

export function errorMessage(error: unknown) {
  return error instanceof Error
    ? error.message
    : "알 수 없는 오류가 발생했습니다.";
}
