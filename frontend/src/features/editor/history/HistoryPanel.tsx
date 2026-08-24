import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Box,
  Button,
  Chip,
  Paper,
  Stack,
  Tooltip,
  Typography,
} from "@mui/material";
import { CompareArrows, History } from "@mui/icons-material";
import { api, formatDate } from "../../../lib/api";
import type { DocumentItem, RevisionItem } from "../../../types";
import { RevisionDiffView } from "./RevisionDiffView";

export function HistoryPanel({ document }: { document: DocumentItem }) {
  const client = useQueryClient();
  // Two picked revisions are compared; picking a third replaces the older one,
  // so the list never needs a separate "clear" step.
  const [picked, setPicked] = useState<number[]>([]);
  const query = useQuery({
    queryKey: ["revisions", document.id],
    queryFn: () =>
      api<RevisionItem[]>(`/api/v1/documents/${document.id}/revisions`),
  });
  const restore = useMutation({
    mutationFn: (revision: number) =>
      api(`/api/v1/documents/${document.id}/revisions/${revision}/restore`, {
        method: "POST",
      }),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: ["document", document.id] });
      window.location.reload();
    },
  });

  const comparison = useMemo(() => {
    if (picked.length !== 2) return null;
    const [first, second] = picked as [number, number];
    // The API always reads older to newer, whichever order they were picked in.
    return first < second
      ? { from: first, to: second }
      : { from: second, to: first };
  }, [picked]);

  const toggle = (revision: number) =>
    setPicked((current) => {
      if (current.includes(revision))
        return current.filter((item) => item !== revision);
      return [...current, revision].slice(-2);
    });

  return (
    <Stack gap={1}>
      <Stack direction="row" alignItems="center" justifyContent="space-between" mb={1}>
        <Typography variant="h3">버전 기록</Typography>
        {picked.length > 0 && (
          <Button size="small" color="inherit" onClick={() => setPicked([])}>
            선택 해제
          </Button>
        )}
      </Stack>

      {comparison ? (
        <RevisionDiffView
          documentId={document.id}
          from={comparison.from}
          to={comparison.to}
        />
      ) : (
        <Typography variant="caption" color="text.secondary">
          두 버전을 선택하면 어떤 블록이 바뀌었는지 비교합니다.
        </Typography>
      )}

      {(query.data ?? []).map((item) => {
        const selected = picked.includes(item.revision);
        return (
          <Paper
            variant="outlined"
            key={item.id}
            sx={{
              p: 1.5,
              borderColor: selected ? "primary.main" : undefined,
              borderWidth: selected ? 2 : 1,
            }}
          >
            <Stack direction="row" justifyContent="space-between" alignItems="center">
              <Box>
                <Stack direction="row" gap={0.75} alignItems="center">
                  <Typography fontWeight={700}>Revision {item.revision}</Typography>
                  {item.revision === document.revision && (
                    <Chip size="small" label="현재" />
                  )}
                </Stack>
                <Typography variant="body2" color="text.secondary">
                  {item.author.displayName} · {formatDate(item.createdAt)}
                </Typography>
                <Typography variant="caption" color="text.secondary">
                  {item.reason}
                </Typography>
              </Box>
              <Stack direction="row" gap={0.5}>
                <Tooltip title={selected ? "비교에서 제외" : "비교할 버전으로 선택"}>
                  <Button
                    size="small"
                    variant={selected ? "contained" : "outlined"}
                    onClick={() => toggle(item.revision)}
                    startIcon={<CompareArrows />}
                  >
                    비교
                  </Button>
                </Tooltip>
                {item.revision !== document.revision &&
                  document.permission !== "VIEWER" && (
                    <Button
                      size="small"
                      startIcon={<History />}
                      onClick={() => restore.mutate(item.revision)}
                    >
                      복원
                    </Button>
                  )}
              </Stack>
            </Stack>
          </Paper>
        );
      })}
    </Stack>
  );
}
