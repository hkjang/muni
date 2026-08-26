import { Node } from "@tiptap/core";
import {
  NodeViewWrapper,
  ReactNodeViewRenderer,
  useEditorState,
  type NodeViewProps,
} from "@tiptap/react";
import { Box, Typography } from "@mui/material";
import { buildOutline, type RawHeading } from "../outline/outline";

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    muniTableOfContents: {
      insertTableOfContents: () => ReturnType;
    };
  }
}

/**
 * A contents list inside the document.
 *
 * The outline panel is for moving around while writing; this is for the reader
 * of the finished thing — the page of contents a report is expected to open
 * with, and the one part of it nobody wants to keep in step by hand.
 *
 * Nothing is stored inside the node. The entries are worked out from the
 * headings every time it is drawn, and again when the document is exported, so
 * a contents list can never disagree with the document it belongs to.
 */
export const TableOfContentsNode = Node.create({
  name: "tableOfContents",
  group: "block",
  atom: true,
  selectable: true,

  parseHTML() {
    return [{ tag: "div[data-table-of-contents]" }];
  },

  renderHTML() {
    return ["div", { "data-table-of-contents": "true" }];
  },

  addNodeView() {
    return ReactNodeViewRenderer(TableOfContentsView);
  },

  addCommands() {
    return {
      insertTableOfContents:
        () =>
        ({ chain }) =>
          chain().insertContent({ type: this.name }).run(),
    };
  },
});

function TableOfContentsView({ editor, selected }: NodeViewProps) {
  const outline = useEditorState({
    editor,
    selector: ({ editor: current }) => {
      const headings: RawHeading[] = [];
      current.state.doc.descendants((node, pos) => {
        if (node.type.name !== "heading") return true;
        headings.push({
          level: Number(node.attrs.level ?? 1),
          text: node.textContent,
          pos,
        });
        return false;
      });
      return buildOutline(headings);
    },
    // The list only changes when a heading does, so the comparison keeps the
    // node from redrawing on every keystroke elsewhere in the document.
    equalityFn: (previous, next) =>
      Boolean(previous) &&
      Boolean(next) &&
      previous.length === next!.length &&
      previous.every(
        (item, index) =>
          item.text === next![index]?.text && item.depth === next![index]?.depth,
      ),
  });

  return (
    <NodeViewWrapper>
      <Box
        contentEditable={false}
        sx={{
          my: 2,
          py: 1.5,
          px: 2,
          borderLeft: "3px solid",
          borderColor: selected ? "primary.main" : "divider",
          bgcolor: "action.hover",
          borderRadius: 1,
        }}
      >
        <Typography
          variant="caption"
          sx={{ fontWeight: 700, color: "text.secondary", letterSpacing: ".04em" }}
        >
          목차
        </Typography>
        {outline.length === 0 ? (
          <Typography variant="body2" color="text.secondary" mt={0.75}>
            제목 스타일을 지정하면 여기에 목차가 만들어집니다.
          </Typography>
        ) : (
          <Box mt={0.75}>
            {outline.map((item, index) => (
              <Box
                key={`${item.pos}-${index}`}
                onClick={() =>
                  editor.chain().focus().setTextSelection(item.pos + 1).scrollIntoView().run()
                }
                sx={{
                  pl: item.depth * 2,
                  py: 0.25,
                  fontSize: 14.5,
                  cursor: "pointer",
                  color: item.depth === 0 ? "text.primary" : "text.secondary",
                  fontWeight: item.depth === 0 ? 600 : 400,
                  "&:hover": { textDecoration: "underline" },
                }}
              >
                {item.text}
              </Box>
            ))}
          </Box>
        )}
      </Box>
    </NodeViewWrapper>
  );
}
