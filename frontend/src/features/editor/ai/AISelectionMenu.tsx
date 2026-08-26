import { useCallback, useMemo, useRef, useState } from "react";
import type { Editor } from "@tiptap/react";
import { BubbleMenu } from "@tiptap/react/menus";
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  IconButton,
  Paper,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from "@mui/material";
import {
  AutoAwesomeOutlined,
  CheckOutlined,
  CloseOutlined,
  RefreshOutlined,
  SendOutlined,
  StopOutlined,
} from "@mui/icons-material";
import { Markdown } from "../../../components/Markdown";
import { buildPrompt, selectionActions } from "./aiActions";
import {
  resultContent,
  selectionMarkdown,
  selectionShape,
  type SelectionShape,
} from "./selectionContent";
import { useAIStream } from "./useAIStream";

type Range = { from: number; to: number };

/**
 * AISelectionMenu is the floating menu shown after selecting text. The model's
 * answer is never written straight into the document: it is previewed first and
 * only applied when the author accepts it, so an AI edit stays a proposal until
 * a person agrees to it.
 */
export function AISelectionMenu({
  editor,
  enabled,
  canEdit,
  maxTokens,
}: {
  editor: Editor;
  enabled: boolean;
  canEdit: boolean;
  maxTokens?: number;
}) {
  const stream = useAIStream();
  const [range, setRange] = useState<Range | null>(null);
  const [label, setLabel] = useState("");
  const [custom, setCustom] = useState("");
  const [asking, setAsking] = useState(false);
  const originalRef = useRef("");
  const shapeRef = useRef<SelectionShape>({ inline: true });

  const active = Boolean(range) || stream.running;

  const reset = useCallback(() => {
    stream.reset();
    setRange(null);
    setLabel("");
    setCustom("");
    setAsking(false);
    originalRef.current = "";
    shapeRef.current = { inline: true };
  }, [stream]);

  const start = useCallback(
    async (instruction: string, actionLabel: string) => {
      const { from, to } = editor.state.selection;
      // The model is shown Markdown, not stripped text, so the formatting in
      // the selection survives the rewrite.
      const selected = selectionMarkdown(editor, from, to);
      if (!selected) return;
      originalRef.current = selected;
      shapeRef.current = selectionShape(editor, from, to);
      setRange({ from, to });
      setLabel(actionLabel);
      setAsking(false);
      await stream.run({
        prompt: buildPrompt(instruction, selected),
        action: "selection_" + actionLabel,
        maxTokens,
      });
    },
    [editor, maxTokens, stream],
  );

  const apply = useCallback(() => {
    const result = stream.text.trim();
    if (!range || !result) return;
    editor
      .chain()
      .focus()
      .insertContentAt(range, resultContent(result, shapeRef.current))
      .run();
    reset();
  }, [editor, range, reset, stream.text]);

  const shouldShow = useMemo(
    () =>
      ({
        editor: current,
        from,
        to,
      }: {
        editor: Editor;
        from: number;
        to: number;
      }) => {
        if (!enabled || !canEdit || !current.isEditable) return false;
        if (active) return true;
        if (from === to) return false;
        return (
          current.state.doc.textBetween(from, to, " ", " ").trim().length > 0
        );
      },
    [active, canEdit, enabled],
  );

  if (!enabled || !canEdit) return null;

  const done = Boolean(range) && !stream.running && Boolean(stream.text.trim());

  return (
    <BubbleMenu
      editor={editor}
      shouldShow={shouldShow}
      options={{ placement: "bottom-start", offset: 8 }}
    >
      <Paper
        elevation={8}
        // Keeping focus in the editor preserves the selection the result
        // will be written back to.
        onMouseDown={(event) => event.preventDefault()}
        sx={{
          p: 1,
          borderRadius: 2,
          // A phone is narrower than the fixed widths these panels used to
          // assume, and a menu wider than the screen cannot be reached.
          maxWidth: "min(520px, calc(100vw - 24px))",
          border: "1px solid",
          borderColor: "divider",
        }}
      >
        {!range && !asking && (
          <Stack direction="row" flexWrap="wrap" gap={0.5} alignItems="center">
            <AutoAwesomeOutlined
              fontSize="small"
              sx={{ mx: 0.5, opacity: 0.7 }}
            />
            {selectionActions.map((action) => (
              <Chip
                key={action.id}
                size="small"
                label={action.label}
                onClick={() => void start(action.instruction, action.label)}
              />
            ))}
            <Chip
              size="small"
              variant="outlined"
              label="직접 지시"
              onClick={() => setAsking(true)}
            />
          </Stack>
        )}

        {!range && asking && (
          <Stack
            direction="row"
            gap={1}
            alignItems="center"
            sx={{ minWidth: "min(380px, calc(100vw - 60px))" }}
          >
            <TextField
              autoFocus
              fullWidth
              size="small"
              placeholder="선택한 글을 어떻게 바꿀까요?"
              value={custom}
              onChange={(event) => setCustom(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter" && custom.trim()) {
                  event.preventDefault();
                  void start(custom.trim(), "직접 지시");
                }
                if (event.key === "Escape") setAsking(false);
              }}
            />
            <IconButton
              size="small"
              aria-label="지시 보내기"
              disabled={!custom.trim()}
              onClick={() => void start(custom.trim(), "직접 지시")}
            >
              <SendOutlined fontSize="small" />
            </IconButton>
          </Stack>
        )}

        {(range || stream.running) && (
          <Stack gap={1} sx={{ minWidth: "min(380px, calc(100vw - 60px))" }}>
            <Stack direction="row" alignItems="center" gap={1}>
              <Typography variant="caption" color="text.secondary" flex={1}>
                AI 제안 · {label}
              </Typography>
              {stream.running && (
                <>
                  <CircularProgress size={14} />
                  <Tooltip title="중지">
                    <IconButton size="small" onClick={stream.stop}>
                      <StopOutlined fontSize="small" />
                    </IconButton>
                  </Tooltip>
                </>
              )}
              <Tooltip title="닫기">
                <IconButton size="small" onClick={reset}>
                  <CloseOutlined fontSize="small" />
                </IconButton>
              </Tooltip>
            </Stack>

            {stream.error && <Alert severity="error">{stream.error}</Alert>}

            {!stream.error && (
              <Box
                sx={{
                  maxHeight: 220,
                  overflowY: "auto",
                  fontSize: 14,
                  lineHeight: 1.6,
                  bgcolor: "action.hover",
                  borderRadius: 1,
                  px: 1.2,
                  py: 1,
                }}
              >
                {/* The preview is rendered, not shown raw, so it matches what
                    pressing 적용 will put in the document. */}
                {stream.text ? (
                  <Markdown text={stream.text} />
                ) : (
                  <Typography variant="body2" color="text.secondary">
                    {stream.thinking ? "생각하는 중…" : "생성 중…"}
                  </Typography>
                )}
              </Box>
            )}

            {done && (
              <Stack direction="row" gap={1}>
                <Button
                  size="small"
                  variant="contained"
                  startIcon={<CheckOutlined />}
                  onClick={apply}
                >
                  적용
                </Button>
                <Button
                  size="small"
                  startIcon={<RefreshOutlined />}
                  onClick={() => {
                    const instruction = selectionActions.find(
                      (action) => action.label === label,
                    )?.instruction;
                    void start(instruction ?? custom, label);
                  }}
                >
                  다시
                </Button>
                <Button size="small" color="inherit" onClick={reset}>
                  취소
                </Button>
              </Stack>
            )}
          </Stack>
        )}
      </Paper>
    </BubbleMenu>
  );
}
