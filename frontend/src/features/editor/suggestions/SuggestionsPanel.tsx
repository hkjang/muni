import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { Editor } from "@tiptap/react";
import {
  Button,
  ButtonGroup,
  Chip,
  Divider,
  Paper,
  Stack,
  TextField,
  Typography,
} from "@mui/material";
import { AutoAwesome, Check, Close } from "@mui/icons-material";
import { api, jsonBody } from "../../../lib/api";
import type { DocumentItem } from "../../../types";
import type { Suggestion } from "../types";
import { applySuggestion } from "./applySuggestion";
import { AIPatchRequest } from "./AIPatchRequest";

export function SuggestionsPanel({
  document,
  editor,
  canComment,
  canEdit,
  aiEnabled,
}: {
  document: DocumentItem;
  editor: Editor;
  canComment: boolean;
  canEdit: boolean;
  aiEnabled: boolean;
}) {
  const client = useQueryClient();
  const [replacement, setReplacement] = useState("");
  const query = useQuery({
    queryKey: ["suggestions", document.id],
    queryFn: () =>
      api<Suggestion[]>(`/api/v1/documents/${document.id}/suggestions`),
  });
  const create = useMutation({
    mutationFn: () => {
      const { from, to } = editor.state.selection;
      return api(`/api/v1/documents/${document.id}/suggestions`, {
        method: "POST",
        ...jsonBody({
          range: { from, to },
          previousValue: editor.state.doc.textBetween(from, to, " "),
          newValue: replacement,
        }),
      });
    },
    onSuccess: () => {
      setReplacement("");
      void client.invalidateQueries({ queryKey: ["suggestions", document.id] });
    },
  });
  const decide = useMutation({
    mutationFn: ({
      item,
      decision,
    }: {
      item: Suggestion;
      decision: "ACCEPTED" | "REJECTED";
    }) => {
      if (decision === "ACCEPTED") {
        const outcome = applySuggestion(editor, item);
        if (!outcome.applied) {
          // Refusing beats resolving a suggestion that was never applied.
          throw new Error(
            outcome.reason === "block-gone"
              ? "제안이 가리키는 부분이 문서에서 사라졌습니다."
              : "이 제안은 본문에 적용할 수 없습니다.",
          );
        }
      }
      return api(`/api/v1/suggestions/${item.id}/decision`, {
        method: "POST",
        ...jsonBody({ decision }),
      });
    },
    onSuccess: () =>
      client.invalidateQueries({ queryKey: ["suggestions", document.id] }),
  });
  return (
    <Stack gap={1.5}>
      {canComment && (
        <>
          <Typography variant="h3">변경 제안</Typography>
          <Typography variant="body2" color="text.secondary">
            본문의 교체할 범위를 선택하고 새 문구를 입력하세요.
          </Typography>
          <TextField
            multiline
            minRows={2}
            value={replacement}
            onChange={(e) => setReplacement(e.target.value)}
            placeholder="제안할 문구"
          />
          <Button
            variant="contained"
            disabled={!replacement.trim()}
            onClick={() => create.mutate()}
          >
            제안 등록
          </Button>
          <Divider />
          <AIPatchRequest documentId={document.id} enabled={aiEnabled} />
          <Divider />
        </>
      )}
      {(query.data ?? []).map((item) => (
        <Paper key={item.id} variant="outlined" sx={{ p: 1.75 }}>
          <Stack direction="row" justifyContent="space-between" alignItems="center">
            <Stack direction="row" gap={0.75} alignItems="center">
              {item.origin === "AI" ? (
                <Chip size="small" color="secondary" icon={<AutoAwesome />} label="AI" />
              ) : (
                <Typography fontWeight={700}>{item.author.displayName}</Typography>
              )}
            </Stack>
            <Chip size="small" label={item.status} />
          </Stack>
          {item.note && (
            <Typography variant="caption" color="text.secondary" display="block" mt={0.75}>
              {item.note}
            </Typography>
          )}
          {typeof item.previousValue === "string" && item.previousValue && (
            <Typography
              variant="body2"
              sx={{ whiteSpace: "pre-wrap", mt: 1, textDecoration: "line-through", opacity: 0.7 }}
            >
              {item.previousValue}
            </Typography>
          )}
          <Typography variant="body2" color="text.secondary" mt={1}>
            제안
          </Typography>
          <Typography sx={{ whiteSpace: "pre-wrap" }}>
            {typeof item.newValue === "string"
              ? item.newValue
              : JSON.stringify(item.newValue)}
          </Typography>
          {canEdit && item.status === "PENDING" && (
            <ButtonGroup size="small" sx={{ mt: 1.5 }}>
              <Button
                color="error"
                startIcon={<Close />}
                onClick={() => decide.mutate({ item, decision: "REJECTED" })}
              >
                거절
              </Button>
              <Button
                color="success"
                startIcon={<Check />}
                onClick={() => decide.mutate({ item, decision: "ACCEPTED" })}
              >
                적용
              </Button>
            </ButtonGroup>
          )}
        </Paper>
      ))}
      {!(query.data ?? []).length && (
        <Typography color="text.secondary" textAlign="center" py={4}>
          대기 중인 제안이 없습니다.
        </Typography>
      )}
    </Stack>
  );
}
