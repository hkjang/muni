import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Box,
  Card,
  Chip,
  CircularProgress,
  Grid,
  MenuItem,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from "@mui/material";
import { ErrorOutline, ManageSearch } from "@mui/icons-material";
import { api, formatDate } from "../../lib/api";

type AIUsage = {
  days: number;
  summary: {
    calls: number;
    failed: number;
    tokens: number;
    promptTokens: number;
    completionTokens: number;
    people: number;
    averageMs: number;
    toolCalls: number;
  };
  byAction: { action: string; calls: number; failed: number; tokens: number }[];
  items: {
    id: string;
    userName?: string;
    documentId?: string;
    documentTitle?: string;
    action: string;
    model: string;
    status: string;
    promptTokens?: number;
    completionTokens?: number;
    promptChars?: number;
    completionChars?: number;
    durationMs?: number;
    errorCode?: string;
    errorMessage?: string;
    toolCalls: number;
    createdAt: string;
  }[];
};

/** What each recorded action was, in words rather than in identifiers. */
const actionLabels: Record<string, string> = {
  chat: "일반 질문",
  agent: "문서 Agent",
  document_agent: "문서 Agent",
  workspace_agent: "워크스페이스 Agent",
  patch: "AI 수정 제안",
};

function actionLabel(action: string): string {
  if (actionLabels[action]) return actionLabels[action];
  // Selection-menu calls are recorded as selection_다듬기 and friends.
  if (action.startsWith("selection_")) return `선택 영역 · ${action.slice(10)}`;
  return action;
}

const periods = [
  { value: 1, label: "최근 24시간" },
  { value: 7, label: "최근 7일" },
  { value: 30, label: "최근 30일" },
  { value: 90, label: "최근 90일" },
];

export function AdminAIUsagePage() {
  const [days, setDays] = useState(7);
  const [status, setStatus] = useState("");
  const query = useQuery({
    queryKey: ["ai-usage", days, status],
    queryFn: () =>
      api<AIUsage>(
        `/api/v1/admin/ai-usage?days=${days}&limit=100${status ? `&status=${status}` : ""}`,
      ),
    refetchInterval: 60000,
  });

  const summary = query.data?.summary;
  return (
    <Box sx={{ p: { xs: 2.5, sm: 4, lg: 5 }, maxWidth: 1200, mx: "auto" }}>
      <Typography variant="h1">AI 호출 감사</Typography>
      <Typography color="text.secondary" mt={0.7} mb={3}>
        누가 어떤 문서에서 AI를 사용했는지, 얼마나 썼고 무엇이 실패했는지
        기록합니다. 주고받은 내용은 저장하지 않고 분량만 남깁니다.
      </Typography>

      <Stack direction="row" gap={1.5} mb={3} flexWrap="wrap">
        <TextField
          select
          size="small"
          label="기간"
          value={days}
          onChange={(event) => setDays(Number(event.target.value))}
          sx={{ minWidth: 150 }}
        >
          {periods.map((period) => (
            <MenuItem key={period.value} value={period.value}>
              {period.label}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          select
          size="small"
          label="결과"
          value={status}
          onChange={(event) => setStatus(event.target.value)}
          sx={{ minWidth: 150 }}
        >
          <MenuItem value="">전체</MenuItem>
          <MenuItem value="COMPLETED">성공</MenuItem>
          <MenuItem value="FAILED">실패</MenuItem>
        </TextField>
      </Stack>

      {query.isLoading && <CircularProgress />}

      {summary && (
        <Grid container spacing={2} mb={3}>
          <Metric label="호출" value={summary.calls.toLocaleString()} />
          <Metric
            label="실패"
            value={summary.failed.toLocaleString()}
            tone={summary.failed > 0 ? "error.main" : undefined}
          />
          <Metric label="사용자" value={summary.people.toLocaleString()} />
          <Metric
            label="토큰"
            value={summary.tokens.toLocaleString()}
            detail={`입력 ${summary.promptTokens.toLocaleString()} · 출력 ${summary.completionTokens.toLocaleString()}`}
          />
          <Metric
            label="평균 응답"
            value={`${(summary.averageMs / 1000).toFixed(1)}초`}
          />
          <Metric
            label="도구 호출"
            value={summary.toolCalls.toLocaleString()}
          />
        </Grid>
      )}

      {(query.data?.byAction.length ?? 0) > 0 && (
        <Card sx={{ p: 2, mb: 3 }}>
          <Typography variant="h3" mb={1.5}>
            기능별
          </Typography>
          <Stack gap={1}>
            {query.data?.byAction.map((row) => (
              <Stack
                key={row.action}
                direction="row"
                alignItems="center"
                gap={1.5}
              >
                <Typography sx={{ minWidth: 190 }}>
                  {actionLabel(row.action)}
                </Typography>
                <Typography variant="body2" color="text.secondary" flex={1}>
                  {row.calls.toLocaleString()}회
                  {row.failed > 0 ? ` · 실패 ${row.failed}` : ""}
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  {row.tokens > 0 ? `${row.tokens.toLocaleString()} 토큰` : "—"}
                </Typography>
              </Stack>
            ))}
          </Stack>
        </Card>
      )}

      <Stack gap={1}>
        {(query.data?.items ?? []).map((item) => (
          <Card key={item.id} sx={{ p: 2 }}>
            <Stack
              direction={{ xs: "column", sm: "row" }}
              alignItems={{ sm: "center" }}
              gap={1.5}
            >
              <Chip
                size="small"
                color={item.status === "COMPLETED" ? "default" : "error"}
                icon={
                  item.status === "COMPLETED" ? undefined : (
                    <ErrorOutline fontSize="small" />
                  )
                }
                label={actionLabel(item.action)}
              />
              <Box sx={{ flex: 1, minWidth: 0 }}>
                <Typography fontWeight={650} noWrap>
                  {item.userName ?? "삭제된 사용자"}
                  {item.documentTitle ? ` · ${item.documentTitle}` : ""}
                </Typography>
                <Typography variant="body2" color="text.secondary" noWrap>
                  {item.model}
                  {item.promptTokens || item.completionTokens
                    ? ` · ${(item.promptTokens ?? 0).toLocaleString()}→${(item.completionTokens ?? 0).toLocaleString()} 토큰`
                    : ` · ${(item.promptChars ?? 0).toLocaleString()}→${(item.completionChars ?? 0).toLocaleString()}자`}
                  {item.durationMs
                    ? ` · ${(item.durationMs / 1000).toFixed(1)}초`
                    : ""}
                </Typography>
                {item.errorCode && (
                  <Typography variant="body2" color="error.main">
                    {item.errorCode}
                    {item.errorMessage ? ` · ${item.errorMessage}` : ""}
                  </Typography>
                )}
              </Box>
              {item.toolCalls > 0 && (
                <Tooltip title={`도구 ${item.toolCalls}회 사용`}>
                  <Stack direction="row" alignItems="center" gap={0.4}>
                    <ManageSearch fontSize="small" color="disabled" />
                    <Typography variant="caption" color="text.secondary">
                      {item.toolCalls}
                    </Typography>
                  </Stack>
                </Tooltip>
              )}
              <Typography variant="body2" color="text.secondary">
                {formatDate(item.createdAt)}
              </Typography>
            </Stack>
          </Card>
        ))}
        {query.data && query.data.items.length === 0 && (
          <Typography color="text.secondary" textAlign="center" py={5}>
            이 기간에 기록된 AI 호출이 없습니다.
          </Typography>
        )}
      </Stack>
    </Box>
  );
}

function Metric({
  label,
  value,
  detail,
  tone,
}: {
  label: string;
  value: string;
  detail?: string;
  tone?: string;
}) {
  return (
    <Grid size={{ xs: 6, sm: 4, md: 2 }}>
      <Card sx={{ p: 2, height: "100%" }}>
        <Typography variant="caption" color="text.secondary">
          {label}
        </Typography>
        <Typography variant="h3" sx={{ mt: 0.5, color: tone }}>
          {value}
        </Typography>
        {detail && (
          <Typography variant="caption" color="text.secondary">
            {detail}
          </Typography>
        )}
      </Card>
    </Grid>
  );
}
