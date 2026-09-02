import type { Editor } from "@tiptap/react";
import { BubbleMenu } from "@tiptap/react/menus";
import { Box, Divider, IconButton, Paper, Stack, Tooltip } from "@mui/material";
import {
  DeleteOutline,
  TableRowsOutlined,
  ViewColumnOutlined,
  ViewWeekOutlined,
  CallMerge,
  CallSplit,
} from "@mui/icons-material";
import { cellShades, normalizeShade } from "./extensions/cellBackground";
import {
  cellAlignments,
  normalizeAlignment,
  type CellAlignment,
} from "./extensions/cellAlign";
import VerticalAlignTop from "@mui/icons-material/VerticalAlignTop";
import VerticalAlignCenter from "@mui/icons-material/VerticalAlignCenter";
import VerticalAlignBottom from "@mui/icons-material/VerticalAlignBottom";

// Where a cell's text sits between the top and the bottom of its row. muni
// read this out of Word and Hangul files and wrote it back, and had no way to
// set it — a document muni did not write kept its shape, and one muni did
// could not be given one.
const alignmentIcons: Record<CellAlignment, typeof VerticalAlignTop> = {
  top: VerticalAlignTop,
  middle: VerticalAlignCenter,
  bottom: VerticalAlignBottom,
};
const alignmentLabels: Record<CellAlignment, string> = {
  top: "위",
  middle: "가운데",
  bottom: "아래",
};

/**
 * TableTools appears while the caret is inside a table.
 *
 * Inserting a table without being able to add a row to it is not much of a
 * table, and burying row and column commands in the main toolbar would mean
 * carrying eight buttons that are disabled almost all the time.
 */
export function TableTools({
  editor,
  canEdit,
}: {
  editor: Editor;
  canEdit: boolean;
}) {
  if (!canEdit) return null;
  return (
    <BubbleMenu
      editor={editor}
      shouldShow={({ editor: current }) =>
        current.isEditable && current.isActive("table")
      }
      options={{ placement: "top", offset: 10 }}
    >
      <Paper
        elevation={6}
        onMouseDown={(event) => event.preventDefault()}
        sx={{
          px: 0.5,
          py: 0.25,
          borderRadius: 2,
          border: "1px solid",
          borderColor: "divider",
        }}
      >
        <Stack direction="row" alignItems="center" gap={0.25}>
          <Tooltip title="위에 행 추가">
            <IconButton
              size="small"
              onClick={() => editor.chain().focus().addRowBefore().run()}
            >
              <TableRowsOutlined
                fontSize="small"
                sx={{ transform: "scaleY(-1)" }}
              />
            </IconButton>
          </Tooltip>
          <Tooltip title="아래에 행 추가">
            <IconButton
              size="small"
              onClick={() => editor.chain().focus().addRowAfter().run()}
            >
              <TableRowsOutlined fontSize="small" />
            </IconButton>
          </Tooltip>
          <Tooltip title="행 삭제">
            <IconButton
              size="small"
              onClick={() => editor.chain().focus().deleteRow().run()}
            >
              <TableRowsOutlined fontSize="small" color="error" />
            </IconButton>
          </Tooltip>
          <Divider flexItem orientation="vertical" sx={{ mx: 0.4, my: 0.5 }} />
          <Tooltip title="왼쪽에 열 추가">
            <IconButton
              size="small"
              onClick={() => editor.chain().focus().addColumnBefore().run()}
            >
              <ViewColumnOutlined
                fontSize="small"
                sx={{ transform: "scaleX(-1)" }}
              />
            </IconButton>
          </Tooltip>
          <Tooltip title="오른쪽에 열 추가">
            <IconButton
              size="small"
              onClick={() => editor.chain().focus().addColumnAfter().run()}
            >
              <ViewColumnOutlined fontSize="small" />
            </IconButton>
          </Tooltip>
          <Tooltip title="열 삭제">
            <IconButton
              size="small"
              onClick={() => editor.chain().focus().deleteColumn().run()}
            >
              <ViewColumnOutlined fontSize="small" color="error" />
            </IconButton>
          </Tooltip>
          <Divider flexItem orientation="vertical" sx={{ mx: 0.4, my: 0.5 }} />
          <Tooltip title="헤더 행 전환">
            <IconButton
              size="small"
              onClick={() => editor.chain().focus().toggleHeaderRow().run()}
            >
              <ViewWeekOutlined fontSize="small" />
            </IconButton>
          </Tooltip>
          <Tooltip title="셀 병합">
            <span>
              <IconButton
                aria-label="셀 병합"
                size="small"
                disabled={!editor.can().mergeCells()}
                onClick={() => editor.chain().focus().mergeCells().run()}
              >
                <CallMerge fontSize="small" />
              </IconButton>
            </span>
          </Tooltip>
          <Tooltip title="셀 분할">
            <span>
              <IconButton
                aria-label="셀 분할"
                size="small"
                disabled={!editor.can().splitCell()}
                onClick={() => editor.chain().focus().splitCell().run()}
              >
                <CallSplit fontSize="small" />
              </IconButton>
            </span>
          </Tooltip>
          <Divider flexItem orientation="vertical" sx={{ mx: 0.4, my: 0.5 }} />
          {cellAlignments.map((alignment) => {
            const current = normalizeAlignment(
              editor.getAttributes("tableCell").verticalAlign ??
                editor.getAttributes("tableHeader").verticalAlign,
            );
            const Icon = alignmentIcons[alignment];
            return (
              <Tooltip
                key={alignment}
                title={`세로 정렬 ${alignmentLabels[alignment]}`}
              >
                <span>
                  <IconButton
                    aria-label={`세로 정렬 ${alignmentLabels[alignment]}`}
                    size="small"
                    color={current === alignment ? "primary" : "default"}
                    onClick={() =>
                      editor
                        .chain()
                        .focus()
                        .updateAttributes("tableCell", {
                          verticalAlign: alignment,
                        })
                        .updateAttributes("tableHeader", {
                          verticalAlign: alignment,
                        })
                        .run()
                    }
                  >
                    <Icon fontSize="small" />
                  </IconButton>
                </span>
              </Tooltip>
            );
          })}
          <Divider flexItem orientation="vertical" sx={{ mx: 0.4, my: 0.5 }} />
          {cellShades.map((shade) => {
            const current = normalizeShade(
              editor.getAttributes("tableCell").backgroundColor ??
                editor.getAttributes("tableHeader").backgroundColor,
            );
            const selected = current === shade.value;
            return (
              <Tooltip key={shade.label} title={`셀 배경 ${shade.label}`}>
                <Box
                  component="button"
                  type="button"
                  aria-label={`셀 배경 ${shade.label}`}
                  onClick={() =>
                    editor
                      .chain()
                      .focus()
                      .updateAttributes("tableCell", {
                        backgroundColor: shade.value || null,
                      })
                      .updateAttributes("tableHeader", {
                        backgroundColor: shade.value || null,
                      })
                      .run()
                  }
                  sx={{
                    width: 20,
                    height: 20,
                    p: 0,
                    mx: 0.15,
                    borderRadius: "50%",
                    cursor: "pointer",
                    bgcolor: shade.value || "#fff",
                    border: "1px solid",
                    borderColor: selected ? "primary.main" : "divider",
                    boxShadow: selected
                      ? "0 0 0 2px rgba(81,81,198,.25)"
                      : "none",
                    // The "없음" swatch reads as a crossed-out circle.
                    backgroundImage: shade.value
                      ? "none"
                      : "linear-gradient(45deg,transparent 45%,#c9ccd8 45%,#c9ccd8 55%,transparent 55%)",
                  }}
                />
              </Tooltip>
            );
          })}
          <Divider flexItem orientation="vertical" sx={{ mx: 0.4, my: 0.5 }} />
          <Tooltip title="표 삭제">
            <IconButton
              size="small"
              color="error"
              onClick={() => editor.chain().focus().deleteTable().run()}
            >
              <DeleteOutline fontSize="small" />
            </IconButton>
          </Tooltip>
        </Stack>
      </Paper>
    </BubbleMenu>
  );
}
