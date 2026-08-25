import type { Editor } from "@tiptap/react";
import { BubbleMenu } from "@tiptap/react/menus";
import { Divider, IconButton, Paper, Stack, Tooltip } from "@mui/material";
import {
  DeleteOutline,
  TableRowsOutlined,
  ViewColumnOutlined,
  ViewWeekOutlined,
  CallMerge,
  CallSplit,
} from "@mui/icons-material";

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
              <TableRowsOutlined fontSize="small" sx={{ transform: "scaleY(-1)" }} />
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
              <ViewColumnOutlined fontSize="small" sx={{ transform: "scaleX(-1)" }} />
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
                size="small"
                disabled={!editor.can().splitCell()}
                onClick={() => editor.chain().focus().splitCell().run()}
              >
                <CallSplit fontSize="small" />
              </IconButton>
            </span>
          </Tooltip>
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
