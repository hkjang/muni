import { describe, expect, it } from "vitest";
import { Schema } from "@tiptap/pm/model";
import type { Node as ProseMirrorNode } from "@tiptap/pm/model";
import {
  blockIdAttribute,
  collectAssignments,
  createBlockId,
  defaultBlockIdTypes,
} from "./blockId";

const schema = new Schema({
  nodes: {
    doc: { content: "block+" },
    paragraph: {
      group: "block",
      content: "text*",
      attrs: { [blockIdAttribute]: { default: null } },
    },
    heading: {
      group: "block",
      content: "text*",
      attrs: { [blockIdAttribute]: { default: null } },
    },
    bulletList: { group: "block", content: "paragraph+" },
    text: {},
  },
});

const types = new Set(defaultBlockIdTypes);

function paragraph(text: string, blockId: string | null = null): ProseMirrorNode {
  return schema.node("paragraph", { [blockIdAttribute]: blockId }, [schema.text(text)]);
}

describe("createBlockId", () => {
  it("produces unique identifiers", () => {
    const ids = new Set(Array.from({ length: 500 }, createBlockId));
    expect(ids.size).toBe(500);
  });

  it("uses a recognisable prefix", () => {
    expect(createBlockId()).toMatch(/^blk_[0-9a-z]+$/);
  });
});

describe("collectAssignments", () => {
  it("stamps every block that has no id", () => {
    const doc = schema.node("doc", null, [paragraph("첫째"), paragraph("둘째")]);
    const assignments = collectAssignments(doc, types);
    expect(assignments).toHaveLength(2);
    expect(new Set(assignments.map((item) => item.id)).size).toBe(2);
  });

  it("leaves blocks that already have a unique id alone", () => {
    const doc = schema.node("doc", null, [
      paragraph("첫째", "blk_a"),
      paragraph("둘째", "blk_b"),
    ]);
    expect(collectAssignments(doc, types)).toHaveLength(0);
  });

  it("re-stamps a duplicate so a split or paste cannot clone an identity", () => {
    // Splitting a paragraph copies its attributes onto the new node.
    const doc = schema.node("doc", null, [
      paragraph("앞부분", "blk_same"),
      paragraph("뒷부분", "blk_same"),
    ]);
    const assignments = collectAssignments(doc, types);
    expect(assignments).toHaveLength(1);
    // The first block in document order keeps the original identity.
    expect(assignments[0]!.position).toBeGreaterThan(0);
    expect(assignments[0]!.id).not.toBe("blk_same");
  });

  it("ignores node types that are not anchorable", () => {
    const doc = schema.node("doc", null, [
      schema.node("bulletList", null, [paragraph("항목", "blk_x")]),
    ]);
    const assignments = collectAssignments(doc, types);
    // The paragraph inside is still anchorable; the list itself is not.
    expect(assignments).toHaveLength(0);
  });

  it("does not reuse an id it just handed out", () => {
    const doc = schema.node("doc", null, [
      paragraph("a"),
      paragraph("b"),
      paragraph("c", "blk_fixed"),
    ]);
    const assignments = collectAssignments(doc, types);
    const ids = assignments.map((item) => item.id);
    expect(new Set(ids).size).toBe(ids.length);
    expect(ids).not.toContain("blk_fixed");
  });
});
