import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Button,
  Chip,
  CircularProgress,
  Paper,
  Stack,
  Tooltip,
  Typography,
} from "@mui/material";
import {
  DeleteOutline,
  DownloadOutlined,
  FormatQuote,
  OpenInNew,
  Refresh,
} from "@mui/icons-material";
import { api, errorMessage, formatDate } from "../../../lib/api";
import { isBusy, statusLabel, type PresentationLink } from "./types";
import { PresentationSync } from "./PresentationSync";

/**
 * PresentationList shows the decks made from this document. Generation happens
 * on the presentation service, so the list polls while anything is still being
 * built and stops as soon as everything has settled.
 */
export function PresentationList({
  documentId,
  canEdit,
}: {
  documentId: string;
  canEdit: boolean;
}) {
  const client = useQueryClient();
  const query = useQuery({
    queryKey: ["presentations", documentId],
    queryFn: () =>
      api<PresentationLink[]>(`/api/v1/documents/${documentId}/presentations`),
    refetchInterval: (query) =>
      (query.state.data ?? []).some((item) => isBusy(item.status)) ? 4000 : false,
  });

  const refresh = useMutation({
    mutationFn: (item: PresentationLink) =>
      api(
        `/api/v1/documents/${documentId}/presentations/${item.id}/status`,
      ),
    onSuccess: () =>
      client.invalidateQueries({ queryKey: ["presentations", documentId] }),
  });

  // Writing the document and revision onto each slide, so a deck can say where
  // its numbers came from.
  const cite = useMutation({
    mutationFn: (item: PresentationLink) =>
      api<{ added: number }>(
        `/api/v1/documents/${documentId}/presentations/${item.id}/citations`,
        { method: "POST" },
      ),
    onSuccess: () =>
      client.invalidateQueries({ queryKey: ["presentations", documentId] }),
  });

  const unlink = useMutation({
    mutationFn: (item: PresentationLink) =>
      api(`/api/v1/documents/${documentId}/presentations/${item.id}`, {
        method: "DELETE",
      }),
    onSuccess: () =>
      client.invalidateQueries({ queryKey: ["presentations", documentId] }),
  });

  const items = query.data ?? [];
  if (query.isPending)
    return (
      <Stack alignItems="center" py={2}>
        <CircularProgress size={20} />
      </Stack>
    );
  if (query.error)
    return <Alert severity="error">{errorMessage(query.error)}</Alert>;
  if (items.length === 0)
    return (
      <Typography variant="body2" color="text.secondary">
        아직 만든 발표자료가 없습니다.
      </Typography>
    );

  return (
    <Stack gap={1}>
      {unlink.error && <Alert severity="error">{errorMessage(unlink.error)}</Alert>}
      {cite.error && <Alert severity="error">{errorMessage(cite.error)}</Alert>}
      {cite.isSuccess && (
        <Alert severity="info">
          {cite.data.added > 0
            ? `${cite.data.added}장에 출처를 표시했습니다.`
            : "출처를 표시할 슬라이드를 찾지 못했습니다."}
        </Alert>
      )}
      {items.map((item) => (
        <Paper key={item.id} variant="outlined" sx={{ p: 1.5 }}>
          <Stack direction="row" justifyContent="space-between" alignItems="flex-start" gap={1}>
            <div>
              <Typography fontWeight={700}>{item.title}</Typography>
              <Stack direction="row" gap={0.5} alignItems="center" mt={0.5} flexWrap="wrap">
                <Chip
                  size="small"
                  color={
                    item.status === "completed"
                      ? "success"
                      : item.status === "failed"
                        ? "error"
                        : "default"
                  }
                  label={statusLabel(item.status)}
                />
                {item.slideCount > 0 && (
                  <Chip size="small" variant="outlined" label={`${item.slideCount}장`} />
                )}
                {item.stale && (
                  <Tooltip title="이 발표자료를 만든 뒤 문서가 수정되었습니다.">
                    <Chip size="small" color="warning" label="문서 변경됨" />
                  </Tooltip>
                )}
              </Stack>
              <Typography variant="caption" color="text.secondary" display="block" mt={0.5}>
                Revision {item.documentRevision} · {formatDate(item.createdAt)}
              </Typography>
            </div>
            {isBusy(item.status) && <CircularProgress size={18} />}
          </Stack>

          <Stack direction="row" gap={0.5} mt={1.25} flexWrap="wrap">
            {item.editorUrl && (
              <Button
                size="small"
                startIcon={<OpenInNew />}
                component="a"
                href={item.editorUrl}
                target="_blank"
                rel="noopener noreferrer"
              >
                편집
              </Button>
            )}
            <Button
              size="small"
              startIcon={<DownloadOutlined />}
              disabled={item.status !== "completed"}
              component="a"
              href={`/api/v1/documents/${documentId}/presentations/${item.id}/download?format=pptx`}
            >
              PPTX
            </Button>
            <Button
              size="small"
              startIcon={<Refresh />}
              disabled={refresh.isPending}
              onClick={() => refresh.mutate(item)}
            >
              상태 확인
            </Button>
            {canEdit && item.status === "completed" && (
              <Tooltip title="각 슬라이드에 근거가 된 문서와 버전을 표시합니다.">
                <Button
                  size="small"
                  startIcon={<FormatQuote />}
                  disabled={cite.isPending}
                  onClick={() => cite.mutate(item)}
                >
                  출처 표시
                </Button>
              </Tooltip>
            )}
            {item.stale && item.status === "completed" && (
              <PresentationSync
                documentId={documentId}
                linkId={item.id}
                canEdit={canEdit}
                onDone={() =>
                  client.invalidateQueries({
                    queryKey: ["presentations", documentId],
                  })
                }
              />
            )}
            {canEdit && (
              <Button
                size="small"
                color="error"
                startIcon={<DeleteOutline />}
                disabled={unlink.isPending}
                onClick={() => unlink.mutate(item)}
              >
                삭제
              </Button>
            )}
          </Stack>
        </Paper>
      ))}
    </Stack>
  );
}
