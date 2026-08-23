import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { KeyOutlined, LockResetOutlined, Search } from "@mui/icons-material";
import {
  Alert,
  Box,
  Button,
  Card,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControl,
  InputAdornment,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  TextField,
  Typography,
} from "@mui/material";
import { useState } from "react";
import { api, errorMessage, formatDate, jsonBody } from "../../lib/api";
import type { User } from "../../types";
type AdminUser = User & { lastLoginAt?: string };
type PersonalKey = {
  id: string;
  name: string;
  fingerprint: string;
  status: "ACTIVE" | "RETIRED" | "REVOKED";
  version: number;
  createdAt: string;
};
export function AdminUsersPage() {
  const [q, setQ] = useState("");
  const [keyUser, setKeyUser] = useState<AdminUser | null>(null);
  const client = useQueryClient();
  const query = useQuery({
    queryKey: ["admin-users", q],
    queryFn: () =>
      api<AdminUser[]>(`/api/v1/admin/users?q=${encodeURIComponent(q)}`),
  });
  const update = useMutation({
    mutationFn: ({ id, patch }: { id: string; patch: Partial<AdminUser> }) =>
      api(`/api/v1/admin/users/${id}`, { method: "PATCH", ...jsonBody(patch) }),
    onSuccess: () => client.invalidateQueries({ queryKey: ["admin-users"] }),
  });
  const userKeys = useQuery({
    queryKey: ["admin-user-keys", keyUser?.id],
    queryFn: () =>
      api<PersonalKey[]>(`/api/v1/admin/users/${keyUser?.id}/keys`),
    enabled: Boolean(keyUser),
  });
  const rotateKey = useMutation({
    mutationFn: () =>
      api(`/api/v1/admin/users/${keyUser?.id}/keys/rotate`, {
        method: "POST",
        ...jsonBody({ name: "관리자 회전 키" }),
      }),
    onSuccess: () =>
      client.invalidateQueries({
        queryKey: ["admin-user-keys", keyUser?.id],
      }),
  });
  const revokeKey = useMutation({
    mutationFn: (keyId: string) =>
      api(`/api/v1/admin/users/${keyUser?.id}/keys/${keyId}`, {
        method: "DELETE",
      }),
    onSuccess: () =>
      client.invalidateQueries({
        queryKey: ["admin-user-keys", keyUser?.id],
      }),
  });
  return (
    <Box sx={{ p: { xs: 2.5, sm: 4, lg: 5 }, maxWidth: 1200, mx: "auto" }}>
      <Typography variant="h1">사용자 관리</Typography>
      <Typography color="text.secondary" mt={0.7} mb={3}>
        OIDC 자동 생성 사용자와 로컬 관리자를 함께 관리합니다.
      </Typography>
      <TextField
        value={q}
        onChange={(e) => setQ(e.target.value)}
        placeholder="이름, 이메일, 아이디 검색"
        sx={{ width: "100%", maxWidth: 520, mb: 3 }}
        InputProps={{
          startAdornment: (
            <InputAdornment position="start">
              <Search />
            </InputAdornment>
          ),
        }}
      />
      {update.error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {errorMessage(update.error)}
        </Alert>
      )}
      <Stack gap={1.25}>
        {(query.data ?? []).map((user) => (
          <Card key={user.id} sx={{ p: 2 }}>
            <Stack
              direction={{ xs: "column", md: "row" }}
              alignItems={{ md: "center" }}
              gap={2}
            >
              <Box sx={{ flex: 1, minWidth: 0 }}>
                <Stack direction="row" gap={1} alignItems="center">
                  <Typography fontWeight={720}>{user.displayName}</Typography>
                  <Chip size="small" label={user.username} />
                </Stack>
                <Typography variant="body2" color="text.secondary" mt={0.4}>
                  {user.email} · 최근 로그인 {formatDate(user.lastLoginAt)}
                </Typography>
              </Box>
              <FormControl size="small" sx={{ minWidth: 130 }}>
                <InputLabel>역할</InputLabel>
                <Select
                  label="역할"
                  value={user.role}
                  onChange={(e) =>
                    update.mutate({
                      id: user.id,
                      patch: { role: e.target.value as User["role"] },
                    })
                  }
                >
                  <MenuItem value="USER">USER</MenuItem>
                  <MenuItem value="ADMIN">ADMIN</MenuItem>
                </Select>
              </FormControl>
              <Button
                variant="outlined"
                startIcon={<KeyOutlined />}
                onClick={() => setKeyUser(user)}
              >
                키 관리
              </Button>
              <FormControl size="small" sx={{ minWidth: 140 }}>
                <InputLabel>계정 상태</InputLabel>
                <Select
                  label="계정 상태"
                  value={user.status}
                  onChange={(e) =>
                    update.mutate({
                      id: user.id,
                      patch: { status: e.target.value as User["status"] },
                    })
                  }
                >
                  <MenuItem value="ACTIVE">활성</MenuItem>
                  <MenuItem value="SUSPENDED">정지</MenuItem>
                </Select>
              </FormControl>
            </Stack>
          </Card>
        ))}
      </Stack>
      <Dialog
        open={Boolean(keyUser)}
        onClose={() => setKeyUser(null)}
        fullWidth
        maxWidth="sm"
      >
        <DialogTitle>{keyUser?.displayName} · 개인 키 관리</DialogTitle>
        <DialogContent>
          {(userKeys.error || rotateKey.error || revokeKey.error) && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {errorMessage(
                userKeys.error || rotateKey.error || revokeKey.error,
              )}
            </Alert>
          )}
          <Button
            variant="contained"
            startIcon={<LockResetOutlined />}
            disabled={rotateKey.isPending}
            onClick={() => rotateKey.mutate()}
          >
            활성 키 회전
          </Button>
          <Stack divider={<Divider />} mt={2}>
            {(userKeys.data ?? []).map((key) => (
              <Stack
                key={key.id}
                direction="row"
                alignItems="center"
                justifyContent="space-between"
                gap={1}
                py={1.25}
              >
                <Box minWidth={0}>
                  <Stack direction="row" gap={1} alignItems="center">
                    <Typography fontWeight={700}>{key.name}</Typography>
                    <Chip size="small" label={key.status} />
                  </Stack>
                  <Typography variant="body2" color="text.secondary" noWrap>
                    v{key.version} · {key.fingerprint} ·{" "}
                    {formatDate(key.createdAt)}
                  </Typography>
                </Box>
                {key.status === "RETIRED" && (
                  <Button
                    size="small"
                    color="error"
                    onClick={() => revokeKey.mutate(key.id)}
                  >
                    폐기
                  </Button>
                )}
              </Stack>
            ))}
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setKeyUser(null)}>닫기</Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
