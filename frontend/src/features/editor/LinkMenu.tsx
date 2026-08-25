import { useEffect, useState } from "react";
import type { Editor } from "@tiptap/react";
import { BubbleMenu } from "@tiptap/react/menus";
import {
  IconButton,
  InputBase,
  Paper,
  Stack,
  Tooltip,
  Typography,
} from "@mui/material";
import {
  CheckOutlined,
  EditOutlined,
  LinkOffOutlined,
  OpenInNew,
} from "@mui/icons-material";

/**
 * LinkMenu is what appears when the caret is inside a link.
 *
 * A link used to be edited through window.prompt, which cannot show where the
 * link already points without pre-filling a box the reader has to read
 * character by character, and looks nothing like the rest of the application.
 */
export function LinkMenu({
  editor,
  canEdit,
}: {
  editor: Editor;
  canEdit: boolean;
}) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const href = (editor.getAttributes("link").href as string | undefined) ?? "";

  useEffect(() => {
    if (!editor.isActive("link")) setEditing(false);
  }, [editor, href]);

  const save = () => {
    const value = normalizeHref(draft);
    if (!value) {
      editor.chain().focus().extendMarkRange("link").unsetLink().run();
    } else {
      editor
        .chain()
        .focus()
        .extendMarkRange("link")
        .setLink({ href: value })
        .run();
    }
    setEditing(false);
  };

  return (
    <BubbleMenu
      editor={editor}
      shouldShow={({ editor: current }) => current.isActive("link")}
      options={{ placement: "bottom", offset: 8 }}
    >
      <Paper
        elevation={6}
        onMouseDown={(event) => event.preventDefault()}
        sx={{
          px: 1,
          py: 0.5,
          borderRadius: 2,
          border: "1px solid",
          borderColor: "divider",
          maxWidth: 420,
        }}
      >
        <Stack direction="row" alignItems="center" gap={0.5}>
          {editing ? (
            <>
              <InputBase
                autoFocus
                value={draft}
                placeholder="https://"
                onChange={(event) => setDraft(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    event.preventDefault();
                    save();
                  }
                  if (event.key === "Escape") setEditing(false);
                }}
                sx={{ width: 260, fontSize: 14 }}
              />
              <Tooltip title="적용">
                <IconButton size="small" onClick={save}>
                  <CheckOutlined fontSize="small" />
                </IconButton>
              </Tooltip>
            </>
          ) : (
            <>
              <Typography
                variant="body2"
                component="a"
                href={href}
                target="_blank"
                rel="noopener noreferrer"
                sx={{
                  maxWidth: 250,
                  overflow: "hidden",
                  textOverflow: "ellipsis",
                  whiteSpace: "nowrap",
                  px: 0.5,
                }}
              >
                {href}
              </Typography>
              <Tooltip title="새 탭에서 열기">
                <IconButton
                  size="small"
                  component="a"
                  href={href}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  <OpenInNew fontSize="small" />
                </IconButton>
              </Tooltip>
              {canEdit && (
                <>
                  <Tooltip title="주소 바꾸기">
                    <IconButton
                      size="small"
                      onClick={() => {
                        setDraft(href);
                        setEditing(true);
                      }}
                    >
                      <EditOutlined fontSize="small" />
                    </IconButton>
                  </Tooltip>
                  <Tooltip title="링크 제거">
                    <IconButton
                      size="small"
                      onClick={() =>
                        editor
                          .chain()
                          .focus()
                          .extendMarkRange("link")
                          .unsetLink()
                          .run()
                      }
                    >
                      <LinkOffOutlined fontSize="small" />
                    </IconButton>
                  </Tooltip>
                </>
              )}
            </>
          )}
        </Stack>
      </Paper>
    </BubbleMenu>
  );
}

/**
 * normalizeHref accepts what people actually type.
 *
 * "example.com" is a link, and only http, https and mailto are followable —
 * anything else is refused rather than written into the document, because a
 * javascript: URL in a shared document is an attack on whoever opens it.
 */
export function normalizeHref(value: string): string {
  const trimmed = value.trim();
  if (!trimmed) return "";
  if (/^(https?:|mailto:)/i.test(trimmed)) return trimmed;
  if (/^[a-z][a-z0-9+.-]*:/i.test(trimmed)) return "";
  if (/^[^\s/@]+@[^\s/@]+\.[^\s/@]+$/.test(trimmed)) return `mailto:${trimmed}`;
  return `https://${trimmed}`;
}
