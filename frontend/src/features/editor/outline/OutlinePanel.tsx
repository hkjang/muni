import { useCallback, useEffect, useMemo, useState } from "react";
import type { Editor } from "@tiptap/react";
import { Box, IconButton, Stack, Tooltip, Typography } from "@mui/material";
import { ChevronLeft, ListAltOutlined } from "@mui/icons-material";
import { buildOutline, currentOutlineIndex, type RawHeading } from "./outline";

/**
 * OutlinePanel lists the document's headings beside the page.
 *
 * In a long document this is the fastest way to move around, and it doubles as
 * a check on the structure: a section that is hard to name, or a level that
 * jumps, shows up here before a reader ever hits it.
 */
export function OutlinePanel({
  editor,
  onClose,
}: {
  editor: Editor;
  onClose: () => void;
}) {
  const [version, setVersion] = useState(0);

  useEffect(() => {
    // Headings change with the document and the current section changes with
    // the caret, so both kinds of transaction matter.
    const onTransaction = () => setVersion((value) => value + 1);
    editor.on("transaction", onTransaction);
    return () => {
      editor.off("transaction", onTransaction);
    };
  }, [editor]);

  const outline = useMemo(() => {
    const headings: RawHeading[] = [];
    editor.state.doc.descendants((node, pos) => {
      if (node.type.name === "heading") {
        headings.push({
          level: Number(node.attrs.level ?? 1),
          text: node.textContent,
          pos,
        });
        return false;
      }
      return true;
    });
    return buildOutline(headings);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [editor, version]);

  const active = currentOutlineIndex(outline, editor.state.selection.from);

  const goTo = useCallback(
    (pos: number) => {
      editor
        .chain()
        .focus()
        // Inside the heading rather than on it, so the caret is usable the
        // moment the reader lands.
        .setTextSelection(pos + 1)
        .scrollIntoView()
        .run();
    },
    [editor],
  );

  return (
    <Box
      className="muni-no-print"
      sx={{
        width: 250,
        flexShrink: 0,
        borderRight: "1px solid",
        borderColor: "divider",
        bgcolor: "#fff",
        display: "flex",
        flexDirection: "column",
        minHeight: 0,
      }}
    >
      <Stack
        direction="row"
        alignItems="center"
        gap={1}
        sx={{ px: 2, py: 1.5, borderBottom: "1px solid", borderColor: "divider" }}
      >
        <ListAltOutlined fontSize="small" color="disabled" />
        <Typography variant="caption" sx={{ fontWeight: 700, flex: 1 }}>
          문서 개요
        </Typography>
        <Tooltip title="개요 닫기">
          <IconButton size="small" onClick={onClose} aria-label="개요 닫기">
            <ChevronLeft fontSize="small" />
          </IconButton>
        </Tooltip>
      </Stack>
      <Box sx={{ overflowY: "auto", flex: 1, py: 1 }} className="admin-menu-scroll">
        {outline.length === 0 ? (
          <Typography
            variant="body2"
            color="text.secondary"
            sx={{ px: 2, py: 3, lineHeight: 1.7 }}
          >
            제목 스타일을 지정하면 여기에 개요가 만들어집니다. 본문에서
            <br />
            <Box component="span" sx={{ fontFamily: "monospace" }}>
              # 
            </Box>
            과 공백을 입력해도 제목이 됩니다.
          </Typography>
        ) : (
          outline.map((item, index) => (
            <Box
              key={`${item.pos}-${index}`}
              component="button"
              type="button"
              onClick={() => goTo(item.pos)}
              title={item.text}
              sx={{
                display: "block",
                width: "100%",
                textAlign: "left",
                border: 0,
                bgcolor: index === active ? "action.selected" : "transparent",
                borderLeft: "2px solid",
                borderLeftColor: index === active ? "primary.main" : "transparent",
                cursor: "pointer",
                font: "inherit",
                fontSize: 13.5,
                lineHeight: 1.5,
                color: index === active ? "text.primary" : "text.secondary",
                fontWeight: item.depth === 0 ? 650 : 400,
                pl: 2 + item.depth * 1.5,
                pr: 1.5,
                py: 0.7,
                whiteSpace: "nowrap",
                overflow: "hidden",
                textOverflow: "ellipsis",
                "&:hover": { bgcolor: "action.hover" },
              }}
            >
              {item.text}
            </Box>
          ))
        )}
      </Box>
    </Box>
  );
}
