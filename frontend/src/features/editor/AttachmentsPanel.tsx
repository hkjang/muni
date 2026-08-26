import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  Stack,
  Typography,
} from "@mui/material";
import {
  DeleteOutline,
  DownloadOutlined,
  InsertDriveFileOutlined,
} from "@mui/icons-material";
import { api, errorMessage, formatDate } from "../../lib/api";
import { humanBytes } from "../../lib/humanBytes";

type Attachment = {
  id: string;
  name: string;
  mediaType: string;
  sizeBytes: number;
  createdAt: string;
  uploadedBy: string;
  inUse: boolean;
  url: string;
};

/**
 * What this document is carrying.
 *
 * Files could be uploaded and never removed: the delete endpoint existed and
 * nothing called it. An image dragged in by mistake stayed in the database for
 * the life of the document, and nothing told anyone it was there.
 *
 * The useful distinction is not "what is attached" but "what is still being
 * used". A file deleted from the text keeps its bytes with nothing pointing at
 * them, and those are the ones worth clearing.
 */
export function AttachmentsPanel({
  documentId,
  canEdit,
}: {
  documentId: string;
  canEdit: boolean;
}) {
  const client = useQueryClient();
  const [confirming, setConfirming] = useState<Attachment | null>(null);

  const attachments = useQuery({
    queryKey: ["attachments", documentId],
    queryFn: () =>
      api<Attachment[]>(`/api/v1/documents/${documentId}/attachments`),
  });

  const remove = useMutation({
    mutationFn: (id: string) =>
      api(`/api/v1/attachments/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      setConfirming(null);
      void client.invalidateQueries({ queryKey: ["attachments", documentId] });
    },
  });

  const items = attachments.data ?? [];
  const unused = items.filter((item) => !item.inUse);
  const wasted = unused.reduce((sum, item) => sum + item.sizeBytes, 0);

  return (
    <Stack gap={1.5}>
      <Typography variant="h3">첨부파일</Typography>
      {items.length === 0 && (
        <Typography variant="body2" color="text.secondary">
          이 문서에 올린 파일이 없습니다. 본문에 이미지나 파일을 넣으면 여기에
          나타납니다.
        </Typography>
      )}

      {unused.length > 0 && (
        <Alert severity="info">
          본문에서 빠진 파일이 {unused.length}개 있습니다 ({humanBytes(wasted)}
          ). 문서에서 지워도 파일 자체는 남아 있어서, 여기서 지워야 없어집니다.
        </Alert>
      )}

      <Stack gap={0.75}>
        {items.map((item) => (
          <Stack
            key={item.id}
            direction="row"
            gap={1}
            alignItems="center"
            sx={{ py: 0.8, borderBottom: 1, borderColor: "divider" }}
          >
            <InsertDriveFileOutlined fontSize="small" color="disabled" />
            <Stack sx={{ flex: 1, minWidth: 0 }}>
              <Typography variant="body2" fontWeight={620} noWrap>
                {item.name}
              </Typography>
              <Typography variant="caption" color="text.secondary" noWrap>
                {humanBytes(item.sizeBytes)}
                {item.uploadedBy ? ` · ${item.uploadedBy}` : ""} ·{" "}
                {formatDate(item.createdAt, false)}
              </Typography>
            </Stack>
            {!item.inUse && (
              <Chip size="small" variant="outlined" label="본문에 없음" />
            )}
            <Button
              size="small"
              startIcon={<DownloadOutlined />}
              href={item.url}
              download={item.name}
            >
              받기
            </Button>
            {canEdit && (
              <Button
                size="small"
                color="error"
                startIcon={<DeleteOutline />}
                onClick={() => setConfirming(item)}
              >
                삭제
              </Button>
            )}
          </Stack>
        ))}
      </Stack>

      {remove.error && (
        <Alert severity="error">{errorMessage(remove.error)}</Alert>
      )}

      <Dialog open={Boolean(confirming)} onClose={() => setConfirming(null)}>
        <DialogTitle>{confirming?.name}을(를) 삭제할까요?</DialogTitle>
        <DialogContent>
          <DialogContentText component="div">
            {confirming?.inUse ? (
              // Deleting a file the text still points at leaves a broken image
              // in the document, so it is worth saying before, not after.
              <Alert severity="warning" sx={{ mb: 1 }}>
                이 파일은 <b>본문에서 아직 쓰이고 있습니다.</b> 삭제하면 그
                자리가 깨진 이미지로 남습니다.
              </Alert>
            ) : (
              <Typography variant="body2" color="text.secondary" mb={1}>
                본문에서는 이미 빠진 파일입니다.
              </Typography>
            )}
            되돌릴 수 없습니다.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirming(null)}>취소</Button>
          <Button
            color="error"
            variant="contained"
            disabled={remove.isPending}
            onClick={() => confirming && remove.mutate(confirming.id)}
          >
            {remove.isPending ? "삭제 중…" : "삭제"}
          </Button>
        </DialogActions>
      </Dialog>
    </Stack>
  );
}
