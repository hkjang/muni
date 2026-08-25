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
import { Check, ReplyOutlined, UndoOutlined } from "@mui/icons-material";
import { api, formatDate, jsonBody } from "../../../lib/api";
import type { CommentItem, DocumentItem } from "../../../types";
import { blockIdAt, locateAnchor, readAnchor } from "./anchor";
import { buildThreads, isResolved, sortThreads } from "./threads";

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
  const [replyTo, setReplyTo] = useState<string | null>(null);
  const [reply, setReply] = useState("");
  const query = useQuery({
    queryKey: ["comments", document.id],
    queryFn: () =>
      api<CommentItem[]>(`/api/v1/documents/${document.id}/comments`),
  });
  const refresh = () =>
    client.invalidateQueries({ queryKey: ["comments", document.id] });

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
      void refresh();
    },
  });

  const answer = useMutation({
    mutationFn: ({ parentId, text }: { parentId: string; text: string }) =>
      api(`/api/v1/documents/${document.id}/comments`, {
        method: "POST",
        ...jsonBody({ body: text, parentId }),
      }),
    onSuccess: () => {
      setReply("");
      setReplyTo(null);
      void refresh();
    },
  });

  const resolve = useMutation({
    mutationFn: (id: string) =>
      api(`/api/v1/comments/${id}/resolve`, { method: "POST" }),
    onSuccess: () => refresh(),
  });

  const reopen = useMutation({
    mutationFn: (id: string) =>
      api(`/api/v1/comments/${id}/reopen`, { method: "POST" }),
    onSuccess: () => refresh(),
  });

  const threads = sortThreads(buildThreads(query.data ?? []));
  const openCount = threads.filter((thread) => !isResolved(thread)).length;

  const goToAnchor = (comment: CommentItem) => {
    const anchor = readAnchor(comment.anchor);
    const range = locateAnchor(editor, anchor);
    if (!range) return;
    editor.chain().focus().setTextSelection(range).scrollIntoView().run();
  };

  return (
    <Stack gap={1.5}>
      {canComment && (
        <>
          <Typography variant="h3">선택 영역에 댓글</Typography>
          <TextField
            multiline
            minRows={2}
            value={body}
            onChange={(event) => setBody(event.target.value)}
            placeholder="본문에서 문장을 선택한 뒤 남기면 그 위치에 붙습니다."
          />
          <Button
            variant="contained"
            disabled={!body.trim() || create.isPending}
            onClick={() => create.mutate()}
          >
            댓글 등록
          </Button>
          <Divider />
        </>
      )}

      {threads.length > 0 && (
        <Typography variant="caption" color="text.secondary">
          열린 댓글 {openCount}개 · 전체 {threads.length}개
        </Typography>
      )}

      {threads.map((thread) => {
        const anchor = readAnchor(thread.root.anchor);
        const resolved = isResolved(thread);
        return (
          <Paper
            key={thread.root.id}
            variant="outlined"
            sx={{ p: 1.75, opacity: resolved ? 0.72 : 1 }}
          >
            {anchor.selectedText?.trim() && (
              <Box
                component="button"
                type="button"
                onClick={() => goToAnchor(thread.root)}
                title="본문에서 보기"
                sx={quotedStyle}
              >
                {anchor.selectedText}
              </Box>
            )}
            <CommentBody comment={thread.root} />
            {thread.replies.length > 0 && (
              <Stack
                gap={1.25}
                sx={{
                  mt: 1.25,
                  pl: 1.5,
                  borderLeft: "2px solid",
                  borderColor: "divider",
                }}
              >
                {thread.replies.map((item) => (
                  <CommentBody key={item.id} comment={item} />
                ))}
              </Stack>
            )}

            <Stack direction="row" gap={0.5} mt={1.25} alignItems="center">
              {resolved ? (
                <>
                  <Chip size="small" label="해결됨" />
                  {canComment && (
                    <Button
                      size="small"
                      startIcon={<UndoOutlined />}
                      onClick={() => reopen.mutate(thread.root.id)}
                    >
                      다시 열기
                    </Button>
                  )}
                </>
              ) : (
                canComment && (
                  <>
                    <Button
                      size="small"
                      startIcon={<ReplyOutlined />}
                      onClick={() =>
                        setReplyTo(replyTo === thread.root.id ? null : thread.root.id)
                      }
                    >
                      답글
                    </Button>
                    <Button
                      size="small"
                      startIcon={<Check />}
                      onClick={() => resolve.mutate(thread.root.id)}
                    >
                      해결
                    </Button>
                  </>
                )
              )}
            </Stack>

            {replyTo === thread.root.id && (
              <Stack gap={1} mt={1}>
                <TextField
                  autoFocus
                  multiline
                  minRows={2}
                  size="small"
                  value={reply}
                  onChange={(event) => setReply(event.target.value)}
                  placeholder="답글"
                  onKeyDown={(event) => {
                    if (event.key === "Escape") setReplyTo(null);
                  }}
                />
                <Stack direction="row" gap={1}>
                  <Button
                    size="small"
                    variant="contained"
                    disabled={!reply.trim() || answer.isPending}
                    onClick={() =>
                      answer.mutate({ parentId: thread.root.id, text: reply })
                    }
                  >
                    등록
                  </Button>
                  <Button size="small" color="inherit" onClick={() => setReplyTo(null)}>
                    취소
                  </Button>
                </Stack>
              </Stack>
            )}
          </Paper>
        );
      })}

      {threads.length === 0 && (
        <Typography color="text.secondary" textAlign="center" py={4}>
          등록된 댓글이 없습니다.
        </Typography>
      )}
    </Stack>
  );
}

function CommentBody({ comment }: { comment: CommentItem }) {
  return (
    <Box>
      <Stack direction="row" justifyContent="space-between" alignItems="baseline">
        <Typography fontWeight={700} variant="body2">
          {comment.author.displayName}
        </Typography>
        <Typography variant="caption" color="text.secondary">
          {formatDate(comment.createdAt)}
        </Typography>
      </Stack>
      <Typography sx={{ whiteSpace: "pre-wrap", mt: 0.3 }} variant="body2">
        {comment.body}
      </Typography>
    </Box>
  );
}

const quotedStyle = {
  display: "block",
  width: "100%",
  textAlign: "left",
  mb: 1,
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
} as const;
