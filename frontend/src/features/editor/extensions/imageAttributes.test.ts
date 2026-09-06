import { describe, expect, it } from "vitest";
import { Editor } from "@tiptap/core";
import Document from "@tiptap/extension-document";
import Paragraph from "@tiptap/extension-paragraph";
import Text from "@tiptap/extension-text";
import {
  SizedImage,
  contentWidth,
  heightFor,
  percentFor,
  pixelsFor,
} from "./imageAttributes";

function imageHTML(attrs: Record<string, unknown>): string {
  const editor = new Editor({
    extensions: [Document, Paragraph, Text, SizedImage],
  });
  try {
    editor.commands.setContent({ type: "doc", content: [{ type: "image", attrs }] });
    return editor.getHTML();
  } finally {
    editor.destroy();
  }
}

describe("image sizing", () => {
  it("turns a preset into the width that is stored", () => {
    expect(pixelsFor(100)).toBe(contentWidth);
    expect(pixelsFor(50)).toBe(Math.round(contentWidth / 2));
  });

  it("reads a stored width back as its preset", () => {
    expect(percentFor(pixelsFor(25))).toBe(25);
    expect(percentFor(pixelsFor(75))).toBe(75);
  });

  it("treats an image with no width as full width", () => {
    expect(percentFor(null)).toBe(100);
  });

  it("reports nothing for a width that is not one of the presets", () => {
    expect(percentFor(123)).toBeNull();
  });

  it("tolerates rounding either way", () => {
    expect(percentFor(pixelsFor(50) + 2)).toBe(50);
  });
});

describe("a picture drawn in a shape of its own", () => {
  it("keeps that shape when the width moves to a preset", () => {
    // A stamp drawn 163x40 is not the shape of its bytes; doubling the width
    // without the height would stretch it.
    expect(heightFor(163, 40, 326)).toBe(80);
  });

  it("says nothing about a picture that has no stored height", () => {
    expect(heightFor(163, null, 326)).toBeNull();
    expect(heightFor(null, 40, 326)).toBeNull();
  });

  it("never scales a height away entirely", () => {
    expect(heightFor(716, 4, 25)).toBe(1);
  });

  // The page stylesheet gives every picture height:auto so a wide one shrinks
  // to the page instead of spilling off it — which also flattened a squashed
  // picture back to the shape of its bytes. The ratio survives the shrinking.
  it("is drawn with the ratio it was stored with", () => {
    const html = imageHTML({ src: "/api/v1/attachments/x", width: 163, height: 40 });
    // The browser rewrites the style it was handed, so the shape is what is
    // checked rather than the spelling.
    expect(html).toMatch(/aspect-ratio:\s*163\s*\/\s*40/);
    expect(html).toContain('height="40"');
  });

  it("says nothing about a picture stored without a height", () => {
    const html = imageHTML({ src: "/api/v1/attachments/x", width: 163 });
    expect(html).not.toContain("aspect-ratio");
  });
});
