import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { Editor } from "@tiptap/react";
import {
  Box,
  Button,
  Chip,
  Divider,
  Paper,
  Stack,
  TextField,
  Typography,
} from "@mui/material";
import { Check } from "@mui/icons-material";
import { api, formatDate, jsonBody } from "../../../lib/api";
import type { CommentItem, DocumentItem } from "../../../types";
import { blockIdAt, locateAnchor, readAnchor } from "./anchor";

export function CommentsPanel({
  document,
  editor,
  canComment,
}: {
  document: DocumentItem;
  editor: Editor;
  canComment: boolean;
}) {
  const client = useQueryClient();
  const [body, setBody] = useState("");
  const query = useQuery({
    queryKey: ["comments", document.id],
    queryFn: () =>
      api<CommentItem[]>(`/api/v1/documents/${document.id}/comments`),
  });
  const create = useMutation({
    mutationFn: () => {
      const { from, to } = editor.state.selection;
      return api(`/api/v1/documents/${document.id}/comments`, {
        method: "POST",
        ...jsonBody({
          body,
          anchor: {
            from,
            to,
            // The block id is what makes the comment survive editing above
            // it; the positions are kept for comments read by older clients.
            blockId: blockIdAt(editor, from),
            selectedText: editor.state.doc.textBetween(from, to, " "),
          },
        }),
      });
    },
    onSuccess: () => {
      setBody("");
      void client.invalidateQueries({ queryKey: ["comments", document.id] });
    },
  });
  const resolve = useMutation({
    mutationFn: (id: string) =>
      api(`/api/v1/comments/${id}/resolve`, { method: "POST" }),
    onSuccess: () =>
      client.invalidateQueries({ queryKey: ["comments", document.id] }),
  });
  return (
    <Stack gap={1.5}>
      {canComment && (
        <>
          <Typography variant="h3">선택 영역에 댓글</Typography>
          <TextField
            multiline
            minRows={2}
            value={body}
            onChange={(e) => setBody(e.target.value)}
            placeholder="@아이디로 멘션할 수 있습니다."
          />
          <Button
            variant="contained"
            disabled={!body.trim()}
            onClick={() => create.mutate()}
          >
            댓글 등록
          </Button>
          <Divider />
        </>
      )}
      {(query.data ?? []).map((comment) => (
        <Paper
          key={comment.id}
          variant="outlined"
          sx={{ p: 1.75, opacity: comment.resolvedAt ? 0.8 : 1 }}
        >
          <Stack direction="row" justifyContent="space-between">
            <Typography fontWeight={700}>
              {comment.author.displayName}
            </Typography>
            <Typography variant="caption" color="text.secondary">
              {formatDate(comment.createdAt)}
            </Typography>
          </Stack>
          {(() => {
            const anchor = readAnchor(comment.anchor);
            if (!anchor.selectedText?.trim()) return null;
            return (
              <Box
                component="button"
                type="button"
                onClick={() => {
                  const range = locateAnchor(editor, anchor);
                  if (!range) return;
                  editor
                    .chain()
                    .focus()
                    .setTextSelection(range)
                    .scrollIntoView()
                    .run();
                }}
                title="본문에서 보기"
                sx={{
                  display: "block",
                  width: "100%",
                  textAlign: "left",
                  mt: 0.75,
                  px: 1,
                  py: 0.5,
                  border: 0,
                  borderLeft: "3px solid",
                  borderLeftColor: "warning.light",
                  bgcolor: "action.hover",
                  borderRadius: 0.5,
                  cursor: "pointer",
                  font: "inherit",
                  fontSize: 13,
                  color: "text.secondary",
                  whiteSpace: "nowrap",
                  overflow: "hidden",
                  textOverflow: "ellipsis",
                  "&:hover": { bgcolor: "action.selected" },
                }}
              >
                {anchor.selectedText}
              </Box>
            );
          })()}
          <Typography sx={{ whiteSpace: "pre-wrap", my: 1 }}>
            {comment.body}
          </Typography>
          {comment.resolvedAt ? (
            <Chip size="small" label="해결됨" />
          ) : (
            <Button
              size="small"
              startIcon={<Check />}
              onClick={() => resolve.mutate(comment.id)}
            >
              해결
            </Button>
          )}
        </Paper>
      ))}
      {!(query.data ?? []).length && (
        <Typography color="text.secondary" textAlign="center" py={4}>
          등록된 댓글이 없습니다.
        </Typography>
      )}
    </Stack>
  );
}
