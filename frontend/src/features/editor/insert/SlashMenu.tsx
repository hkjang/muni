import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { Editor } from "@tiptap/react";
import {
  Box,
  ListSubheader,
  Paper,
  Popper,
  Stack,
  Typography,
} from "@mui/material";
import { api } from "../../../lib/api";
import {
  groupInsertCommands,
  insertCommands,
  matchInsertCommands,
  readSlashQuery,
  todayLabel,
  type InsertCommand,
} from "./slashCommands";

type Trigger = { from: number; to: number; query: string };

/**
 * SlashMenu opens when a line starts with "/" and inserts what is chosen.
 *
 * The toolbar cannot hold everything and reaching it means leaving the
 * keyboard. Typing the name of what you want is the fastest path there is, and
 * it is where anyone who has used Google Docs or Notion will look first.
 */
export function SlashMenu({
  editor,
  documentId,
  canEdit,
}: {
  editor: Editor;
  documentId: string;
  canEdit: boolean;
}) {
  const [trigger, setTrigger] = useState<Trigger | null>(null);
  const [active, setActive] = useState(0);
  const [anchor, setAnchor] = useState<{ top: number; left: number } | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const pendingImage = useRef<Trigger | null>(null);

  const matches = useMemo(
    () => matchInsertCommands(insertCommands, trigger?.query ?? ""),
    [trigger?.query],
  );
  const grouped = useMemo(() => groupInsertCommands(matches), [matches]);

  const close = useCallback(() => {
    setTrigger(null);
    setActive(0);
    setAnchor(null);
  }, []);

  // The trigger is read from the document rather than from keystrokes, so it
  // stays correct when text is pasted, deleted or edited from the middle.
  useEffect(() => {
    if (!canEdit) return;
    const onTransaction = () => {
      const { selection } = editor.state;
      if (!selection.empty || !editor.isEditable) return close();
      const { $from } = selection;
      if ($from.parent.type.name === "codeBlock") return close();
      const textBefore = $from.parent.textBetween(
        0,
        $from.parentOffset,
        "\n",
        "\n",
      );
      const found = readSlashQuery(textBefore);
      if (!found) return close();
      const from = selection.from - found.length;
      setTrigger({ from, to: selection.from, query: found.query });
      setActive(0);
      try {
        const coords = editor.view.coordsAtPos(from);
        setAnchor({ top: coords.bottom, left: coords.left });
      } catch {
        /* The position has already gone; the next transaction will fix it. */
      }
    };
    editor.on("transaction", onTransaction);
    return () => {
      editor.off("transaction", onTransaction);
    };
  }, [canEdit, close, editor]);

  const run = useCallback(
    (command: InsertCommand, at: Trigger) => {
      const chain = editor.chain().focus().deleteRange({ from: at.from, to: at.to });
      switch (command.id) {
        case "h1":
        case "h2":
        case "h3":
          chain.setNode("heading", { level: Number(command.id.slice(1)) }).run();
          break;
        case "paragraph":
          chain.setParagraph().run();
          break;
        case "bulletList":
          chain.toggleBulletList().run();
          break;
        case "orderedList":
          chain.toggleOrderedList().run();
          break;
        case "taskList":
          chain.toggleTaskList().run();
          break;
        case "blockquote":
          chain.toggleBlockquote().run();
          break;
        case "codeBlock":
          chain.toggleCodeBlock().run();
          break;
        case "table":
          chain.insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run();
          break;
        case "horizontalRule":
          chain.setHorizontalRule().run();
          break;
        case "pageBreak":
          chain.setPageBreak().run();
          break;
        case "tableOfContents":
          chain.insertTableOfContents().run();
          break;
        case "date":
          chain.insertContent(todayLabel(new Date())).run();
          break;
        case "image":
          // The range is removed now so the picker does not leave "/이미지"
          // behind if it is cancelled.
          chain.run();
          pendingImage.current = at;
          fileInput.current?.click();
          break;
      }
      close();
    },
    [close, editor],
  );

  const uploadImage = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    pendingImage.current = null;
    if (!file) return;
    const form = new FormData();
    form.set("file", file);
    try {
      const result = await api<{ url: string }>(
        `/api/v1/documents/${documentId}/attachments`,
        { method: "POST", body: form },
      );
      editor.chain().focus().setImage({ src: result.url, alt: file.name }).run();
    } finally {
      event.target.value = "";
    }
  };

  // Keys are taken on the editor itself, in the capture phase, so the menu
  // gets them before ProseMirror moves the caret.
  useEffect(() => {
    if (!trigger) return;
    const dom = editor.view.dom;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        close();
        return;
      }
      if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        event.preventDefault();
        event.stopPropagation();
        setActive((current) => {
          const step = event.key === "ArrowDown" ? 1 : -1;
          const total = matches.length;
          if (total === 0) return 0;
          return (current + step + total) % total;
        });
        return;
      }
      if (event.key === "Enter" || event.key === "Tab") {
        const command = matches[active];
        if (!command) return;
        event.preventDefault();
        event.stopPropagation();
        run(command, trigger);
      }
    };
    dom.addEventListener("keydown", onKeyDown, true);
    return () => dom.removeEventListener("keydown", onKeyDown, true);
  }, [active, close, editor, matches, run, trigger]);

  if (!canEdit) return null;

  const open = Boolean(trigger && anchor && matches.length > 0);
  return (
    <>
      <input
        ref={fileInput}
        hidden
        type="file"
        accept="image/*"
        onChange={uploadImage}
      />
      <Popper
        open={open}
        placement="bottom-start"
        anchorEl={
          anchor
            ? {
                getBoundingClientRect: () =>
                  new DOMRect(anchor.left, anchor.top, 0, 0),
              }
            : null
        }
        sx={{ zIndex: 1300 }}
      >
        <Paper
          elevation={8}
          sx={{
            mt: 0.5,
            width: 268,
            maxHeight: 320,
            overflowY: "auto",
            borderRadius: 2,
            border: "1px solid",
            borderColor: "divider",
            py: 0.5,
          }}
          className="admin-menu-scroll"
        >
          {grouped.map((entry) => (
            <Box key={entry.group}>
              <ListSubheader sx={{ lineHeight: "26px", fontSize: 11.5, px: 1.5 }}>
                {entry.group}
              </ListSubheader>
              {entry.commands.map((command) => {
                const index = matches.indexOf(command);
                return (
                  <Stack
                    key={command.id}
                    direction="row"
                    alignItems="center"
                    gap={1}
                    onMouseEnter={() => setActive(index)}
                    onMouseDown={(event) => {
                      event.preventDefault();
                      if (trigger) run(command, trigger);
                    }}
                    sx={{
                      px: 1.5,
                      py: 0.7,
                      cursor: "pointer",
                      bgcolor: index === active ? "action.selected" : "transparent",
                    }}
                  >
                    <Typography variant="body2" sx={{ flex: 1 }}>
                      {command.label}
                    </Typography>
                    {command.hint && (
                      <Typography
                        variant="caption"
                        color="text.disabled"
                        sx={{ fontFamily: "monospace" }}
                      >
                        {command.hint}
                      </Typography>
                    )}
                  </Stack>
                );
              })}
            </Box>
          ))}
        </Paper>
      </Popper>
    </>
  );
}
