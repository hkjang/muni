import { Extension } from "@tiptap/core";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import type { EditorState } from "@tiptap/pm/state";
import {
  looksLikeMarkdown,
  markdownToContent,
} from "../../../lib/markdownContent";

/**
 * isPastableURL decides whether what was pasted is an address.
 *
 * Only the schemes a document should carry count, and it has to be the whole
 * clipboard — a paragraph that happens to mention a link is text being pasted,
 * not a link being applied.
 */
export function isPastableURL(value: string): boolean {
  const trimmed = value.trim();
  if (!trimmed || /\s/u.test(trimmed)) return false;
  if (trimmed.length > 2048) return false;
  return /^(https?:\/\/|mailto:)[^\s]+$/i.test(trimmed);
}

/**
 * insideVerbatim reports whether the cursor sits somewhere the characters are
 * the content — a code block, or a run already marked as code.
 *
 * Markdown pasted there is a code sample. Turning ``` into a code block, or
 * **into** bold, would be reading somebody's example as an instruction.
 */
export function insideVerbatim(state: EditorState): boolean {
  const { $from } = state.selection;
  for (let depth = $from.depth; depth > 0; depth--) {
    if ($from.node(depth).type.name === "codeBlock") return true;
  }
  const codeMark = state.schema.marks.code;
  return Boolean(codeMark && codeMark.isInSet($from.marks()));
}

/**
 * PasteBehaviour makes pasting do what people expect it to.
 *
 * Three things, all of which every other editor does and muni did not: pasting
 * an address over selected words turns them into a link instead of replacing
 * them, Ctrl+Shift+V pastes the text without carrying a stylesheet's worth of
 * formatting in from wherever it was copied, and Markdown pasted from a file,
 * a chat or a model's answer arrives as formatting rather than as the literal
 * characters `## 제목`.
 */
export const PasteBehaviour = Extension.create({
  name: "muniPaste",

  addProseMirrorPlugins() {
    // The paste event carries no modifier keys, so whether Shift was held has
    // to be remembered from the keystroke that caused it.
    let plainNext = false;
    const editor = this.editor;

    return [
      new Plugin({
        key: new PluginKey("muniPaste"),
        props: {
          handleKeyDown(_view, event) {
            if (
              (event.metaKey || event.ctrlKey) &&
              event.shiftKey &&
              event.key.toLowerCase() === "v"
            )
              plainNext = true;
            return false;
          },
          handlePaste(view, event) {
            const text = event.clipboardData?.getData("text/plain") ?? "";

            if (plainNext) {
              plainNext = false;
              if (!text) return false;
              const { state, dispatch } = view;
              dispatch(state.tr.insertText(text).scrollIntoView());
              return true;
            }

            // Markdown before links: an address on its own is not Markdown,
            // so the two never both match.
            if (
              text &&
              looksLikeMarkdown(text) &&
              !insideVerbatim(view.state) &&
              editor
            ) {
              const content = markdownToContent(text, { forceBlocks: false });
              // A string back means the reader found no formatting worth
              // keeping; the ordinary paste does that better.
              if (typeof content !== "string") {
                editor.commands.insertContent(content);
                return true;
              }
            }

            if (!text || !isPastableURL(text)) return false;
            const { state, dispatch } = view;
            const { from, to, empty } = state.selection;
            if (empty) return false;
            const linkType = state.schema.marks.link;
            if (!linkType) return false;
            // Replacing the words with the address loses what they said; the
            // address belongs on them.
            dispatch(
              state.tr
                .removeMark(from, to, linkType)
                .addMark(from, to, linkType.create({ href: text.trim() })),
            );
            return true;
          },
        },
      }),
    ];
  },
});
