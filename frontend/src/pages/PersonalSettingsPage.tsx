import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ContentCopy,
  KeyOutlined,
  LockResetOutlined,
  PersonOutline,
  VpnKeyOutlined,
} from "@mui/icons-material";
import {
  Alert,
  Box,
  Button,
  Card,
  Checkbox,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControlLabel,
  Grid,
  IconButton,
  Stack,
  Tab,
  Tabs,
  TextField,
  Tooltip,
  Typography,
} from "@mui/material";
import { api, errorMessage, formatDate, jsonBody } from "../lib/api";
import { useAuth } from "../contexts/AuthContext";

type PersonalKey = {
  id: string;
  name: string;
  fingerprint: string;
  status: "ACTIVE" | "RETIRED" | "REVOKED";
  version: number;
  createdAt: string;
  retiredAt?: string;
};
type APIKey = {
  id: string;
  name: string;
  prefix: string;
  scopes: string[];
  expiresAt?: string;
  lastUsedAt?: string;
  createdAt: string;
  revokedAt?: string;
};
const scopeLabels: Record<string, string> = {
  "api:read": "REST API 읽기",
  "api:write": "REST API 쓰기",
  "mcp:read": "MCP 읽기 도구",
  "mcp:write": "MCP 쓰기 도구",
  "ai:use": "AI 스트리밍 호출",
};

export function PersonalSettingsPage() {
  const { user, build } = useAuth();
  const [tab, setTab] = useState(0);
  const [rotateOpen, setRotateOpen] = useState(false);
  const [apiOpen, setApiOpen] = useState(false);
  const [keyName, setKeyName] = useState("내 API 키");
  const [scopes, setScopes] = useState<string[]>(["mcp:read"]);
  const [revealed, setRevealed] = useState("");
  const client = useQueryClient();
  const keys = useQuery({
    queryKey: ["personal-keys"],
    queryFn: () => api<PersonalKey[]>("/api/v1/me/keys"),
  });
  const apiKeys = useQuery({
    queryKey: ["api-keys"],
    queryFn: () => api<APIKey[]>("/api/v1/me/api-keys"),
  });
  const rotate = useMutation({
    mutationFn: () =>
      api("/api/v1/me/keys/rotate", {
        method: "POST",
        ...jsonBody({ name: "회전된 개인 키" }),
      }),
    onSuccess: () => {
      setRotateOpen(false);
      void client.invalidateQueries({ queryKey: ["personal-keys"] });
    },
  });
  const revokePersonal = useMutation({
    mutationFn: (id: string) =>
      api<void>(`/api/v1/me/keys/${id}`, { method: "DELETE" }),
    onSuccess: () => client.invalidateQueries({ queryKey: ["personal-keys"] }),
  });
  const createAPI = useMutation({
    mutationFn: () =>
      api<{ token: string }>("/api/v1/me/api-keys", {
        method: "POST",
        ...jsonBody({ name: keyName, scopes }),
      }),
    onSuccess: (result) => {
      setApiOpen(false);
      setRevealed(result.token);
      void client.invalidateQueries({ queryKey: ["api-keys"] });
    },
  });
  const revoke = useMutation({
    mutationFn: (id: string) =>
      api<void>(`/api/v1/me/api-keys/${id}`, { method: "DELETE" }),
    onSuccess: () => client.invalidateQueries({ queryKey: ["api-keys"] }),
  });
  return (
    <Box sx={{ p: { xs: 2.5, sm: 4, lg: 5 }, maxWidth: 1100, mx: "auto" }}>
      <Typography variant="h1">개인 설정</Typography>
      <Typography color="text.secondary" mt={0.7} mb={3}>
        내 프로필과 개인 암호화 키, API 연결을 관리합니다. 서비스 전체 설정과
        분리되어 있습니다.
      </Typography>
      <Tabs
        value={tab}
        onChange={(_, value) => setTab(value)}
        sx={{ borderBottom: "1px solid", borderColor: "divider", mb: 3 }}
      >
        <Tab icon={<PersonOutline />} iconPosition="start" label="프로필" />
        <Tab
          icon={<LockResetOutlined />}
          iconPosition="start"
          label="개인 키"
        />
        <Tab icon={<VpnKeyOutlined />} iconPosition="start" label="API · MCP" />
      </Tabs>
      {tab === 0 && (
        <Grid container spacing={2}>
          <Grid size={{ xs: 12, md: 7 }}>
            <Card sx={{ p: 3 }}>
              <Typography variant="h3" mb={2}>
                프로필 정보
              </Typography>
              <Stack gap={2}>
                <TextField
                  label="표시 이름"
                  value={user?.displayName ?? ""}
                  disabled
                />
                <TextField label="이메일" value={user?.email ?? ""} disabled />
                <TextField
                  label="아이디"
                  value={user?.username ?? ""}
                  disabled
                />
                <Alert severity="info">
                  OIDC 프로필 정보는 Keycloak에서 관리됩니다. 로컬 계정 프로필
                  편집은 관리자에게 요청하세요.
                </Alert>
              </Stack>
            </Card>
          </Grid>
          <Grid size={{ xs: 12, md: 5 }}>
            <Card sx={{ p: 3 }}>
              <Typography variant="h3" mb={2}>
                서비스 정보
              </Typography>
              <Info label="역할" value={user?.role ?? "—"} />
              <Info label="언어" value={user?.locale ?? "ko-KR"} />
              <Info label="버전" value={build?.version ?? "dev"} />
              <Info
                label="커밋"
                value={build?.commit?.slice(0, 12) ?? "none"}
              />
            </Card>
          </Grid>
        </Grid>
      )}
      {tab === 1 && (
        <Stack gap={2}>
          <Card sx={{ p: 3 }}>
            <Stack
              direction={{ xs: "column", sm: "row" }}
              justifyContent="space-between"
              gap={2}
            >
              <Box>
                <Typography variant="h3">개인 암호화 키</Typography>
                <Typography color="text.secondary" mt={0.5}>
                  키 원문은 노출하지 않고 master key로 봉인합니다. 회전하면 기존
                  키는 안전하게 보관됩니다.
                </Typography>
              </Box>
              <Button
                variant="contained"
                startIcon={<LockResetOutlined />}
                onClick={() => setRotateOpen(true)}
              >
                키 회전
              </Button>
            </Stack>
            {(keys.error || revokePersonal.error) && (
              <Alert severity="error" sx={{ mt: 2 }}>
                {errorMessage(keys.error || revokePersonal.error)}
              </Alert>
            )}
            <Stack divider={<Divider />} mt={2}>
              {(keys.data ?? []).map((key) => (
                <Stack
                  key={key.id}
                  direction="row"
                  justifyContent="space-between"
                  alignItems="center"
                  py={1.5}
                >
                  <Box>
                    <Stack direction="row" gap={1} alignItems="center">
                      <KeyOutlined
                        color={key.status === "ACTIVE" ? "primary" : "disabled"}
                      />
                      <Typography fontWeight={700}>{key.name}</Typography>
                      <Chip
                        size="small"
                        color={key.status === "ACTIVE" ? "success" : "default"}
                        label={key.status}
                      />
                    </Stack>
                    <Typography variant="body2" color="text.secondary" mt={0.5}>
                      v{key.version} · fingerprint {key.fingerprint} ·{" "}
                      {formatDate(key.createdAt)}
                    </Typography>
                  </Box>
                  {key.status === "RETIRED" && (
                    <Button
                      color="error"
                      variant="outlined"
                      size="small"
                      onClick={() => revokePersonal.mutate(key.id)}
                    >
                      과거 키 폐기
                    </Button>
                  )}
                </Stack>
              ))}
            </Stack>
          </Card>
        </Stack>
      )}
      {tab === 2 && (
        <Card sx={{ p: 3 }}>
          <Stack
            direction={{ xs: "column", sm: "row" }}
            justifyContent="space-between"
            gap={2}
          >
            <Box>
              <Typography variant="h3">API 및 MCP 키</Typography>
              <Typography color="text.secondary" mt={0.5}>
                필요한 범위만 선택하세요. 비밀키는 생성 직후 한 번만 표시됩니다.
              </Typography>
            </Box>
            <Button
              variant="contained"
              startIcon={<VpnKeyOutlined />}
              onClick={() => setApiOpen(true)}
            >
              API 키 만들기
            </Button>
          </Stack>
          <Stack divider={<Divider />} mt={2}>
            {(apiKeys.data ?? []).map((key) => (
              <Stack
                key={key.id}
                direction={{ xs: "column", sm: "row" }}
                justifyContent="space-between"
                alignItems={{ sm: "center" }}
                gap={1.5}
                py={1.5}
              >
                <Box>
                  <Stack direction="row" gap={1} alignItems="center">
                    <Typography fontWeight={700}>{key.name}</Typography>
                    {key.revokedAt && <Chip size="small" label="폐기됨" />}
                  </Stack>
                  <Typography variant="body2" color="text.secondary" mt={0.4}>
                    {key.prefix}… · {key.scopes.join(", ")} · 최근 사용{" "}
                    {formatDate(key.lastUsedAt)}
                  </Typography>
                </Box>
                {!key.revokedAt && (
                  <Button
                    color="error"
                    variant="outlined"
                    onClick={() => revoke.mutate(key.id)}
                  >
                    폐기
                  </Button>
                )}
              </Stack>
            ))}
          </Stack>
          {!(apiKeys.data ?? []).length && (
            <Typography color="text.secondary" textAlign="center" py={5}>
              발급한 API 키가 없습니다.
            </Typography>
          )}
        </Card>
      )}
      <Dialog open={rotateOpen} onClose={() => setRotateOpen(false)}>
        <DialogTitle>개인 키를 회전할까요?</DialogTitle>
        <DialogContent>
          <Typography>
            새 키를 활성화하고 현재 키는 RETIRED 상태로 보존합니다. 기존 암호화
            자료를 읽을 수 있도록 과거 키는 자동 삭제되지 않습니다.
          </Typography>
          {rotate.error && (
            <Alert severity="error" sx={{ mt: 2 }}>
              {errorMessage(rotate.error)}
            </Alert>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setRotateOpen(false)}>취소</Button>
          <Button
            variant="contained"
            onClick={() => rotate.mutate()}
            disabled={rotate.isPending}
          >
            키 회전
          </Button>
        </DialogActions>
      </Dialog>
      <Dialog
        open={apiOpen}
        onClose={() => setApiOpen(false)}
        fullWidth
        maxWidth="sm"
      >
        <DialogTitle>새 API 키</DialogTitle>
        <DialogContent sx={{ pt: "8px!important" }}>
          <TextField
            fullWidth
            label="키 이름"
            value={keyName}
            onChange={(event) => setKeyName(event.target.value)}
            sx={{ mb: 2 }}
          />
          {Object.entries(scopeLabels).map(([scope, label]) => (
            <FormControlLabel
              key={scope}
              control={
                <Checkbox
                  checked={scopes.includes(scope)}
                  onChange={(_, checked) =>
                    setScopes((current) =>
                      checked
                        ? [...current, scope]
                        : current.filter((value) => value !== scope),
                    )
                  }
                />
              }
              label={label}
            />
          ))}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setApiOpen(false)}>취소</Button>
          <Button
            variant="contained"
            disabled={!keyName.trim() || !scopes.length || createAPI.isPending}
            onClick={() => createAPI.mutate()}
          >
            키 만들기
          </Button>
        </DialogActions>
      </Dialog>
      <Dialog
        open={Boolean(revealed)}
        onClose={() => setRevealed("")}
        fullWidth
        maxWidth="sm"
      >
        <DialogTitle>API 키를 안전하게 보관하세요</DialogTitle>
        <DialogContent>
          <Alert severity="warning" sx={{ mb: 2 }}>
            이 값은 닫은 뒤 다시 확인할 수 없습니다.
          </Alert>
          <Stack direction="row" gap={1}>
            <TextField
              fullWidth
              value={revealed}
              inputProps={{ readOnly: true }}
            />
            <Tooltip title="복사">
              <IconButton
                onClick={() => navigator.clipboard.writeText(revealed)}
              >
                <ContentCopy />
              </IconButton>
            </Tooltip>
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button variant="contained" onClick={() => setRevealed("")}>
            보관했습니다
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
function Info({ label, value }: { label: string; value: string }) {
  return (
    <Stack
      direction="row"
      justifyContent="space-between"
      py={1.2}
      borderBottom="1px solid"
      borderColor="divider"
    >
      <Typography color="text.secondary">{label}</Typography>
      <Typography fontWeight={650}>{value}</Typography>
    </Stack>
  );
}
