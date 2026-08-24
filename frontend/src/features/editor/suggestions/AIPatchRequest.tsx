import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Button,
  Chip,
  Stack,
  TextField,
  Typography,
} from "@mui/material";
import { AutoAwesome } from "@mui/icons-material";
import { api, errorMessage, jsonBody } from "../../../lib/api";

const presets = [
  { label: "임원 보고 관점", instruction: "임원 보고에 맞게 결론을 앞세우고 군더더기를 덜어 주세요." },
  { label: "근거 보강", instruction: "근거 없이 단정한 문장을 찾아 완화하거나 근거를 요구하는 문장으로 고쳐 주세요." },
  { label: "문장 다듬기", instruction: "뜻을 바꾸지 말고 어색한 문장을 자연스럽게 다듬어 주세요." },
];

/**
 * AIPatchRequest asks the model to propose block-level rewrites. Nothing is
 * applied: the proposals land in the same review queue a colleague's suggestion
 * does, so a person decides each one.
 */
export function AIPatchRequest({
  documentId,
  enabled,
}: {
  documentId: string;
  enabled: boolean;
}) {
  const client = useQueryClient();
  const [instruction, setInstruction] = useState("");
  const [outcome, setOutcome] = useState("");

  const propose = useMutation({
    mutationFn: (value: string) =>
      api<{ count: number }>(`/api/v1/documents/${documentId}/ai/patch`, {
        method: "POST",
        ...jsonBody({ instruction: value }),
      }),
    onSuccess: (result) => {
      setOutcome(
        result.count > 0
          ? `${result.count}건을 제안했습니다. 아래에서 확인하세요.`
          : "고칠 부분을 찾지 못했습니다.",
      );
      void client.invalidateQueries({ queryKey: ["suggestions", documentId] });
    },
  });

  if (!enabled) return null;

  const ask = (value: string) => {
    if (!value.trim() || propose.isPending) return;
    setInstruction(value);
    setOutcome("");
    propose.mutate(value.trim());
  };

  return (
    <Stack gap={1}>
      <Typography variant="body2" color="text.secondary">
        AI에게 문서 전체를 검토해 고칠 부분을 제안하도록 요청할 수 있습니다.
        제안은 바로 적용되지 않고 아래 목록에서 하나씩 확인합니다.
      </Typography>
      <Stack direction="row" gap={0.75} flexWrap="wrap">
        {presets.map((preset) => (
          <Chip
            key={preset.label}
            clickable
            size="small"
            variant="outlined"
            label={preset.label}
            onClick={() => ask(preset.instruction)}
          />
        ))}
      </Stack>
      <TextField
        size="small"
        multiline
        minRows={2}
        placeholder="어떤 관점으로 고칠지 알려 주세요"
        value={instruction}
        onChange={(event) => setInstruction(event.target.value)}
      />
      <Button
        variant="outlined"
        startIcon={<AutoAwesome />}
        disabled={!instruction.trim() || propose.isPending}
        onClick={() => ask(instruction)}
      >
        {propose.isPending ? "검토 중…" : "AI 제안 요청"}
      </Button>
      {propose.error && (
        <Alert severity="error">{errorMessage(propose.error)}</Alert>
      )}
      {outcome && <Alert severity="info">{outcome}</Alert>}
    </Stack>
  );
}
