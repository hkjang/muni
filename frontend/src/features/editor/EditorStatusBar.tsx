import { useEffect, useMemo, useState } from "react";
import type { Editor } from "@tiptap/react";
import { IconButton, Stack, Tooltip, Typography } from "@mui/material";
import {
  KeyboardOutlined,
  ListAltOutlined,
  RemoveOutlined,
  AddOutlined,
} from "@mui/icons-material";
import { documentStats } from "./stats/documentStats";

/**
 * EditorStatusBar sits under the page and answers the two questions people ask
 * about a document they are writing to a limit: how long is it, and how long
 * does it take to read.
 *
 * When something is selected the counts are for the selection, which is what
 * makes it useful for trimming a section down to size.
 */
export function EditorStatusBar({
  editor,
  zoom,
  onZoom,
  outlineOpen,
  onToggleOutline,
  onShortcuts,
}: {
  editor: Editor;
  zoom: number;
  onZoom: (value: number) => void;
  outlineOpen: boolean;
  onToggleOutline: () => void;
  onShortcuts: () => void;
}) {
  const [version, setVersion] = useState(0);
  useEffect(() => {
    const onTransaction = () => setVersion((value) => value + 1);
    editor.on("transaction", onTransaction);
    return () => {
      editor.off("transaction", onTransaction);
    };
  }, [editor]);

  const { stats, selected } = useMemo(() => {
    const { from, to, empty } = editor.state.selection;
    const text = empty
      ? editor.state.doc.textBetween(0, editor.state.doc.content.size, "\n", " ")
      : editor.state.doc.textBetween(from, to, "\n", " ");
    return { stats: documentStats(text), selected: !empty };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [editor, version]);

  return (
    <Stack
      direction="row"
      alignItems="center"
      gap={1.5}
      className="muni-no-print"
      sx={{
        px: 2,
        py: 0.5,
        borderTop: "1px solid",
        borderColor: "divider",
        bgcolor: "#fff",
        minHeight: 36,
      }}
    >
      <Tooltip title={outlineOpen ? "문서 개요 닫기" : "문서 개요 열기"}>
        <IconButton size="small" onClick={onToggleOutline} aria-label="문서 개요">
          <ListAltOutlined
            fontSize="small"
            color={outlineOpen ? "primary" : "disabled"}
          />
        </IconButton>
      </Tooltip>
      <Typography variant="caption" color="text.secondary">
        {selected ? "선택 " : ""}
        {stats.words.toLocaleString()}단어 · {stats.characters.toLocaleString()}자
        {!selected && stats.readingMinutes > 0
          ? ` · 읽는 데 약 ${stats.readingMinutes}분`
          : ""}
      </Typography>
      <Tooltip title="공백 제외">
        <Typography variant="caption" color="text.disabled">
          ({stats.charactersNoSpaces.toLocaleString()}자)
        </Typography>
      </Tooltip>
      <div style={{ flex: 1 }} />
      <Tooltip title="단축키 (Ctrl+/)">
        <IconButton size="small" onClick={onShortcuts} aria-label="단축키">
          <KeyboardOutlined fontSize="small" color="disabled" />
        </IconButton>
      </Tooltip>
      <Stack direction="row" alignItems="center">
        <Tooltip title="축소">
          <span>
            <IconButton
              size="small"
              aria-label="축소"
              disabled={zoom <= 50}
              onClick={() => onZoom(Math.max(50, zoom - 10))}
            >
              <RemoveOutlined fontSize="small" />
            </IconButton>
          </span>
        </Tooltip>
        <Typography
          variant="caption"
          color="text.secondary"
          sx={{ width: 42, textAlign: "center", cursor: "pointer" }}
          onClick={() => onZoom(100)}
          title="100%로 되돌리기"
        >
          {zoom}%
        </Typography>
        <Tooltip title="확대">
          <span>
            <IconButton
              size="small"
              aria-label="확대"
              disabled={zoom >= 200}
              onClick={() => onZoom(Math.min(200, zoom + 10))}
            >
              <AddOutlined fontSize="small" />
            </IconButton>
          </span>
        </Tooltip>
      </Stack>
    </Stack>
  );
}
