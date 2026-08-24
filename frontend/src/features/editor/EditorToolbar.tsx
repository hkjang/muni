import type React from "react";
import { useEditorState, type Editor } from "@tiptap/react";
import {
  Divider,
  IconButton,
  MenuItem,
  Select,
  ToggleButton,
  ToggleButtonGroup,
  Toolbar,
  Tooltip,
} from "@mui/material";
import {
  BorderColor,
  Code,
  FormatAlignCenter,
  FormatAlignLeft,
  FormatAlignRight,
  FormatBold,
  FormatColorText,
  FormatIndentDecrease,
  FormatIndentIncrease,
  FormatItalic,
  FormatListBulleted,
  FormatListNumbered,
  FormatQuote,
  FormatUnderlined,
  HorizontalRule,
  ImageOutlined,
  InsertLink,
  Redo,
  StrikethroughS,
  TableChartOutlined,
  TaskAlt,
  Undo,
} from "@mui/icons-material";
import { api } from "../../lib/api";

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
    }),
  });
  const link = () => {
    const previous = editor.getAttributes("link").href as string | undefined;
    const href = window.prompt(
      "연결할 URL을 입력하세요.",
      previous ?? "https://",
    );
    if (href === null) return;
    if (!href) {
      editor.chain().focus().extendMarkRange("link").unsetLink().run();
      return;
    }
    editor.chain().focus().extendMarkRange("link").setLink({ href }).run();
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
      <Tooltip title="들여쓰기">
        <span>
          <IconButton
            disabled={
              !editor
                .can()
                .sinkListItem(
                  editor.isActive("taskItem") ? "taskItem" : "listItem",
                )
            }
            onClick={() =>
              editor
                .chain()
                .focus()
                .sinkListItem(
                  editor.isActive("taskItem") ? "taskItem" : "listItem",
                )
                .run()
            }
          >
            <FormatIndentIncrease />
          </IconButton>
        </span>
      </Tooltip>
      <Tooltip title="내어쓰기">
        <span>
          <IconButton
            disabled={
              !editor
                .can()
                .liftListItem(
                  editor.isActive("taskItem") ? "taskItem" : "listItem",
                )
            }
            onClick={() =>
              editor
                .chain()
                .focus()
                .liftListItem(
                  editor.isActive("taskItem") ? "taskItem" : "listItem",
                )
                .run()
            }
          >
            <FormatIndentDecrease />
          </IconButton>
        </span>
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
      <Tooltip title="가로 구분선">
        <IconButton
          onClick={() => editor.chain().focus().setHorizontalRule().run()}
        >
          <HorizontalRule />
        </IconButton>
      </Tooltip>
    </Toolbar>
  );
}
