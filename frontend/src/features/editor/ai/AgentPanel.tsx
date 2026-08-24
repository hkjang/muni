import { useState } from "react";
import type { Editor } from "@tiptap/react";
import {
  Alert,
  Box,
  Button,
  Chip,
  Paper,
  Stack,
  TextField,
  Typography,
} from "@mui/material";
import { AutoAwesome } from "@mui/icons-material";
import type { DocumentItem } from "../../../types";
import { useAIStream } from "./useAIStream";

const shortcuts = [
  { label: "문서 요약", prompt: "이 문서를 핵심 항목 중심으로 요약해줘." },
  { label: "내용 검토", prompt: "이 문서의 논리, 누락, 모호한 표현을 검토해줘." },
  { label: "목차 제안", prompt: "이 문서에 적절한 목차와 구조를 제안해줘." },
];

/**
 * AgentPanel asks about the document as a whole. The selection menu handles
 * rewrites; this panel answers questions and hands the reply to the author to
 * insert where they want it.
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
  const stream = useAIStream();

  const ask = (value = prompt) => {
    if (!value.trim() || stream.running) return;
    setPrompt(value);
    void stream.run({
      prompt: value,
      action: "document_agent",
      documentId: document.id,
      maxTokens: Math.min(262144, maxTokens),
    });
  };

  if (!enabled)
    return (
      <Alert severity="info">
        관리자가 AI 연결을 활성화하면 문서 Agent를 사용할 수 있습니다.
      </Alert>
    );

  return (
    <Stack gap={1.5}>
      <Typography variant="h3">Document Agent</Typography>
      <Typography variant="body2" color="text.secondary">
        현재 문서 ACL을 확인한 본문만 AI 컨텍스트에 포함합니다.
      </Typography>
      <Stack direction="row" gap={0.75} flexWrap="wrap">
        {shortcuts.map((shortcut) => (
          <Chip
            key={shortcut.label}
            clickable
            label={shortcut.label}
            onClick={() => ask(shortcut.prompt)}
          />
        ))}
      </Stack>
      <TextField
        multiline
        minRows={3}
        placeholder="이 문서에 대해 질문하거나 작업을 요청하세요"
        value={prompt}
        onChange={(event) => setPrompt(event.target.value)}
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
            onClick={() => ask()}
          >
            스트리밍 실행
          </Button>
        )}
      </Stack>
      {stream.error && <Alert severity="error">{stream.error}</Alert>}
      {(stream.text || stream.running) && (
        <Paper
          variant="outlined"
          sx={{
            p: 2,
            bgcolor: "#fafafe",
            whiteSpace: "pre-wrap",
            fontSize: 15,
            lineHeight: 1.7,
          }}
        >
          {stream.text}
          {stream.running && (
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
          )}
        </Paper>
      )}
      {stream.text && canEdit && (
        <Button
          variant="outlined"
          onClick={() =>
            editor.chain().focus().insertContent(stream.text).run()
          }
        >
          커서 위치에 삽입
        </Button>
      )}
    </Stack>
  );
}
