import { Node, mergeAttributes } from "@tiptap/core";

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    muniFootnote: {
      /** Attach a note to the cursor position. */
      setFootnote: (text?: string) => ReturnType;
    };
  }
}

/**
 * 각주.
 *
 * The note's words live inside the node, at the point in the sentence they
 * belong to, so moving the sentence moves the note with it. Nothing is
 * numbered in storage: a note's number is its position among the others, and a
 * stored number is wrong the moment a paragraph moves. The server counts them
 * when it renders — the same rule heading numbers and the contents list
 * already follow.
 *
 * On screen the note shows inline in brackets rather than as a bare number.
 * The editor has no pages to put a note at the foot of, and a number that
 * leads nowhere is worse than the words themselves: the author needs to read
 * what they wrote. A .docx export turns it into a real Word footnote.
 */
export const Footnote = Node.create({
  name: "footnote",
  group: "inline",
  inline: true,
  content: "text*",
  // Marks would let somebody bold half a note for no reason and complicate
  // every path that reads it back out as one line.
  marks: "",

  parseHTML() {
    return [{ tag: "span[data-footnote]" }];
  },

  renderHTML({ HTMLAttributes }) {
    return [
      "span",
      mergeAttributes(HTMLAttributes, {
        "data-footnote": "",
        class: "muni-footnote",
        // Read out as a note rather than as a parenthesis in the sentence.
        role: "note",
      }),
      0,
    ];
  },

  addCommands() {
    return {
      setFootnote:
        (text = "") =>
        ({ chain }) =>
          chain()
            .focus()
            .insertContent({
              type: this.name,
              content: text ? [{ type: "text", text }] : [],
            })
            .run(),
    };
  },
});
