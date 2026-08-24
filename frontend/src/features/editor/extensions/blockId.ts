import { Extension } from "@tiptap/core";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import type { Node as ProseMirrorNode } from "@tiptap/pm/model";

/**
 * Node types that get a stable identity. Comments, citations, AI patches,
 * revision diffs and deep links all need to point at a block that survives
 * editing, and a document position does not: inserting a paragraph above
 * shifts every offset below it.
 *
 * Containers (bulletList, tableRow, …) are left out on purpose — an anchor is
 * only useful on something a reader can be taken to.
 */
export const defaultBlockIdTypes = [
  "paragraph",
  "heading",
  "blockquote",
  "codeBlock",
  "horizontalRule",
  "image",
  "listItem",
  "taskItem",
  "table",
];

export const blockIdAttribute = "blockId";
const blockIdPluginKey = new PluginKey("muniBlockId");

/**
 * createBlockId returns a lexicographically sortable identifier: a base36
 * timestamp followed by randomness, so ids created later sort later while
 * staying unique across collaborators.
 */
export function createBlockId(): string {
  const time = Date.now().toString(36).padStart(9, "0");
  return `blk_${time}${randomSuffix(10)}`;
}

function randomSuffix(length: number): string {
  const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz";
  const source = globalThis.crypto;
  if (source?.getRandomValues) {
    const bytes = new Uint8Array(length);
    source.getRandomValues(bytes);
    return Array.from(bytes, (byte) => alphabet[byte % alphabet.length]).join("");
  }
  let out = "";
  for (let index = 0; index < length; index++) {
    out += alphabet[Math.floor(Math.random() * alphabet.length)];
  }
  return out;
}

export type BlockIdOptions = {
  types: string[];
};

/**
 * BlockId keeps a `blockId` attribute on every anchorable block.
 *
 * Assignment happens in appendTransaction rather than in the node spec so that
 * splitting or pasting a block — both of which copy the source node's
 * attributes — cannot leave two blocks claiming the same identity. The first
 * block in document order keeps the id; later copies are re-stamped.
 */
export const BlockId = Extension.create<BlockIdOptions>({
  name: "blockId",

  addOptions() {
    return { types: defaultBlockIdTypes };
  },

  addGlobalAttributes() {
    return [
      {
        types: this.options.types,
        attributes: {
          [blockIdAttribute]: {
            default: null,
            // The id is data, not styling: keep it out of the rendered markup
            // except as a data attribute so deep links can find the element.
            parseHTML: (element) => element.getAttribute("data-block-id"),
            renderHTML: (attributes) => {
              const value = attributes[blockIdAttribute];
              return value ? { "data-block-id": value } : {};
            },
          },
        },
      },
    ];
  },

  addProseMirrorPlugins() {
    const types = new Set(this.options.types);
    return [
      new Plugin({
        key: blockIdPluginKey,
        appendTransaction: (transactions, _oldState, newState) => {
          if (!transactions.some((transaction) => transaction.docChanged)) return null;
          // Remote CRDT updates already carry the ids their author assigned;
          // re-stamping them here would fight the other client.
          if (transactions.some(isRemote)) return null;

          const assignments = collectAssignments(newState.doc, types);
          if (assignments.length === 0) return null;

          const transaction = newState.tr;
          for (const { position, node, id } of assignments) {
            transaction.setNodeMarkup(position, undefined, {
              ...node.attrs,
              [blockIdAttribute]: id,
            });
          }
          return transaction.setMeta("addToHistory", false);
        },
      }),
    ];
  },
});

function isRemote(transaction: { getMeta: (key: string) => unknown }): boolean {
  return Boolean(transaction.getMeta("y-sync$")) || Boolean(transaction.getMeta("y-undo$"));
}

type Assignment = { position: number; node: ProseMirrorNode; id: string };

/**
 * collectAssignments finds blocks with no id and blocks whose id another block
 * already claimed. Exported for tests.
 */
export function collectAssignments(
  doc: ProseMirrorNode,
  types: Set<string>,
): Assignment[] {
  const seen = new Set<string>();
  const assignments: Assignment[] = [];
  doc.descendants((node, position) => {
    if (!node.isBlock || !types.has(node.type.name)) return;
    const current = node.attrs[blockIdAttribute];
    if (typeof current === "string" && current && !seen.has(current)) {
      seen.add(current);
      return;
    }
    let id = createBlockId();
    while (seen.has(id)) id = createBlockId();
    seen.add(id);
    assignments.push({ position, node, id });
  });
  return assignments;
}
