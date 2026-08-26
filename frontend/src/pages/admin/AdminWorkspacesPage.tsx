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
  ArchiveOutlined,
  SearchOutlined,
  SwapHorizOutlined,
  UnarchiveOutlined,
} from "@mui/icons-material";
import { api, errorMessage, formatDate, jsonBody } from "../../lib/api";
import type { User } from "../../types";

type AdminWorkspace = {
  id: string;
  name: string;
  slug: string;
  kind: string;
  ownerName: string;
  members: number;
  documents: number;
  lastEditedAt?: string;
  createdAt: string;
  deletedAt?: string;
};

const scopes = [
  { value: "active", label: "사용 중" },
  { value: "archived", label: "정리됨" },
  { value: "all", label: "전체" },
];

const kindLabels: Record<string, string> = {
  PERSONAL: "개인",
  TEAM: "팀",
  DEPARTMENT: "부서",
  ORGANIZATION: "조직",
};

/**
 * AdminWorkspacesPage lists every workspace in the system.
 *
 * An administrator could only see the workspaces they were a member of, which
 * is usually a handful of them — so the questions an operator gets asked, such
 * as which workspaces are dormant or who owns one nobody can get into, had no
 * screen that could answer them.
 */
export function AdminWorkspacesPage() {
  const client = useQueryClient();
  const [query, setQuery] = useState("");
  const [scope, setScope] = useState("active");
  const [transfer, setTransfer] = useState<AdminWorkspace | null>(null);
  const [archive, setArchive] = useState<AdminWorkspace | null>(null);
  const [newOwner, setNewOwner] = useState<User | null>(null);
  const [ownerQuery, setOwnerQuery] = useState("");

  const list = useQuery({
    queryKey: ["admin-workspaces", query, scope],
    queryFn: () =>
      api<AdminWorkspace[]>(
        `/api/v1/admin/workspaces?limit=100&scope=${scope}${query.trim() ? `&q=${encodeURIComponent(query.trim())}` : ""}`,
      ),
  });
  const candidates = useQuery({
    queryKey: ["admin-users", "ws-transfer", ownerQuery],
    queryFn: () =>
      api<User[]>(`/api/v1/admin/users?q=${encodeURIComponent(ownerQuery)}&limit=20`),
    enabled: Boolean(transfer),
  });
  const refresh = () =>
    client.invalidateQueries({ queryKey: ["admin-workspaces"] });

  const runTransfer = useMutation({
    mutationFn: () =>
      api(`/api/v1/admin/workspaces/${transfer?.id}/transfer`, {
        method: "POST",
        ...jsonBody({ ownerId: newOwner?.id }),
      }),
    onSuccess: () => {
      setTransfer(null);
      setNewOwner(null);
      void refresh();
    },
  });
  const runArchive = useMutation({
    mutationFn: () =>
      api<{ documents: number }>(`/api/v1/admin/workspaces/${archive?.id}`, {
        method: "DELETE",
      }),
    onSuccess: () => {
      setArchive(null);
      void refresh();
    },
  });
  const runRestore = useMutation({
    mutationFn: (id: string) =>
      api(`/api/v1/admin/workspaces/${id}/restore`, { method: "POST" }),
    onSuccess: () => void refresh(),
  });

  return (
    <Box sx={{ p: { xs: 2.5, sm: 4, lg: 5 }, maxWidth: 1200, mx: "auto" }}>
      <Typography variant="h1">워크스페이스</Typography>
      <Typography color="text.secondary" mt={0.7} mb={3}>
        문서 수와 마지막 편집 시각으로 쓰이고 있는 곳과 비어 있는 곳을
        구분합니다.
      </Typography>

      {(runArchive.error || runRestore.error) && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {errorMessage(runArchive.error || runRestore.error)}
        </Alert>
      )}

      <Stack direction="row" gap={1.5} mb={3} flexWrap="wrap">
        <TextField
          size="small"
          placeholder="이름 또는 slug로 찾기"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          sx={{ minWidth: 320 }}
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
        {(list.data ?? []).map((workspace) => (
          <Card key={workspace.id} sx={{ p: 2 }}>
            <Stack
              direction={{ xs: "column", sm: "row" }}
              alignItems={{ sm: "center" }}
              gap={1.5}
            >
              <Chip size="small" label={kindLabels[workspace.kind] ?? workspace.kind} />
              <Box sx={{ flex: 1, minWidth: 0 }}>
                <Typography fontWeight={650} noWrap>
                  {workspace.deletedAt && (
                    <Chip size="small" label="정리됨" sx={{ mr: 0.75 }} />
                  )}
                  {workspace.name}
                  <Typography component="span" variant="body2" color="text.secondary">
                    {" "}
                    /{workspace.slug}
                  </Typography>
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  소유자 {workspace.ownerName} · 구성원 {workspace.members}명 · 문서{" "}
                  {workspace.documents}개
                </Typography>
              </Box>
              <Typography variant="body2" color="text.secondary">
                {workspace.lastEditedAt
                  ? `마지막 편집 ${formatDate(workspace.lastEditedAt)}`
                  : "편집된 문서 없음"}
              </Typography>
              {workspace.deletedAt ? (
                <Button
                  size="small"
                  startIcon={<UnarchiveOutlined />}
                  onClick={() => runRestore.mutate(workspace.id)}
                >
                  되돌리기
                </Button>
              ) : (
                workspace.kind !== "PERSONAL" && (
                  <>
                    <Button
                      size="small"
                      variant="outlined"
                      startIcon={<SwapHorizOutlined />}
                      onClick={() => {
                        setTransfer(workspace);
                        setNewOwner(null);
                        setOwnerQuery("");
                      }}
                    >
                      소유권 이전
                    </Button>
                    <Button
                      size="small"
                      color="error"
                      startIcon={<ArchiveOutlined />}
                      onClick={() => setArchive(workspace)}
                    >
                      정리
                    </Button>
                  </>
                )
              )}
            </Stack>
          </Card>
        ))}
        {list.data && list.data.length === 0 && (
          <Typography color="text.secondary" textAlign="center" py={5}>
            찾는 워크스페이스가 없습니다.
          </Typography>
        )}
      </Stack>

      <Dialog
        open={Boolean(transfer)}
        onClose={() => setTransfer(null)}
        fullWidth
        maxWidth="sm"
      >
        <DialogTitle>워크스페이스 소유권 이전</DialogTitle>
        <DialogContent>
          {runTransfer.error && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {errorMessage(runTransfer.error)}
            </Alert>
          )}
          <Typography variant="body2" color="text.secondary" mb={2}>
            <strong>{transfer?.name}</strong>의 소유자를 바꿉니다. 새 소유자는
            워크스페이스 구성원으로 추가되고 OWNER 역할을 받습니다.
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

      <Dialog
        open={Boolean(archive)}
        onClose={() => setArchive(null)}
        fullWidth
        maxWidth="sm"
      >
        <DialogTitle>워크스페이스 정리</DialogTitle>
        <DialogContent>
          <Alert severity="warning">
            <strong>{archive?.name}</strong>을 사용 중단합니다. 문서{" "}
            {archive?.documents ?? 0}개는 <strong>휴지통으로</strong> 들어가며
            지워지지 않습니다. 워크스페이스와 문서 모두 되돌릴 수 있습니다.
          </Alert>
          <Typography variant="body2" color="text.secondary" mt={2}>
            승인 대기 중인 문서가 있으면 정리되지 않습니다. 검토는 다른 사람이
            내릴 결정이기 때문입니다.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setArchive(null)}>취소</Button>
          <Button
            color="error"
            variant="contained"
            disabled={runArchive.isPending}
            onClick={() => runArchive.mutate()}
          >
            정리하기
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
