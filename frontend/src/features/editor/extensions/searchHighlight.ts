import { Extension } from "@tiptap/core";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";
import type { Node as ProseMirrorNode } from "@tiptap/pm/model";
import { findMatches, type FindOptions, type MatchRange } from "../find/findMatches";

export const searchPluginKey = new PluginKey("muniSearch");

/** A match, in editor positions rather than string offsets. */
export type DocumentMatch = { from: number; to: number };

/**
 * documentText flattens the document into one string, remembering where each
 * character came from.
 *
 * Search runs on the flat string — that is the only way to find a phrase that
 * spans several text nodes, which is what happens the moment one word in it is
 * bold. The position list maps a match back to the document.
 */
export function documentText(doc: ProseMirrorNode): {
  text: string;
  positions: number[];
} {
  let text = "";
  const positions: number[] = [];
  doc.descendants((node, pos) => {
    if (node.isText) {
      const value = node.text ?? "";
      for (let index = 0; index < value.length; index += 1) {
        text += value[index];
        positions.push(pos + index);
      }
      return false;
    }
    // A newline between blocks stops a match from running across a paragraph
    // boundary, where it would read as a phrase nobody wrote.
    if (node.isBlock && text.length > 0 && !text.endsWith("\n")) {
      text += "\n";
      positions.push(pos);
    }
    return true;
  });
  return { text, positions };
}

export function locateMatches(
  doc: ProseMirrorNode,
  query: string,
  options: FindOptions,
): DocumentMatch[] {
  if (!query) return [];
  const { text, positions } = documentText(doc);
  return findMatches(text, query, options).map((match: MatchRange) => ({
    from: positions[match.start] ?? 0,
    to: (positions[match.end - 1] ?? 0) + 1,
  }));
}

type SearchState = {
  query: string;
  options: FindOptions;
  matches: DocumentMatch[];
  active: number;
};

const empty: SearchState = { query: "", options: {}, matches: [], active: -1 };

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    muniSearch: {
      setSearch: (query: string, options?: FindOptions) => ReturnType;
      setActiveMatch: (index: number) => ReturnType;
      clearSearch: () => ReturnType;
    };
  }
}

/**
 * SearchHighlight marks every match in the document and picks one out.
 *
 * The matches are recomputed whenever the document changes so a replacement
 * does not leave the highlights pointing at text that has moved.
 */
export const SearchHighlight = Extension.create({
  name: "muniSearch",

  addCommands() {
    return {
      setSearch:
        (query, options = {}) =>
        ({ dispatch, tr, state }) => {
          if (dispatch) {
            const matches = locateMatches(state.doc, query, options);
            dispatch(
              tr.setMeta(searchPluginKey, {
                query,
                options,
                matches,
                active: matches.length > 0 ? 0 : -1,
              } satisfies SearchState),
            );
          }
          return true;
        },
      setActiveMatch:
        (index) =>
        ({ dispatch, tr, state }) => {
          const current = searchPluginKey.getState(state) as SearchState | undefined;
          if (!current) return false;
          if (dispatch)
            dispatch(tr.setMeta(searchPluginKey, { ...current, active: index }));
          return true;
        },
      clearSearch:
        () =>
        ({ dispatch, tr }) => {
          if (dispatch) dispatch(tr.setMeta(searchPluginKey, empty));
          return true;
        },
    };
  },

  addProseMirrorPlugins() {
    return [
      new Plugin({
        key: searchPluginKey,
        state: {
          init: () => empty,
          apply(transaction, value, _oldState, newState) {
            const meta = transaction.getMeta(searchPluginKey) as SearchState | undefined;
            if (meta) return meta;
            if (!transaction.docChanged || !value.query) return value;
            // The document moved under the matches; find them again rather
            // than trying to map stale ranges through the change.
            const matches = locateMatches(newState.doc, value.query, value.options);
            return {
              ...value,
              matches,
              active: matches.length === 0 ? -1 : Math.min(Math.max(value.active, 0), matches.length - 1),
            };
          },
        },
        props: {
          decorations(state) {
            const search = searchPluginKey.getState(state) as SearchState | undefined;
            if (!search || search.matches.length === 0) return DecorationSet.empty;
            return DecorationSet.create(
              state.doc,
              search.matches.map((match, index) =>
                Decoration.inline(match.from, match.to, {
                  class:
                    index === search.active
                      ? "muni-search-match muni-search-match--active"
                      : "muni-search-match",
                }),
              ),
            );
          },
        },
      }),
    ];
  },
});

export function searchState(state: unknown): SearchState {
  const value = searchPluginKey.getState(state as never) as SearchState | undefined;
  return value ?? empty;
}
