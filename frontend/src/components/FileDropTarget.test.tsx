import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { FileDropTarget } from "./FileDropTarget";

afterEach(cleanup);

// jsdom has no DataTransfer; the handlers only ever read `types` and `files`.
function drop(element: Element, files: File[]) {
  const event = new Event("drop", { bubbles: true, cancelable: true });
  Object.defineProperty(event, "dataTransfer", {
    value: { files, types: ["Files"] },
  });
  fireEvent(element, event);
  return event;
}

function target(enabled = true) {
  const onFiles = vi.fn();
  render(
    <FileDropTarget enabled={enabled} onFiles={onFiles} hint="놓으세요">
      <p>목록</p>
    </FileDropTarget>,
  );
  return { onFiles, zone: screen.getByText("목록").parentElement as Element };
}

describe("화면에 파일을 떨어뜨릴 때", () => {
  it("문서 파일을 넘겨받고 브라우저가 열지 못하게 막습니다", () => {
    const { onFiles, zone } = target();
    const event = drop(zone, [new File(["x"], "보고서.docx")]);
    expect(event.defaultPrevented).toBe(true);
    expect(onFiles).toHaveBeenCalledTimes(1);
    expect(onFiles.mock.calls[0]?.[0]?.[0]?.name).toBe("보고서.docx");
  });

  it("문서가 아닌 파일은 넘기지 않습니다", () => {
    const { onFiles, zone } = target();
    drop(zone, [new File(["x"], "사진.png", { type: "image/png" })]);
    expect(onFiles).not.toHaveBeenCalled();
  });

  // Even where a drop does nothing, the browser must not be left to open the
  // file and throw away the screen the person was on.
  it("가져올 수 없는 화면에서도 브라우저가 파일을 열지 않습니다", () => {
    const { onFiles, zone } = target(false);
    const event = drop(zone, [new File(["x"], "보고서.docx")]);
    expect(event.defaultPrevented).toBe(true);
    expect(onFiles).not.toHaveBeenCalled();
  });
});
