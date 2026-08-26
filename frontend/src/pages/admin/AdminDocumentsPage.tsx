import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Card,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  InputAdornment,
  MenuItem,
  Stack,
  TextField,
  Typography,
} from "@mui/material";
import {
  DeleteForeverOutlined,
  SearchOutlined,
  SwapHorizOutlined,
} from "@mui/icons-material";
import { api, errorMessage, formatDate, jsonBody } from "../../lib/api";
import type { User } from "../../types";

type AdminDocument = {
  id: string;
  title: string;
  workspaceId: string;
  workspaceName: string;
  ownerId: string;
  ownerName: string;
  status: string;
  workflowStatus: string;
  revision: number;
  updatedAt: string;
  deletedAt?: string;
};

const scopes = [
  { value: "active", label: "사용 중" },
  { value: "trashed", label: "휴지통" },
  { value: "all", label: "전체" },
];

/**
 * AdminDocumentsPage finds a document anywhere in the system.
 *
 * Every other route to a document goes through a permission check, so the
 * questions an operator is handed — who owns the one nobody can open, what is
 * still sitting in the trash — had no screen that could answer them.
 */
export function AdminDocumentsPage() {
  const client = useQueryClient();
  const [search, setSearch] = useState("");
  const [scope, setScope] = useState("active");
  const [transfer, setTransfer] = useState<AdminDocument | null>(null);
  const [purge, setPurge] = useState<AdminDocument | null>(null);
  const [newOwner, setNewOwner] = useState<User | null>(null);
  const [ownerQuery, setOwnerQuery] = useState("");

  const list = useQuery({
    queryKey: ["admin-documents", search, scope],
    queryFn: () =>
      api<AdminDocument[]>(
        `/api/v1/admin/documents?limit=100&scope=${scope}${search.trim() ? `&q=${encodeURIComponent(search.trim())}` : ""}`,
      ),
  });

  const candidates = useQuery({
    queryKey: ["admin-users", "transfer", ownerQuery],
    queryFn: () =>
      api<User[]>(`/api/v1/admin/users?q=${encodeURIComponent(ownerQuery)}&limit=20`),
    enabled: Boolean(transfer),
  });

  const refresh = () =>
    client.invalidateQueries({ queryKey: ["admin-documents"] });

  const runTransfer = useMutation({
    mutationFn: () =>
      api(`/api/v1/admin/documents/${transfer?.id}/transfer`, {
        method: "POST",
        ...jsonBody({ ownerId: newOwner?.id }),
      }),
    onSuccess: () => {
      setTransfer(null);
      setNewOwner(null);
      void refresh();
    },
  });

  const runPurge = useMutation({
    mutationFn: () =>
      api<void>(`/api/v1/admin/documents/${purge?.id}`, { method: "DELETE" }),
    onSuccess: () => {
      setPurge(null);
      void refresh();
    },
  });

  return (
    <Box sx={{ p: { xs: 2.5, sm: 4, lg: 5 }, maxWidth: 1200, mx: "auto" }}>
      <Typography variant="h1">문서 관리</Typography>
      <Typography color="text.secondary" mt={0.7} mb={3}>
        소유자가 떠난 문서를 넘기고, 휴지통에 있는 문서를 완전히 지웁니다.
      </Typography>

      <Stack direction="row" gap={1.5} mb={3} flexWrap="wrap">
        <TextField
          size="small"
          placeholder="제목 또는 소유자로 찾기"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          sx={{ minWidth: 300 }}
          InputProps={{
            startAdornment: (
              <InputAdornment position="start">
                <SearchOutlined fontSize="small" />
              </InputAdornment>
            ),
          }}
        />
        <TextField
          select
          size="small"
          label="범위"
          value={scope}
          onChange={(event) => setScope(event.target.value)}
          sx={{ minWidth: 130 }}
        >
          {scopes.map((item) => (
            <MenuItem key={item.value} value={item.value}>
              {item.label}
            </MenuItem>
          ))}
        </TextField>
      </Stack>

      {list.isLoading && <CircularProgress />}

      <Stack gap={1}>
        {(list.data ?? []).map((item) => (
          <Card key={item.id} sx={{ p: 2, opacity: item.deletedAt ? 0.75 : 1 }}>
            <Stack
              direction={{ xs: "column", md: "row" }}
              alignItems={{ md: "center" }}
              gap={1.5}
            >
              <Box sx={{ flex: 1, minWidth: 0 }}>
                <Stack direction="row" gap={0.75} alignItems="center">
                  <Typography fontWeight={650} noWrap>
                    {item.title}
                  </Typography>
                  {item.deletedAt && <Chip size="small" label="휴지통" />}
                  {item.workflowStatus === "PENDING" && (
                    <Chip size="small" color="warning" label="승인 대기" />
                  )}
                </Stack>
                <Typography variant="body2" color="text.secondary" noWrap>
                  {item.workspaceName} · 소유자 {item.ownerName} · rev{" "}
                  {item.revision} · {formatDate(item.updatedAt)}
                </Typography>
              </Box>
              <Button
                size="small"
                variant="outlined"
                startIcon={<SwapHorizOutlined />}
                onClick={() => {
                  setTransfer(item);
                  setNewOwner(null);
                  setOwnerQuery("");
                }}
              >
                소유권 이전
              </Button>
              {item.deletedAt && (
                <Button
                  size="small"
                  color="error"
                  startIcon={<DeleteForeverOutlined />}
                  onClick={() => setPurge(item)}
                >
                  완전 삭제
                </Button>
              )}
            </Stack>
          </Card>
        ))}
        {list.data && list.data.length === 0 && (
          <Typography color="text.secondary" textAlign="center" py={5}>
            조건에 맞는 문서가 없습니다.
          </Typography>
        )}
      </Stack>

      <Dialog
        open={Boolean(transfer)}
        onClose={() => setTransfer(null)}
        fullWidth
        maxWidth="sm"
      >
        <DialogTitle>소유권 이전</DialogTitle>
        <DialogContent>
          {runTransfer.error && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {errorMessage(runTransfer.error)}
            </Alert>
          )}
          <Typography variant="body2" color="text.secondary" mb={2}>
            <strong>{transfer?.title}</strong>의 소유자를 바꿉니다. 새 소유자가
            워크스페이스 구성원이 아니면 함께 추가되고, 문서를 열어 두고 있던
            사람은 다시 연결됩니다.
          </Typography>
          <Autocomplete
            options={candidates.data ?? []}
            value={newOwner}
            onChange={(_, value) => setNewOwner(value)}
            onInputChange={(_, value) => setOwnerQuery(value)}
            getOptionLabel={(option) =>
              `${option.displayName} (${option.username})`
            }
            isOptionEqualToValue={(option, value) => option.id === value.id}
            renderInput={(params) => (
              <TextField {...params} autoFocus label="새 소유자" />
            )}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setTransfer(null)}>취소</Button>
          <Button
            variant="contained"
            disabled={!newOwner || runTransfer.isPending}
            onClick={() => runTransfer.mutate()}
          >
            넘기기
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog open={Boolean(purge)} onClose={() => setPurge(null)} fullWidth maxWidth="sm">
        <DialogTitle>완전 삭제</DialogTitle>
        <DialogContent>
          {runPurge.error && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {errorMessage(runPurge.error)}
            </Alert>
          )}
          <Alert severity="warning">
            <strong>{purge?.title}</strong>과 그 문서의 모든 버전, 댓글, 제안,
            첨부 파일이 함께 지워집니다. 되돌릴 수 없습니다.
          </Alert>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setPurge(null)}>취소</Button>
          <Button
            color="error"
            variant="contained"
            disabled={runPurge.isPending}
            onClick={() => runPurge.mutate()}
          >
            영구히 삭제
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
