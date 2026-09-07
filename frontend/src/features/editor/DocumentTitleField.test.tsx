import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { DocumentTitleField } from "./DocumentTitleField";

afterEach(cleanup);

function field(props: Partial<Parameters<typeof DocumentTitleField>[0]> = {}) {
  const onCommit = vi.fn().mockResolvedValue(undefined);
  const view = render(
    <DocumentTitleField
      title="처음 제목"
      canEdit
      onCommit={onCommit}
      {...props}
    />,
  );
  return { onCommit, view, input: screen.getByLabelText("문서 제목") };
}

describe("문서 제목 입력", () => {
  it("타이핑한 글자를 그대로 지닙니다", () => {
    const { input } = field();
    fireEvent.change(input, { target: { value: "새로운 제목" } });
    expect((input as HTMLInputElement).value).toBe("새로운 제목");
  });

  // The content autosave puts the server's whole document back into the
  // cache a second and a half after any edit. It used to land on the title
  // being typed and replace it with the one already saved.
  it("자동 저장이 도중에 끼어들어도 쓰던 제목을 덮어쓰지 않습니다", () => {
    const { input, view, onCommit } = field();
    fireEvent.change(input, { target: { value: "쓰는 중인 제목" } });
    view.rerender(
      <DocumentTitleField
        title="서버가 아는 옛 제목"
        canEdit
        onCommit={onCommit}
      />,
    );
    expect((input as HTMLInputElement).value).toBe("쓰는 중인 제목");
  });

  it("칸을 벗어날 때 한 번만 저장합니다", async () => {
    const { input, onCommit } = field();
    fireEvent.change(input, { target: { value: "저장될 제목" } });
    fireEvent.blur(input);
    expect(onCommit).toHaveBeenCalledWith("저장될 제목");
    expect(onCommit).toHaveBeenCalledTimes(1);
  });

  it("바뀐 것이 없으면 저장하지 않습니다", () => {
    const { input, onCommit } = field();
    fireEvent.blur(input);
    expect(onCommit).not.toHaveBeenCalled();
  });

  // Leaving the field while a syllable is still being composed is the
  // browser moving focus for the input method, not the person leaving.
  it("한글을 조합하는 중에는 저장하지 않습니다", () => {
    const { input, onCommit } = field();
    fireEvent.compositionStart(input);
    fireEvent.change(input, { target: { value: "한" } });
    fireEvent.blur(input);
    expect(onCommit).not.toHaveBeenCalled();
  });

  it("읽기 전용이면 고칠 수 없습니다", () => {
    const { input } = field({ canEdit: false });
    expect((input as HTMLInputElement).disabled).toBe(true);
  });
});
