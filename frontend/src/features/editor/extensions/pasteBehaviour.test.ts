import { describe, expect, it } from "vitest";
import { Editor } from "@tiptap/core";
import { documentExtensions } from "../documentExtensions";
import { PasteBehaviour, isPastableURL } from "./pasteBehaviour";

describe("isPastableURL", () => {
  it("recognises a web address", () => {
    expect(isPastableURL("https://example.com/보고서")).toBe(true);
    expect(isPastableURL("http://intranet/a")).toBe(true);
  });

  it("recognises an email link", () => {
    expect(isPastableURL("mailto:hong@example.com")).toBe(true);
  });

  it("ignores surrounding whitespace", () => {
    expect(isPastableURL("  https://example.com \n")).toBe(true);
  });

  it("is not a link when it is a sentence that mentions one", () => {
    expect(isPastableURL("자세한 내용은 https://example.com 을 보세요")).toBe(
      false,
    );
  });

  it("is not a link when it is ordinary text", () => {
    expect(isPastableURL("회의 결과")).toBe(false);
    expect(isPastableURL("")).toBe(false);
  });

  it("refuses a scheme that would run code", () => {
    expect(isPastableURL("javascript:alert(1)")).toBe(false);
    expect(isPastableURL("data:text/html,<script>")).toBe(false);
    expect(isPastableURL("file:///etc/passwd")).toBe(false);
  });

  it("refuses something far too long to be an address anyone pasted", () => {
    expect(isPastableURL("https://example.com/" + "a".repeat(3000))).toBe(
      false,
    );
  });
});

describe("markdown pasted into the editor", () => {
  // jsdom has no DataTransfer, and the handler only ever asks for text.
  function paste(editor: Editor, text: string) {
    const event = new Event("paste", { bubbles: true, cancelable: true });
    Object.defineProperty(event, "clipboardData", {
      value: { getData: (kind: string) => (kind === "text/plain" ? text : "") },
    });
    editor.view.dom.dispatchEvent(event);
  }

  function withEditor(run: (editor: Editor) => void) {
    const editor = new Editor({
      extensions: documentExtensions().concat([PasteBehaviour]),
    });
    try {
      run(editor);
    } finally {
      editor.destroy();
    }
  }

  it("becomes real formatting rather than the characters", () => {
    withEditor((editor) => {
      paste(editor, "## 제목입니다\n\n- 첫째\n- 둘째\n\n**굵은말** 입니다");
      const json = JSON.stringify(editor.getJSON());
      expect(json, "heading").toContain('"heading"');
      expect(json, "list").toContain('"bulletList"');
      expect(json, "bold").toContain('"bold"');
      expect(json, "the hashes are gone").not.toContain("## ");
      expect(json, "the asterisks are gone").not.toContain("**");
    });
  });

  it("leaves ordinary prose alone", () => {
    withEditor((editor) => {
      paste(editor, "그냥 문장입니다. 서식이 없습니다.");
      const json = JSON.stringify(editor.getJSON());
      expect(json, "no heading invented").not.toContain('"heading"');
    });
  });

  it("does not read a code sample as instructions", () => {
    withEditor((editor) => {
      editor.commands.setContent({
        type: "doc",
        content: [
          { type: "codeBlock", content: [{ type: "text", text: "기존" }] },
        ],
      });
      editor.commands.setTextSelection(2);
      paste(editor, "## 제목처럼 보이는 주석\n**굵게**");
      const json = JSON.stringify(editor.getJSON());
      expect(json, "still a code block").toContain('"codeBlock"');
      expect(json, "no heading made").not.toContain('"heading"');
    });
  });
});
