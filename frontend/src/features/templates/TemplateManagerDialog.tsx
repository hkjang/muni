import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Stack,
  TextField,
  Typography,
} from "@mui/material";
import { DeleteOutline, EditOutlined } from "@mui/icons-material";
import { api, errorMessage, formatDate, jsonBody } from "../../lib/api";
import type { Template } from "../../types";

/**
 * Renaming and removing the templates a workspace has collected.
 *
 * Saving a document as a template was one click and the only one: no screen
 * could rename or delete one, though the endpoints for both existed. A
 * workspace kept every template anyone had ever saved, typos included, in the
 * dropdown everybody sees when they start a document.
 */
export function TemplateManagerDialog({
  open,
  onClose,
  workspaceId,
}: {
  open: boolean;
  onClose: () => void;
  workspaceId: string;
}) {
  const client = useQueryClient();
  const [editing, setEditing] = useState<Template | null>(null);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [confirming, setConfirming] = useState<Template | null>(null);

  const templates = useQuery({
    queryKey: ["templates", workspaceId],
    queryFn: () =>
      api<Template[]>(`/api/v1/workspaces/${workspaceId}/templates`),
    enabled: open && Boolean(workspaceId),
  });

  const invalidate = () =>
    client.invalidateQueries({ queryKey: ["templates", workspaceId] });

  const rename = useMutation({
    mutationFn: () =>
      api(`/api/v1/templates/${editing?.id}`, {
        method: "PATCH",
        ...jsonBody({ name: name.trim(), description: description.trim() }),
      }),
    onSuccess: () => {
      setEditing(null);
      void invalidate();
    },
  });

  const remove = useMutation({
    mutationFn: (id: string) =>
      api(`/api/v1/templates/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      setConfirming(null);
      void invalidate();
    },
  });

  const items = templates.data ?? [];

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>서식 관리</DialogTitle>
      <DialogContent>
        <Typography variant="body2" color="text.secondary" mb={2}>
          새 문서를 만들 때 고를 수 있는 서식입니다. 공용 서식은 서비스
          관리자만 바꿀 수 있습니다.
        </Typography>

        {items.length === 0 && (
          <Typography variant="body2" color="text.secondary">
            아직 서식이 없습니다. 문서를 열고 <b>서식으로 저장</b>하면 여기에
            생깁니다.
          </Typography>
        )}

        <Stack gap={0.5}>
          {items.map((template) => (
            <Stack
              key={template.id}
              direction="row"
              gap={1}
              alignItems="center"
              sx={{ py: 1, borderBottom: 1, borderColor: "divider" }}
            >
              <Stack sx={{ flex: 1, minWidth: 0 }}>
                <Stack direction="row" gap={0.8} alignItems="center">
                  <Typography variant="body2" fontWeight={640} noWrap>
                    {template.name}
                  </Typography>
                  {!template.workspaceId && (
                    <Chip size="small" variant="outlined" label="공용" />
                  )}
                </Stack>
                <Typography variant="caption" color="text.secondary" noWrap>
                  {template.description || "설명 없음"} ·{" "}
                  {formatDate(template.updatedAt, false)}
                </Typography>
              </Stack>
              <Button
                size="small"
                startIcon={<EditOutlined />}
                onClick={() => {
                  setEditing(template);
                  setName(template.name);
                  setDescription(template.description);
                  rename.reset();
                }}
              >
                이름
              </Button>
              <Button
                size="small"
                color="error"
                startIcon={<DeleteOutline />}
                onClick={() => setConfirming(template)}
              >
                삭제
              </Button>
            </Stack>
          ))}
        </Stack>

        {remove.error && (
          <Alert severity="error" sx={{ mt: 2 }}>
            {errorMessage(remove.error)}
          </Alert>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>닫기</Button>
      </DialogActions>

      <Dialog
        open={Boolean(editing)}
        onClose={() => setEditing(null)}
        fullWidth
        maxWidth="xs"
      >
        <DialogTitle>서식 이름 바꾸기</DialogTitle>
        <DialogContent>
          <Stack gap={2} mt={1}>
            <TextField
              label="이름"
              value={name}
              onChange={(event) => setName(event.target.value)}
              autoFocus
              fullWidth
            />
            <TextField
              label="설명 (선택)"
              value={description}
              onChange={(event) => setDescription(event.target.value)}
              fullWidth
            />
            {rename.error && (
              <Alert severity="error">{errorMessage(rename.error)}</Alert>
            )}
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setEditing(null)}>취소</Button>
          <Button
            variant="contained"
            disabled={!name.trim() || rename.isPending}
            onClick={() => rename.mutate()}
          >
            저장
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog open={Boolean(confirming)} onClose={() => setConfirming(null)}>
        <DialogTitle>{confirming?.name}을(를) 삭제할까요?</DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary">
            이 서식으로 이미 만든 문서는 그대로 남습니다. 앞으로 새 문서를 만들
            때 고를 수 없게 될 뿐입니다.
          </Typography>
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
    </Dialog>
  );
}
