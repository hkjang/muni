import { Extension } from "@tiptap/core";
import { Plugin, PluginKey } from "@tiptap/pm/state";

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
 * PasteBehaviour makes pasting do what people expect it to.
 *
 * Two things, both of which every other editor does and muni did not: pasting
 * an address over selected words turns them into a link instead of replacing
 * them, and Ctrl+Shift+V pastes the text without carrying a stylesheet's worth
 * of formatting in from wherever it was copied.
 */
export const PasteBehaviour = Extension.create({
  name: "muniPaste",

  addProseMirrorPlugins() {
    // The paste event carries no modifier keys, so whether Shift was held has
    // to be remembered from the keystroke that caused it.
    let plainNext = false;

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
