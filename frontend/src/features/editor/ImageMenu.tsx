import { useState } from "react";
import type { Editor } from "@tiptap/react";
import { BubbleMenu } from "@tiptap/react/menus";
import {
  Divider,
  IconButton,
  InputBase,
  Paper,
  Stack,
  ToggleButton,
  ToggleButtonGroup,
  Tooltip,
} from "@mui/material";
import {
  DeleteOutline,
  FormatAlignCenter,
  FormatAlignLeft,
  FormatAlignRight,
  TextFieldsOutlined,
} from "@mui/icons-material";
import { percentFor, pixelsFor, widthPresets } from "./extensions/imageAttributes";

/**
 * ImageMenu appears while an image is selected.
 *
 * An image used to go in at whatever size it happened to be and always sat on
 * the left, which meant an editor had no way to lay out a page — a screenshot
 * arrived full width and stayed there.
 */
export function ImageMenu({
  editor,
  canEdit,
}: {
  editor: Editor;
  canEdit: boolean;
}) {
  const [alt, setAlt] = useState<string | null>(null);
  if (!canEdit) return null;

  const attributes = editor.getAttributes("image") as {
    width?: number | null;
    textAlign?: string | null;
    alt?: string | null;
  };
  const percent = percentFor(attributes.width);

  const setWidth = (value: number) =>
    editor.chain().focus().updateAttributes("image", { width: pixelsFor(value) }).run();

  return (
    <BubbleMenu
      editor={editor}
      shouldShow={({ editor: current }) =>
        current.isEditable && current.isActive("image")
      }
      options={{ placement: "top", offset: 10 }}
    >
      <Paper
        elevation={6}
        onMouseDown={(event) => event.preventDefault()}
        sx={{
          px: 0.75,
          py: 0.4,
          borderRadius: 2,
          border: "1px solid",
          borderColor: "divider",
        }}
      >
        <Stack direction="row" alignItems="center" gap={0.5}>
          {alt === null ? (
            <>
              <ToggleButtonGroup size="small" exclusive value={percent}>
                {widthPresets.map((preset) => (
                  <ToggleButton
                    key={preset}
                    value={preset}
                    onClick={() => setWidth(preset)}
                    sx={{ px: 1, py: 0.3, fontSize: 12.5 }}
                  >
                    {preset}%
                  </ToggleButton>
                ))}
              </ToggleButtonGroup>
              <Divider flexItem orientation="vertical" sx={{ mx: 0.3, my: 0.5 }} />
              <ToggleButtonGroup
                size="small"
                exclusive
                value={attributes.textAlign ?? "left"}
                onChange={(_, value) =>
                  value &&
                  editor
                    .chain()
                    .focus()
                    .updateAttributes("image", { textAlign: value })
                    .run()
                }
              >
                <ToggleButton value="left" sx={{ px: 0.8, py: 0.3 }}>
                  <FormatAlignLeft fontSize="small" />
                </ToggleButton>
                <ToggleButton value="center" sx={{ px: 0.8, py: 0.3 }}>
                  <FormatAlignCenter fontSize="small" />
                </ToggleButton>
                <ToggleButton value="right" sx={{ px: 0.8, py: 0.3 }}>
                  <FormatAlignRight fontSize="small" />
                </ToggleButton>
              </ToggleButtonGroup>
              <Divider flexItem orientation="vertical" sx={{ mx: 0.3, my: 0.5 }} />
              <Tooltip title="대체 텍스트">
                <IconButton size="small" onClick={() => setAlt(attributes.alt ?? "")}>
                  <TextFieldsOutlined fontSize="small" />
                </IconButton>
              </Tooltip>
              <Tooltip title="이미지 삭제">
                <IconButton
                  size="small"
                  color="error"
                  onClick={() => editor.chain().focus().deleteSelection().run()}
                >
                  <DeleteOutline fontSize="small" />
                </IconButton>
              </Tooltip>
            </>
          ) : (
            <InputBase
              autoFocus
              value={alt}
              placeholder="화면 낭독기가 읽을 설명"
              onChange={(event) => setAlt(event.target.value)}
              onBlur={() => {
                editor.chain().focus().updateAttributes("image", { alt }).run();
                setAlt(null);
              }}
              onKeyDown={(event) => {
                if (event.key === "Enter") {
                  event.preventDefault();
                  editor.chain().focus().updateAttributes("image", { alt }).run();
                  setAlt(null);
                }
                if (event.key === "Escape") setAlt(null);
              }}
              sx={{ width: 280, fontSize: 14, px: 0.5 }}
            />
          )}
        </Stack>
      </Paper>
    </BubbleMenu>
  );
}
