import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { ErrorBoundary } from "./ErrorBoundary";

function Throws({ message }: { message: string }): never {
  throw new Error(message);
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("ErrorBoundary", () => {
  it("shows the children when nothing is wrong", () => {
    render(
      <ErrorBoundary>
        <p>정상 화면</p>
      </ErrorBoundary>,
    );
    expect(screen.getByText("정상 화면")).toBeTruthy();
  });

  it("replaces a white page with something a person can act on", () => {
    // Without a boundary a render error unmounts the whole tree and the person
    // sees nothing at all — no message, no way back.
    vi.spyOn(console, "error").mockImplementation(() => {});
    render(
      <ErrorBoundary>
        <Throws message="무언가 잘못되었습니다" />
      </ErrorBoundary>,
    );
    expect(screen.getByText("문제가 생겼습니다")).toBeTruthy();
    expect(screen.getByText("새로고침")).toBeTruthy();
  });

  it("names a failed screen download as what it is", () => {
    // Routes arrive as separate files now, so this failure means the network
    // dropped or muni was updated — and reloading really is the fix. Saying
    // "문제가 생겼습니다" would send someone hunting for a bug that is not there.
    vi.spyOn(console, "error").mockImplementation(() => {});
    render(
      <ErrorBoundary>
        <Throws message="Failed to fetch dynamically imported module: /assets/EditorPage-abc.js" />
      </ErrorBoundary>,
    );
    expect(screen.getByText("화면을 불러오지 못했습니다")).toBeTruthy();
  });

  it("keeps the message where someone can copy it", () => {
    // An administrator being told "문제가 생겼습니다" and nothing else cannot
    // help.
    vi.spyOn(console, "error").mockImplementation(() => {});
    render(
      <ErrorBoundary>
        <Throws message="TypeError: cannot read length of undefined" />
      </ErrorBoundary>,
    );
    expect(
      screen.getByText("TypeError: cannot read length of undefined"),
    ).toBeTruthy();
  });
});
