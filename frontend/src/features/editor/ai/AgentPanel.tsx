import { useEffect, useRef, useState } from "react";
import type { Editor } from "@tiptap/react";
import {
  Alert,
  Box,
  Button,
  Chip,
  Collapse,
  Divider,
  FormControlLabel,
  IconButton,
  Paper,
  Stack,
  Switch,
  TextField,
  Tooltip,
  Typography,
} from "@mui/material";
import {
  AutoAwesome,
  ErrorOutline,
  ExpandLess,
  ExpandMore,
  ManageSearch,
  PlaylistAddOutlined,
  PsychologyOutlined,
  RestartAltOutlined,
} from "@mui/icons-material";
import type { DocumentItem } from "../../../types";
import { Markdown } from "../../../components/Markdown";
import { markdownToContent } from "../../../lib/markdownContent";
import {
  useAIStream,
  type AIHistoryTurn,
  type AIToolCall,
} from "./useAIStream";

const shortcuts = [
  { label: "문서 요약", prompt: "이 문서를 핵심 항목 중심으로 요약해줘." },
  {
    label: "내용 검토",
    prompt: "이 문서의 논리, 누락, 모호한 표현을 검토해줘.",
  },
  { label: "목차 제안", prompt: "이 문서에 적절한 목차와 구조를 제안해줘." },
];

const workspaceShortcuts = [
  {
    label: "관련 문서 찾기",
    prompt: "이 문서와 관련된 다른 문서를 찾아 무엇이 연결되는지 알려줘.",
  },
  {
    label: "최근 변경 요약",
    prompt: "이 문서의 최근 두 버전을 비교해서 무엇이 달라졌는지 알려줘.",
  },
];

/** Follow-ups only make sense once something has been said. */
const followUps = [
  "더 짧게",
  "근거가 되는 문장을 인용해줘",
  "표로 정리해줘",
  "실무자가 읽을 말투로",
];

/** What each tool is doing, in words a reader can follow. */
const toolLabels: Record<string, string> = {
  search_documents: "문서 검색",
  read_document: "문서 읽기",
  get_document_outline: "목차 확인",
  list_revisions: "버전 목록",
  compare_revisions: "버전 비교",
};

/** One thing that was said, kept so the next question can build on it. */
type Turn = {
  role: "user" | "assistant";
  content: string;
  toolCalls?: AIToolCall[];
  reasoning?: string;
};

/**
 * How many past turns are sent back with the next question. The document body
 * already goes with every request, so history is the cheap part — but it is
 * not free, and a conversation nobody ended would grow until the model refused
 * it. Twelve turns is six exchanges, past which the early ones rarely matter.
 */
const HISTORY_LIMIT = 12;

/**
 * AgentPanel asks about the document as a whole. The selection menu handles
 * rewrites; this panel answers questions and hands the reply to the author to
 * insert where they want it.
 *
 * It used to forget everything between questions. Each request carried one
 * message, so "더 짧게" had nothing to shorten and every follow-up had to
 * restate the whole thing — which is not how anyone talks to an assistant.
 * The server already accepted a list of messages; nothing here was sending
 * one.
 */
export function AgentPanel({
  document,
  editor,
  enabled,
  canEdit,
  maxTokens,
}: {
  document: DocumentItem;
  editor: Editor;
  enabled: boolean;
  canEdit: boolean;
  maxTokens: number;
}) {
  const [prompt, setPrompt] = useState("");
  const [turns, setTurns] = useState<Turn[]>([]);
  // With tools on, the model may search and read other documents the reader
  // has access to before answering; off, it only sees this one.
  const [useTools, setUseTools] = useState(false);
  // The working is worth watching while it is the only thing happening, and
  // worth folding away the moment the answer starts.
  const [openReasoning, setOpenReasoning] = useState(false);
  const [pinnedReasoning, setPinnedReasoning] = useState(false);
  const stream = useAIStream();
  const bottom = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (pinnedReasoning) return;
    setOpenReasoning(stream.thinking);
  }, [stream.thinking, pinnedReasoning]);

  // A conversation that grows off the bottom of the panel is a conversation
  // the reader has to chase.
  useEffect(() => {
    bottom.current?.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [turns.length, stream.running]);

  const ask = async (value = prompt, tools = useTools) => {
    const question = value.trim();
    if (!question || stream.running) return;
    setPrompt("");
    setPinnedReasoning(false);
    // The question joins the transcript before the answer exists, so the
    // reader can see what they asked while it is being worked on.
    setTurns((current) => [...current, { role: "user", content: question }]);

    const history: AIHistoryTurn[] = turns
      .slice(-HISTORY_LIMIT)
      .map(({ role, content }) => ({ role, content }));

    const answer = await stream.run({
      prompt: question,
      action: tools ? "workspace_agent" : "document_agent",
      documentId: document.id,
      maxTokens: Math.min(262144, maxTokens),
      tools,
      history,
    });

    if (!answer.trim()) {
      // Nothing came back — it failed, or it was stopped before any text
      // arrived. Leaving the question in the transcript would imply a reply
      // that never came, and would send an unanswered turn as history next
      // time. The question goes back in the box instead, ready to retry.
      setTurns((current) => current.slice(0, -1));
      setPrompt(question);
      return;
    }
    setTurns((current) => [
      ...current,
      {
        role: "assistant",
        content: answer,
        toolCalls: stream.finished.current.toolCalls,
        reasoning: stream.finished.current.reasoning,
      },
    ]);
    stream.reset();
  };

  const insert = (markdown: string) =>
    editor
      .chain()
      .focus()
      // The answer is Markdown; inserting it as text would put the asterisks
      // and hashes into the document.
      .insertContent(markdownToContent(markdown, { forceBlocks: true }))
      .run();

  if (!enabled)
    return (
      <Alert severity="info">
        관리자가 AI 연결을 활성화하면 문서 Agent를 사용할 수 있습니다.
      </Alert>
    );

  const started = turns.length > 0;

  return (
    <Stack gap={1.5}>
      <Stack direction="row" alignItems="center" gap={1}>
        <Typography variant="h3" flex={1}>
          Document Agent
        </Typography>
        {started && (
          <Tooltip title="지금까지의 대화를 지우고 새로 시작합니다">
            <span>
              <IconButton
                aria-label="지금까지의 대화를 지우고 새로 시작합니다"
                size="small"
                disabled={stream.running}
                onClick={() => {
                  stream.reset();
                  setTurns([]);
                  setPrompt("");
                }}
              >
                <RestartAltOutlined fontSize="small" />
              </IconButton>
            </span>
          </Tooltip>
        )}
      </Stack>
      <Typography variant="body2" color="text.secondary">
        현재 문서 ACL을 확인한 본문만 AI 컨텍스트에 포함합니다. 이어서 물어보면
        앞의 답을 기억합니다.
      </Typography>

      {!started && (
        <Stack direction="row" gap={0.75} flexWrap="wrap">
          {shortcuts.map((shortcut) => (
            <Chip
              key={shortcut.label}
              clickable
              label={shortcut.label}
              onClick={() => void ask(shortcut.prompt, false)}
            />
          ))}
          {workspaceShortcuts.map((shortcut) => (
            <Chip
              key={shortcut.label}
              clickable
              variant="outlined"
              icon={<ManageSearch fontSize="small" />}
              label={shortcut.label}
              onClick={() => {
                setUseTools(true);
                void ask(shortcut.prompt, true);
              }}
            />
          ))}
        </Stack>
      )}

      {started && (
        <Stack gap={1.5}>
          {turns.map((turn, index) =>
            turn.role === "user" ? (
              <Paper
                key={index}
                variant="outlined"
                sx={{
                  px: 1.5,
                  py: 1.1,
                  borderRadius: 1.5,
                  bgcolor: "action.hover",
                  alignSelf: "flex-end",
                  maxWidth: "92%",
                }}
              >
                <Typography variant="body2" sx={{ whiteSpace: "pre-wrap" }}>
                  {turn.content}
                </Typography>
              </Paper>
            ) : (
              <Stack key={index} gap={0.75}>
                <ToolTrail calls={turn.toolCalls} />
                <ReasoningBlock reasoning={turn.reasoning} />
                <Paper variant="outlined" sx={{ p: 2, borderRadius: 1.5 }}>
                  <Markdown text={turn.content} />
                </Paper>
                {canEdit && (
                  <Button
                    size="small"
                    startIcon={<PlaylistAddOutlined />}
                    sx={{ alignSelf: "flex-start" }}
                    onClick={() => insert(turn.content)}
                  >
                    커서 위치에 삽입
                  </Button>
                )}
              </Stack>
            ),
          )}
        </Stack>
      )}

      {stream.running && <ToolTrail calls={stream.toolCalls} />}
      {stream.running && stream.reasoning && (
        <Paper variant="outlined" sx={{ borderRadius: 1.5 }}>
          <Stack
            direction="row"
            alignItems="center"
            gap={0.75}
            sx={{ px: 1.5, py: 1, cursor: "pointer" }}
            onClick={() => {
              setPinnedReasoning(true);
              setOpenReasoning((current) => !current);
            }}
          >
            <PsychologyOutlined fontSize="small" color="disabled" />
            <Typography variant="caption" color="text.secondary" flex={1}>
              {stream.thinking ? "생각하는 중…" : "생각한 과정"}
            </Typography>
            {openReasoning ? (
              <ExpandLess fontSize="small" color="disabled" />
            ) : (
              <ExpandMore fontSize="small" color="disabled" />
            )}
          </Stack>
          <Collapse in={openReasoning}>
            <Box sx={reasoningBody}>{stream.reasoning}</Box>
          </Collapse>
        </Paper>
      )}
      {stream.running && (
        <Paper variant="outlined" sx={{ p: 2, borderRadius: 1.5 }}>
          {stream.text ? (
            <Markdown text={stream.text} />
          ) : (
            <Typography component="span" color="text.secondary">
              생각하는 중…
            </Typography>
          )}
          <Box
            component="span"
            sx={{
              display: "inline-block",
              width: 7,
              height: 17,
              bgcolor: "primary.main",
              ml: 0.4,
              verticalAlign: "text-bottom",
              animation: "blink 1s infinite",
            }}
          />
        </Paper>
      )}
      {stream.error && <Alert severity="error">{stream.error}</Alert>}

      <div ref={bottom} />
      {started && <Divider />}

      {started && !stream.running && (
        <Stack direction="row" gap={0.75} flexWrap="wrap">
          {followUps.map((text) => (
            <Chip
              key={text}
              size="small"
              clickable
              variant="outlined"
              label={text}
              onClick={() => void ask(text)}
            />
          ))}
        </Stack>
      )}

      <Tooltip title="켜면 이 문서 외에 내가 볼 수 있는 다른 문서도 찾아 읽고 답합니다.">
        <FormControlLabel
          sx={{ mt: -0.5 }}
          control={
            <Switch
              size="small"
              checked={useTools}
              onChange={(event) => setUseTools(event.target.checked)}
            />
          }
          label={
            <Typography variant="body2">
              워크스페이스 문서까지 찾아보기
            </Typography>
          }
        />
      </Tooltip>
      <TextField
        multiline
        minRows={started ? 2 : 3}
        placeholder={
          started
            ? "이어서 물어보세요"
            : "이 문서에 대해 질문하거나 작업을 요청하세요"
        }
        value={prompt}
        onChange={(event) => setPrompt(event.target.value)}
        onKeyDown={(event) => {
          // Enter sends, Shift+Enter breaks the line — a conversation is
          // mostly short turns, and reaching for the mouse each time is what
          // makes a panel feel like a form.
          if (event.key === "Enter" && !event.shiftKey && !event.nativeEvent.isComposing) {
            event.preventDefault();
            void ask();
          }
        }}
      />
      <Stack direction="row" gap={1}>
        {stream.running ? (
          <Button color="error" variant="outlined" onClick={stream.stop}>
            중지
          </Button>
        ) : (
          <Button
            variant="contained"
            startIcon={<AutoAwesome />}
            disabled={!prompt.trim()}
            onClick={() => void ask()}
          >
            {started ? "이어서 묻기" : "스트리밍 실행"}
          </Button>
        )}
      </Stack>
    </Stack>
  );
}

const reasoningBody = {
  px: 1.5,
  pb: 1.5,
  maxHeight: 260,
  overflowY: "auto",
  whiteSpace: "pre-wrap",
  fontSize: 13,
  lineHeight: 1.65,
  color: "text.secondary",
  borderTop: "1px solid",
  borderColor: "divider",
  pt: 1.25,
} as const;

function ToolTrail({ calls }: { calls?: AIToolCall[] }) {
  if (!calls || calls.length === 0) return null;
  return (
    <Stack gap={0.5}>
      {calls.map((call, index) => (
        <Stack
          key={`${call.tool}-${index}`}
          direction="row"
          gap={0.75}
          alignItems="center"
        >
          {call.error ? (
            <ErrorOutline fontSize="small" color="error" />
          ) : (
            <ManageSearch fontSize="small" color="disabled" />
          )}
          <Typography
            variant="caption"
            color={call.error ? "error.main" : "text.secondary"}
          >
            {toolLabels[call.tool] ?? call.tool}
            {call.error ? ` · ${call.error}` : ""}
          </Typography>
        </Stack>
      ))}
    </Stack>
  );
}

/** The working behind a finished answer, folded away until asked for. */
function ReasoningBlock({ reasoning }: { reasoning?: string }) {
  const [open, setOpen] = useState(false);
  if (!reasoning?.trim()) return null;
  return (
    <Paper variant="outlined" sx={{ borderRadius: 1.5 }}>
      <Stack
        direction="row"
        alignItems="center"
        gap={0.75}
        sx={{ px: 1.5, py: 0.9, cursor: "pointer" }}
        onClick={() => setOpen((current) => !current)}
      >
        <PsychologyOutlined fontSize="small" color="disabled" />
        <Typography variant="caption" color="text.secondary" flex={1}>
          생각한 과정
        </Typography>
        {open ? (
          <ExpandLess fontSize="small" color="disabled" />
        ) : (
          <ExpandMore fontSize="small" color="disabled" />
        )}
      </Stack>
      <Collapse in={open}>
        <Box sx={reasoningBody}>{reasoning}</Box>
      </Collapse>
    </Paper>
  );
}
