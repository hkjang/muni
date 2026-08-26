import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  DevicesOutlined,
  KeyOutlined,
  LockResetOutlined,
  Search,
} from "@mui/icons-material";
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
type AdminSession = {
  createdAt: string;
  lastSeenAt: string;
  expiresAt: string;
  ip?: unknown;
  userAgent?: string;
};
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
  const [sessionUser, setSessionUser] = useState<AdminUser | null>(null);
  const [passwordUser, setPasswordUser] = useState<AdminUser | null>(null);
  const [password, setPassword] = useState("");
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
  const resetPassword = useMutation({
    mutationFn: () =>
      api<{ sessionsEnded: number }>(
        `/api/v1/admin/users/${passwordUser?.id}/password`,
        { method: "POST", ...jsonBody({ password }) },
      ),
    onSuccess: () => {
      setPasswordUser(null);
      setPassword("");
    },
  });
  const sessions = useQuery({
    queryKey: ["admin-user-sessions", sessionUser?.id],
    queryFn: () =>
      api<AdminSession[]>(`/api/v1/admin/users/${sessionUser?.id}/sessions`),
    enabled: Boolean(sessionUser),
  });
  const revokeSessions = useMutation({
    mutationFn: () =>
      api<{ revoked: number }>(
        `/api/v1/admin/users/${sessionUser?.id}/sessions`,
        { method: "DELETE" },
      ),
    onSuccess: () =>
      client.invalidateQueries({ queryKey: ["admin-user-sessions"] }),
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
                startIcon={<LockResetOutlined />}
                onClick={() => {
                  setPasswordUser(user);
                  setPassword("");
                  resetPassword.reset();
                }}
              >
                비밀번호
              </Button>
              <Button
                variant="outlined"
                startIcon={<DevicesOutlined />}
                onClick={() => setSessionUser(user)}
              >
                로그인 세션
              </Button>
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
        open={Boolean(passwordUser)}
        onClose={() => setPasswordUser(null)}
        fullWidth
        maxWidth="sm"
      >
        <DialogTitle>{passwordUser?.displayName} · 비밀번호 초기화</DialogTitle>
        <DialogContent>
          {resetPassword.error && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {errorMessage(resetPassword.error)}
            </Alert>
          )}
          <Typography variant="body2" color="text.secondary" mb={2}>
            새 비밀번호를 정해 본인에게 직접 전달하세요. 이 계정의 열린 세션은
            모두 종료됩니다. SSO로 로그인하는 계정에 비밀번호를 지정하면 로컬
            로그인도 가능해집니다.
          </Typography>
          <TextField
            autoFocus
            fullWidth
            type="password"
            label="새 비밀번호"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            helperText="12자 이상"
            onKeyDown={(event) => {
              if (event.key === "Enter" && password.length >= 12)
                resetPassword.mutate();
            }}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setPasswordUser(null)}>취소</Button>
          <Button
            variant="contained"
            disabled={password.length < 12 || resetPassword.isPending}
            onClick={() => resetPassword.mutate()}
          >
            초기화
          </Button>
        </DialogActions>
      </Dialog>
      <Dialog
        open={Boolean(sessionUser)}
        onClose={() => setSessionUser(null)}
        fullWidth
        maxWidth="sm"
      >
        <DialogTitle>{sessionUser?.displayName} · 로그인 세션</DialogTitle>
        <DialogContent>
          {revokeSessions.error && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {errorMessage(revokeSessions.error)}
            </Alert>
          )}
          {revokeSessions.isSuccess && (
            <Alert severity="success" sx={{ mb: 2 }}>
              세션 {revokeSessions.data?.revoked ?? 0}개를 종료했습니다.
            </Alert>
          )}
          <Typography variant="body2" color="text.secondary" mb={2}>
            계정을 정지하면 다음 요청부터 바로 막힙니다. 이 기능은 계정은
            그대로 두고 기기에서만 로그아웃시킬 때 사용합니다.
          </Typography>
          <Stack gap={1.25}>
            {(sessions.data ?? []).map((session, index) => (
              <Card key={index} variant="outlined" sx={{ p: 1.5 }}>
                <Typography variant="body2" fontWeight={650}>
                  {String(session.ip ?? "주소 미상")}
                </Typography>
                <Typography variant="caption" color="text.secondary" display="block">
                  {session.userAgent ?? "클라이언트 미상"}
                </Typography>
                <Typography variant="caption" color="text.secondary">
                  마지막 사용 {formatDate(session.lastSeenAt)} · 만료{" "}
                  {formatDate(session.expiresAt)}
                </Typography>
              </Card>
            ))}
            {sessions.data && sessions.data.length === 0 && (
              <Typography color="text.secondary" py={2} textAlign="center">
                열린 세션이 없습니다.
              </Typography>
            )}
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button
            color="error"
            disabled={
              revokeSessions.isPending || (sessions.data ?? []).length === 0
            }
            onClick={() => revokeSessions.mutate()}
          >
            모든 세션 종료
          </Button>
          <Button onClick={() => setSessionUser(null)}>닫기</Button>
        </DialogActions>
      </Dialog>
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
