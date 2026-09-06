import { describe, expect, it, vi } from "vitest";
import { Editor } from "@tiptap/core";
import { documentExtensions } from "../documentExtensions";
import { PasteBehaviour } from "./pasteBehaviour";
import { FileDrop, fileKind, type DroppedFiles } from "./fileDrop";

// jsdom has no DataTransfer; the handlers only ever read `files`.
function withFiles(
  event: Event,
  property: "dataTransfer" | "clipboardData",
  files: File[],
) {
  Object.defineProperty(event, property, {
    value: {
      files,
      types: files.length ? ["Files"] : [],
      getData: () => "",
    },
  });
  return event;
}

function withEditor(run: (editor: Editor, drops: DroppedFiles[]) => void) {
  const drops: DroppedFiles[] = [];
  const editor = new Editor({
    extensions: documentExtensions().concat([
      FileDrop.configure({ onFiles: (drop) => drops.push(drop) }),
      PasteBehaviour,
    ]),
    content: "<p>앞 문단</p>",
  });
  // ProseMirror works out where a drop landed before it asks any plugin,
  // and jsdom lays nothing out, so the position is answered for it.
  (editor.view as { posAtCoords: unknown }).posAtCoords = () => ({
    pos: 1,
    inside: 0,
  });
  try {
    run(editor, drops);
  } finally {
    editor.destroy();
  }
}

const report = new File(["# 제목"], "보고서.md", { type: "text/markdown" });

describe("fileKind", () => {
  it("knows a picture, a document and neither", () => {
    expect(fileKind(new File([""], "a.png", { type: "image/png" }))).toBe(
      "image",
    );
    expect(fileKind(new File([""], "결재.hwpx"))).toBe("document");
    expect(fileKind(new File([""], "보고서.DOCX"))).toBe("document");
    expect(fileKind(new File([""], "archive.zip"))).toBe("unsupported");
  });
});

describe("files dropped on the editor", () => {
  it("are handed over with a position instead of opened by the browser", () => {
    withEditor((editor, drops) => {
      const event = withFiles(
        new Event("drop", { bubbles: true, cancelable: true }),
        "dataTransfer",
        [report],
      );
      editor.view.dom.dispatchEvent(event);
      expect(event.defaultPrevented).toBe(true);
      expect(drops).toHaveLength(1);
      expect(drops[0]?.files[0]?.name).toBe("보고서.md");
      expect(typeof drops[0]?.position).toBe("number");
    });
  });

  it("leaves a drop that carries no files to the editor", () => {
    withEditor((editor, drops) => {
      const event = withFiles(
        new Event("drop", { bubbles: true, cancelable: true }),
        "dataTransfer",
        [],
      );
      editor.view.dom.dispatchEvent(event);
      expect(drops).toHaveLength(0);
    });
  });
});

describe("files pasted into the editor", () => {
  it("go the same way as a drop, ahead of the text paste", () => {
    withEditor((editor, drops) => {
      const spy = vi.fn();
      editor.on("update", spy);
      const event = withFiles(
        new Event("paste", { bubbles: true, cancelable: true }),
        "clipboardData",
        [new File([""], "shot.png", { type: "image/png" })],
      );
      editor.view.dom.dispatchEvent(event);
      expect(drops).toHaveLength(1);
      expect(drops[0]?.files[0]?.type).toBe("image/png");
      expect(spy).not.toHaveBeenCalled();
    });
  });
});
