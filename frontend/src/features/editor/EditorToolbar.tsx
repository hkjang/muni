import type React from "react";
import { useState } from "react";
import { useEditorState, type Editor } from "@tiptap/react";
import {
  Box,
  Divider,
  IconButton,
  ListItemIcon,
  Menu,
  MenuItem,
  Select,
  Stack,
  ToggleButton,
  ToggleButtonGroup,
  Toolbar,
  Tooltip,
} from "@mui/material";
import {
  BorderColor,
  Code,
  FormatAlignCenter,
  FormatAlignJustify,
  FormatAlignLeft,
  FormatAlignRight,
  FormatBold,
  FormatClear,
  FormatColorText,
  FormatLineSpacing,
  FormatIndentDecrease,
  FormatIndentIncrease,
  FormatTextdirectionLToR,
  FormatItalic,
  FormatListBulleted,
  FormatListNumbered,
  FormatQuote,
  FormatUnderlined,
  HorizontalRule,
  ImageOutlined,
  InsertPageBreakOutlined,
  MoreHoriz,
  InsertLink,
  Redo,
  StrikethroughS,
  TableChartOutlined,
  TaskAlt,
  Undo,
} from "@mui/icons-material";
import { api } from "../../lib/api";
import { normalizeHref } from "./LinkMenu";

/**
 * indent moves a list item a level or a paragraph a step, whichever the caret
 * is in — the two are the same gesture to the person pressing the button.
 */
function indent(editor: Editor, direction: 1 | -1) {
  const listType = editor.isActive("taskItem") ? "taskItem" : "listItem";
  if (editor.isActive(listType)) {
    if (direction === 1) editor.chain().focus().sinkListItem(listType).run();
    else editor.chain().focus().liftListItem(listType).run();
    return;
  }
  if (direction === 1) editor.chain().focus().indentParagraph().run();
  else editor.chain().focus().outdentParagraph().run();
}

export function EditorToolbar({
  editor,
  documentId,
}: {
  editor: Editor;
  documentId: string;
}) {
  const state = useEditorState({
    editor,
    selector: ({ editor: current }) => ({
      bold: current.isActive("bold"),
      italic: current.isActive("italic"),
      underline: current.isActive("underline"),
      strike: current.isActive("strike"),
      bullet: current.isActive("bulletList"),
      ordered: current.isActive("orderedList"),
      quote: current.isActive("blockquote"),
      code: current.isActive("codeBlock"),
      align: current.getAttributes("paragraph").textAlign ?? "left",
      lineHeight:
        (current.getAttributes("paragraph").lineHeight as string) ||
        (current.getAttributes("heading").lineHeight as string) ||
        "",
    }),
  });
  // On a phone the toolbar is a long horizontal scroll, and the controls at
  // the end of it are the ones nobody ever reaches. The less-used half moves
  // into a menu there and stays inline on a screen with room for it.
  const [overflow, setOverflow] = useState<HTMLElement | null>(null);
  const link = () => {
    // An existing link is edited in the menu that sits under it; this button
    // is for turning the selection into one.
    if (editor.isActive("link")) {
      editor.chain().focus().extendMarkRange("link").run();
      return;
    }
    const href = normalizeHref(window.prompt("연결할 주소를 입력하세요.", "") ?? "");
    if (!href) return;
    editor.chain().focus().setLink({ href }).run();
  };
  const uploadImage = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;
    const form = new FormData();
    form.set("file", file);
    try {
      const result = await api<{ url: string }>(
        `/api/v1/documents/${documentId}/attachments`,
        { method: "POST", body: form },
      );
      editor
        .chain()
        .focus()
        .setImage({ src: result.url, alt: file.name })
        .run();
    } finally {
      event.target.value = "";
    }
  };
  return (
    <Toolbar
      variant="dense"
      sx={{
        minHeight: "52px!important",
        gap: 0.5,
        width: "max-content",
        minWidth: "100%",
        px: { xs: 1, sm: 2 },
      }}
    >
      <Tooltip title="실행 취소">
        <span>
          <IconButton
            disabled={!editor.can().undo()}
            onClick={() => editor.chain().focus().undo().run()}
          >
            <Undo />
          </IconButton>
        </span>
      </Tooltip>
      <Tooltip title="다시 실행">
        <span>
          <IconButton
            disabled={!editor.can().redo()}
            onClick={() => editor.chain().focus().redo().run()}
          >
            <Redo />
          </IconButton>
        </span>
      </Tooltip>
      <Divider flexItem orientation="vertical" sx={{ mx: 0.5 }} />
      <Select
        size="small"
        value={
          editor.isActive("heading", { level: 1 })
            ? "h1"
            : editor.isActive("heading", { level: 2 })
              ? "h2"
              : editor.isActive("heading", { level: 3 })
                ? "h3"
                : editor.isActive("heading", { level: 4 })
                  ? "h4"
                  : editor.isActive("heading", { level: 5 })
                    ? "h5"
                    : editor.isActive("heading", { level: 6 })
                      ? "h6"
                      : "p"
        }
        onChange={(event) => {
          const value = event.target.value;
          if (value === "p") editor.chain().focus().setParagraph().run();
          else
            editor
              .chain()
              .focus()
              .toggleHeading({
                level: Number(value.slice(1)) as 1 | 2 | 3 | 4 | 5 | 6,
              })
              .run();
        }}
        sx={{ minWidth: 108 }}
      >
        <MenuItem value="p">본문</MenuItem>
        <MenuItem value="h1">제목 1</MenuItem>
        <MenuItem value="h2">제목 2</MenuItem>
        <MenuItem value="h3">제목 3</MenuItem>
        <MenuItem value="h4">제목 4</MenuItem>
        <MenuItem value="h5">제목 5</MenuItem>
        <MenuItem value="h6">제목 6</MenuItem>
      </Select>
      <Select
        size="small"
        value={editor.getAttributes("textStyle").fontSize ?? "17px"}
        onChange={(event) =>
          editor.chain().focus().setFontSize(event.target.value).run()
        }
        sx={{ minWidth: 84 }}
      >
        {["14px", "16px", "17px", "18px", "20px", "24px", "32px"].map(
          (value) => (
            <MenuItem key={value} value={value}>
              {value.replace("px", "")}
            </MenuItem>
          ),
        )}
      </Select>
      <Select
        size="small"
        value={editor.getAttributes("textStyle").fontFamily ?? "Noto Sans KR"}
        onChange={(event) =>
          editor.chain().focus().setFontFamily(event.target.value).run()
        }
        sx={{ minWidth: 130 }}
      >
        <MenuItem value="Noto Sans KR">Noto Sans KR</MenuItem>
        <MenuItem value="serif">명조 계열</MenuItem>
        <MenuItem value="monospace">고정폭</MenuItem>
      </Select>
      <Tooltip title="글자 색">
        <IconButton component="label" aria-label="글자 색">
          <FormatColorText />
          <input
            hidden
            type="color"
            onChange={(event) =>
              editor.chain().focus().setColor(event.target.value).run()
            }
          />
        </IconButton>
      </Tooltip>
      <Tooltip title="강조 색">
        <IconButton component="label" aria-label="강조 색">
          <BorderColor />
          <input
            hidden
            type="color"
            defaultValue="#fff59d"
            onChange={(event) =>
              editor
                .chain()
                .focus()
                .toggleHighlight({ color: event.target.value })
                .run()
            }
          />
        </IconButton>
      </Tooltip>
      <ToggleButtonGroup size="small">
        <ToggleButton
          value="bold"
          selected={state.bold}
          onClick={() => editor.chain().focus().toggleBold().run()}
        >
          <FormatBold />
        </ToggleButton>
        <ToggleButton
          value="italic"
          selected={state.italic}
          onClick={() => editor.chain().focus().toggleItalic().run()}
        >
          <FormatItalic />
        </ToggleButton>
        <ToggleButton
          value="underline"
          selected={state.underline}
          onClick={() => editor.chain().focus().toggleUnderline().run()}
        >
          <FormatUnderlined />
        </ToggleButton>
        <ToggleButton
          value="strike"
          selected={state.strike}
          onClick={() => editor.chain().focus().toggleStrike().run()}
        >
          <StrikethroughS />
        </ToggleButton>
      </ToggleButtonGroup>
      <ToggleButtonGroup
        size="small"
        value={state.align}
        exclusive
        onChange={(_, value) =>
          value && editor.chain().focus().setTextAlign(value).run()
        }
      >
        <ToggleButton value="left">
          <FormatAlignLeft />
        </ToggleButton>
        <ToggleButton value="center">
          <FormatAlignCenter />
        </ToggleButton>
        <ToggleButton value="right">
          <FormatAlignRight />
        </ToggleButton>
        <ToggleButton value="justify">
          <FormatAlignJustify />
        </ToggleButton>
      </ToggleButtonGroup>
      <ToggleButtonGroup size="small">
        <ToggleButton
          value="bullet"
          selected={state.bullet}
          onClick={() => editor.chain().focus().toggleBulletList().run()}
        >
          <FormatListBulleted />
        </ToggleButton>
        <ToggleButton
          value="ordered"
          selected={state.ordered}
          onClick={() => editor.chain().focus().toggleOrderedList().run()}
        >
          <FormatListNumbered />
        </ToggleButton>
        <ToggleButton
          value="task"
          selected={editor.isActive("taskList")}
          onClick={() => editor.chain().focus().toggleTaskList().run()}
        >
          <TaskAlt />
        </ToggleButton>
        <ToggleButton
          value="quote"
          selected={state.quote}
          onClick={() => editor.chain().focus().toggleBlockquote().run()}
        >
          <FormatQuote />
        </ToggleButton>
        <ToggleButton
          value="code"
          selected={state.code}
          onClick={() => editor.chain().focus().toggleCodeBlock().run()}
        >
          <Code />
        </ToggleButton>
      </ToggleButtonGroup>
      <Box
        sx={{ display: { xs: "none", md: "flex" }, alignItems: "center", gap: 0.5 }}
      >
      <Tooltip title="들여쓰기 (Tab)">
        <IconButton onClick={() => indent(editor, 1)}>
          <FormatIndentIncrease />
        </IconButton>
      </Tooltip>
      <Tooltip title="내어쓰기 (Shift+Tab)">
        <IconButton onClick={() => indent(editor, -1)}>
          <FormatIndentDecrease />
        </IconButton>
      </Tooltip>
      <Tooltip title="첫 줄 들여쓰기">
        <IconButton
          onClick={() => editor.chain().focus().toggleFirstLineIndent().run()}
        >
          <FormatTextdirectionLToR />
        </IconButton>
      </Tooltip>
      <Tooltip title="링크">
        <IconButton onClick={link}>
          <InsertLink />
        </IconButton>
      </Tooltip>
      <Tooltip title="이미지 업로드">
        <IconButton component="label">
          <ImageOutlined />
          <input hidden type="file" accept="image/*" onChange={uploadImage} />
        </IconButton>
      </Tooltip>
      <Tooltip title="표 삽입">
        <IconButton
          onClick={() =>
            editor
              .chain()
              .focus()
              .insertTable({ rows: 3, cols: 3, withHeaderRow: true })
              .run()
          }
        >
          <TableChartOutlined />
        </IconButton>
      </Tooltip>
      <Tooltip title="페이지 나누기 (Ctrl+Enter)">
        <IconButton onClick={() => editor.chain().focus().setPageBreak().run()}>
          <InsertPageBreakOutlined />
        </IconButton>
      </Tooltip>
      <Tooltip title="가로 구분선">
        <IconButton
          onClick={() => editor.chain().focus().setHorizontalRule().run()}
        >
          <HorizontalRule />
        </IconButton>
      </Tooltip>
      <Select
        size="small"
        displayEmpty
        value={state.lineHeight}
        aria-label="줄 간격"
        renderValue={(value) => (
          <Stack direction="row" alignItems="center" gap={0.5}>
            <FormatLineSpacing fontSize="small" />
            {value ? String(value) : "기본"}
          </Stack>
        )}
        onChange={(event) => {
          const value = event.target.value;
          if (value) editor.chain().focus().setLineHeight(value).run();
          else editor.chain().focus().unsetLineHeight().run();
        }}
        sx={{ minWidth: 104 }}
      >
        <MenuItem value="">기본</MenuItem>
        {["1", "1.15", "1.5", "1.75", "2", "2.5"].map((value) => (
          <MenuItem key={value} value={value}>
            {value}
          </MenuItem>
        ))}
      </Select>
      <Tooltip title="서식 지우기">
        <IconButton
          onClick={() =>
            editor.chain().focus().unsetAllMarks().clearNodes().run()
          }
        >
          <FormatClear />
        </IconButton>
      </Tooltip>
      </Box>
      <Tooltip title="더 보기">
        <IconButton
          aria-label="더 보기"
          sx={{ display: { xs: "inline-flex", md: "none" } }}
          onClick={(event) => setOverflow(event.currentTarget)}
        >
          <MoreHoriz />
        </IconButton>
      </Tooltip>
      <Menu
        anchorEl={overflow}
        open={Boolean(overflow)}
        onClose={() => setOverflow(null)}
      >
        {[
          { label: "들여쓰기", icon: <FormatIndentIncrease />, run: () => indent(editor, 1) },
          { label: "내어쓰기", icon: <FormatIndentDecrease />, run: () => indent(editor, -1) },
          {
            label: "첫 줄 들여쓰기",
            icon: <FormatTextdirectionLToR />,
            run: () => editor.chain().focus().toggleFirstLineIndent().run(),
          },
          { label: "링크", icon: <InsertLink />, run: link },
          {
            label: "표 삽입",
            icon: <TableChartOutlined />,
            run: () =>
              editor
                .chain()
                .focus()
                .insertTable({ rows: 3, cols: 3, withHeaderRow: true })
                .run(),
          },
          {
            label: "페이지 나누기",
            icon: <InsertPageBreakOutlined />,
            run: () => editor.chain().focus().setPageBreak().run(),
          },
          {
            label: "가로 구분선",
            icon: <HorizontalRule />,
            run: () => editor.chain().focus().setHorizontalRule().run(),
          },
          {
            label: "서식 지우기",
            icon: <FormatClear />,
            run: () => editor.chain().focus().unsetAllMarks().clearNodes().run(),
          },
        ].map((item) => (
          <MenuItem
            key={item.label}
            onClick={() => {
              item.run();
              setOverflow(null);
            }}
          >
            <ListItemIcon>{item.icon}</ListItemIcon>
            {item.label}
          </MenuItem>
        ))}
      </Menu>
    </Toolbar>
  );
}
