import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Box, Button, Paper, Stack, Typography } from "@mui/material";
import { History } from "@mui/icons-material";
import { api, formatDate } from "../../../lib/api";
import type { DocumentItem, RevisionItem } from "../../../types";

export function HistoryPanel({ document }: { document: DocumentItem }) {
  const client = useQueryClient();
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
  return (
    <Stack gap={1}>
      <Typography variant="h3" mb={1}>
        버전 기록
      </Typography>
      {(query.data ?? []).map((item) => (
        <Paper variant="outlined" key={item.id} sx={{ p: 1.5 }}>
          <Stack
            direction="row"
            justifyContent="space-between"
            alignItems="center"
          >
            <Box>
              <Typography fontWeight={700}>Revision {item.revision}</Typography>
              <Typography variant="body2" color="text.secondary">
                {item.author.displayName} · {formatDate(item.createdAt)}
              </Typography>
              <Typography variant="caption" color="text.secondary">
                {item.reason}
              </Typography>
            </Box>
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
        </Paper>
      ))}
    </Stack>
  );
}
