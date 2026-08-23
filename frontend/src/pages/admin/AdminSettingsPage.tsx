import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CheckCircleOutline,
  PsychologyOutlined,
  SaveOutlined,
  SecurityOutlined,
  SettingsOutlined,
  VpnKeyOutlined,
} from "@mui/icons-material";
import {
  Alert,
  Box,
  Button,
  Card,
  Checkbox,
  CircularProgress,
  FormControl,
  FormControlLabel,
  Grid,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  Tab,
  Tabs,
  TextField,
  Typography,
} from "@mui/material";
import { api, errorMessage, jsonBody } from "../../lib/api";
import type { Settings } from "../../types";
import { useAuth } from "../../contexts/AuthContext";

const blank: Settings = {
  general: {
    serviceName: "muni",
    allowLocalLogin: true,
    defaultLocale: "ko-KR",
    pageSize: 30,
  },
  oidc: {
    enabled: false,
    issuerUrl: "",
    clientId: "",
    secretSet: false,
    redirectUrl: "",
    scopes: ["openid", "profile", "email"],
    autoProvision: true,
    defaultRole: "USER",
  },
  ai: {
    enabled: false,
    baseUrl: "",
    apiKeySet: false,
    model: "",
    maxTokens: 32768,
    timeoutSeconds: 600,
    systemPrompt: "",
  },
  workflow: { enabled: false, requiredApprovals: 1, allowSelfApproval: false },
  security: {
    sessionHours: 12,
    apiKeyMaxDays: 365,
    allowPublicLinks: false,
    maxUploadMb: 50,
    auditReads: true,
  },
  export: { enablePdf: true, enableDocx: true },
};
export function AdminSettingsPage() {
  const { refresh } = useAuth();
  const [tab, setTab] = useState(0);
  const [form, setForm] = useState<Settings>(blank);
  const [notice, setNotice] = useState("");
  const client = useQueryClient();
  const query = useQuery({
    queryKey: ["admin-settings"],
    queryFn: () => api<Settings>("/api/v1/admin/settings"),
  });
  useEffect(() => {
    if (query.data) setForm(query.data);
  }, [query.data]);
  const save = useMutation({
    mutationFn: () =>
      api<Settings>("/api/v1/admin/settings", {
        method: "PUT",
        ...jsonBody(form),
      }),
    onSuccess: (data) => {
      setForm(data);
      setNotice("설정을 저장했습니다. 새 요청부터 즉시 적용됩니다.");
      void client.invalidateQueries({ queryKey: ["admin-settings"] });
      void refresh();
    },
  });
  const testOIDC = useMutation({
    mutationFn: () =>
      api("/api/v1/admin/settings/test-oidc", {
        method: "POST",
        ...jsonBody(form.oidc),
      }),
    onSuccess: () => setNotice("OIDC discovery 연결에 성공했습니다."),
  });
  const testAI = useMutation({
    mutationFn: () =>
      api("/api/v1/admin/settings/test-ai", {
        method: "POST",
        ...jsonBody(form.ai),
      }),
    onSuccess: () => setNotice("AI API 연결과 모델 응답을 확인했습니다."),
  });
  const setGroup = <K extends keyof Settings>(group: K, value: Settings[K]) =>
    setForm((current) => ({ ...current, [group]: value }));
  if (query.isLoading)
    return (
      <Box p={5}>
        <CircularProgress />
      </Box>
    );
  return (
    <Box sx={{ p: { xs: 2.5, sm: 4, lg: 5 }, maxWidth: 1160, mx: "auto" }}>
      <Stack
        direction={{ xs: "column", sm: "row" }}
        justifyContent="space-between"
        gap={2}
      >
        <Box>
          <Typography variant="h1">서비스 설정</Typography>
          <Typography color="text.secondary" mt={0.7}>
            환경변수 4개를 제외한 모든 운영 설정을 여기서 관리합니다.
          </Typography>
        </Box>
        <Button
          variant="contained"
          startIcon={<SaveOutlined />}
          onClick={() => save.mutate()}
          disabled={save.isPending}
        >
          전체 저장
        </Button>
      </Stack>
      {(save.error || testOIDC.error || testAI.error) && (
        <Alert severity="error" sx={{ mt: 2 }}>
          {errorMessage(save.error || testOIDC.error || testAI.error)}
        </Alert>
      )}
      {notice && (
        <Alert severity="success" onClose={() => setNotice("")} sx={{ mt: 2 }}>
          {notice}
        </Alert>
      )}
      <Tabs
        value={tab}
        onChange={(_, v) => setTab(v)}
        variant="scrollable"
        sx={{ mt: 3, borderBottom: "1px solid", borderColor: "divider" }}
      >
        <Tab icon={<SettingsOutlined />} iconPosition="start" label="일반" />
        <Tab
          icon={<VpnKeyOutlined />}
          iconPosition="start"
          label="Keycloak OIDC"
        />
        <Tab icon={<PsychologyOutlined />} iconPosition="start" label="AI" />
        <Tab
          icon={<CheckCircleOutline />}
          iconPosition="start"
          label="검토·승인"
        />
        <Tab
          icon={<SecurityOutlined />}
          iconPosition="start"
          label="보안·내보내기"
        />
      </Tabs>
      <Card sx={{ p: { xs: 2, sm: 3 }, mt: 2 }}>
        {tab === 0 && (
          <Stack gap={2} maxWidth={650}>
            <Section
              title="일반 설정"
              description="서비스 표시 이름과 기본 화면 동작을 정합니다."
            />
            <TextField
              label="서비스 이름"
              value={form.general.serviceName}
              onChange={(e) =>
                setGroup("general", {
                  ...form.general,
                  serviceName: e.target.value,
                })
              }
            />
            <TextField
              label="기본 언어"
              value={form.general.defaultLocale}
              onChange={(e) =>
                setGroup("general", {
                  ...form.general,
                  defaultLocale: e.target.value,
                })
              }
            />
            <TextField
              type="number"
              label="목록 페이지 크기"
              value={form.general.pageSize}
              onChange={(e) =>
                setGroup("general", {
                  ...form.general,
                  pageSize: Number(e.target.value),
                })
              }
            />
            <FormControlLabel
              control={
                <Checkbox
                  checked={form.general.allowLocalLogin}
                  onChange={(_, checked) =>
                    setGroup("general", {
                      ...form.general,
                      allowLocalLogin: checked,
                    })
                  }
                />
              }
              label="로컬 로그인 허용"
            />
          </Stack>
        )}
        {tab === 1 && (
          <Stack gap={2} maxWidth={760}>
            <Section
              title="Keycloak / OIDC"
              description="Issuer URL, client ID와 secret만 입력하면 discovery로 엔드포인트를 자동 구성합니다."
            />
            <FormControlLabel
              control={
                <Checkbox
                  checked={form.oidc.enabled}
                  onChange={(_, checked) =>
                    setGroup("oidc", { ...form.oidc, enabled: checked })
                  }
                />
              }
              label="OIDC SSO 활성화"
            />
            <TextField
              label="Issuer URL"
              placeholder="https://keycloak.internal/realms/company"
              value={form.oidc.issuerUrl}
              onChange={(e) =>
                setGroup("oidc", { ...form.oidc, issuerUrl: e.target.value })
              }
            />
            <TextField
              label="Client ID"
              value={form.oidc.clientId}
              onChange={(e) =>
                setGroup("oidc", { ...form.oidc, clientId: e.target.value })
              }
            />
            <TextField
              type="password"
              label={`Client secret${form.oidc.secretSet ? " (설정됨)" : ""}`}
              placeholder={
                form.oidc.secretSet ? "변경할 때만 입력" : "Client secret"
              }
              value={form.oidc.clientSecret ?? ""}
              onChange={(e) =>
                setGroup("oidc", { ...form.oidc, clientSecret: e.target.value })
              }
            />
            <TextField
              label="Redirect URL (비우면 현재 주소로 자동 계산)"
              value={form.oidc.redirectUrl}
              onChange={(e) =>
                setGroup("oidc", { ...form.oidc, redirectUrl: e.target.value })
              }
            />
            <TextField
              label="Scopes (공백 구분)"
              value={form.oidc.scopes.join(" ")}
              onChange={(e) =>
                setGroup("oidc", {
                  ...form.oidc,
                  scopes: e.target.value.split(/\s+/).filter(Boolean),
                })
              }
            />
            <Grid container spacing={2}>
              <Grid size={{ xs: 12, sm: 6 }}>
                <FormControl fullWidth size="small">
                  <InputLabel>신규 사용자 역할</InputLabel>
                  <Select
                    label="신규 사용자 역할"
                    value={form.oidc.defaultRole}
                    onChange={(e) =>
                      setGroup("oidc", {
                        ...form.oidc,
                        defaultRole: e.target.value as "USER" | "ADMIN",
                      })
                    }
                  >
                    <MenuItem value="USER">USER</MenuItem>
                    <MenuItem value="ADMIN">ADMIN</MenuItem>
                  </Select>
                </FormControl>
              </Grid>
              <Grid size={{ xs: 12, sm: 6 }}>
                <FormControlLabel
                  control={
                    <Checkbox
                      checked={form.oidc.autoProvision}
                      onChange={(_, checked) =>
                        setGroup("oidc", {
                          ...form.oidc,
                          autoProvision: checked,
                        })
                      }
                    />
                  }
                  label="첫 로그인 시 사용자 자동 생성"
                />
              </Grid>
            </Grid>
            <Button
              variant="outlined"
              onClick={() => testOIDC.mutate()}
              disabled={testOIDC.isPending}
            >
              Discovery 연결 테스트
            </Button>
          </Stack>
        )}
        {tab === 2 && (
          <Stack gap={2} maxWidth={760}>
            <Section
              title="OpenAI 호환 AI"
              description="모든 채팅 호출은 SSE 스트리밍이 기본이며 출력 max token은 최대 256k까지 제한합니다."
            />
            <FormControlLabel
              control={
                <Checkbox
                  checked={form.ai.enabled}
                  onChange={(_, checked) =>
                    setGroup("ai", { ...form.ai, enabled: checked })
                  }
                />
              }
              label="AI 기능 활성화"
            />
            <TextField
              label="API Base URL"
              placeholder="http://llm-gateway.internal/v1"
              value={form.ai.baseUrl}
              onChange={(e) =>
                setGroup("ai", { ...form.ai, baseUrl: e.target.value })
              }
            />
            <TextField
              type="password"
              label={`API key${form.ai.apiKeySet ? " (설정됨)" : ""}`}
              placeholder={form.ai.apiKeySet ? "변경할 때만 입력" : "선택 사항"}
              value={form.ai.apiKey ?? ""}
              onChange={(e) =>
                setGroup("ai", { ...form.ai, apiKey: e.target.value })
              }
            />
            <TextField
              label="모델"
              value={form.ai.model}
              onChange={(e) =>
                setGroup("ai", { ...form.ai, model: e.target.value })
              }
            />
            <Grid container spacing={2}>
              <Grid size={{ xs: 12, sm: 6 }}>
                <TextField
                  fullWidth
                  type="number"
                  label="Max token (≤262144)"
                  value={form.ai.maxTokens}
                  slotProps={{ htmlInput: { min: 1, max: 262144 } }}
                  onChange={(e) =>
                    setGroup("ai", {
                      ...form.ai,
                      maxTokens: Number(e.target.value),
                    })
                  }
                />
              </Grid>
              <Grid size={{ xs: 12, sm: 6 }}>
                <TextField
                  fullWidth
                  type="number"
                  label="Timeout (초)"
                  value={form.ai.timeoutSeconds}
                  onChange={(e) =>
                    setGroup("ai", {
                      ...form.ai,
                      timeoutSeconds: Number(e.target.value),
                    })
                  }
                />
              </Grid>
            </Grid>
            <TextField
              multiline
              minRows={4}
              label="시스템 프롬프트"
              value={form.ai.systemPrompt}
              onChange={(e) =>
                setGroup("ai", { ...form.ai, systemPrompt: e.target.value })
              }
            />
            <Button
              variant="outlined"
              onClick={() => testAI.mutate()}
              disabled={testAI.isPending}
            >
              AI 모델 연결 테스트
            </Button>
          </Stack>
        )}
        {tab === 3 && (
          <Stack gap={2} maxWidth={650}>
            <Section
              title="조건부 검토·승인"
              description="끄면 문서에서 승인·반려 상태와 관련 UI가 제외됩니다."
            />
            <FormControlLabel
              control={
                <Checkbox
                  checked={form.workflow.enabled}
                  onChange={(_, checked) =>
                    setGroup("workflow", { ...form.workflow, enabled: checked })
                  }
                />
              }
              label="팀장 검토 및 승인 프로세스 활성화"
            />
            <TextField
              type="number"
              label="필요 승인 수"
              value={form.workflow.requiredApprovals}
              onChange={(e) =>
                setGroup("workflow", {
                  ...form.workflow,
                  requiredApprovals: Number(e.target.value),
                })
              }
              disabled={!form.workflow.enabled}
            />
            <FormControlLabel
              control={
                <Checkbox
                  checked={form.workflow.allowSelfApproval}
                  onChange={(_, checked) =>
                    setGroup("workflow", {
                      ...form.workflow,
                      allowSelfApproval: checked,
                    })
                  }
                />
              }
              label="요청자 본인 승인 허용"
              disabled={!form.workflow.enabled}
            />
          </Stack>
        )}
        {tab === 4 && (
          <Stack gap={2} maxWidth={700}>
            <Section
              title="보안 정책"
              description="세션, 공유, API 키와 파일 크기를 서비스 전체에 적용합니다."
            />
            <Grid container spacing={2}>
              <Grid size={{ xs: 12, sm: 6 }}>
                <TextField
                  fullWidth
                  type="number"
                  label="세션 유지 시간"
                  value={form.security.sessionHours}
                  onChange={(e) =>
                    setGroup("security", {
                      ...form.security,
                      sessionHours: Number(e.target.value),
                    })
                  }
                />
              </Grid>
              <Grid size={{ xs: 12, sm: 6 }}>
                <TextField
                  fullWidth
                  type="number"
                  label="API 키 최대 일수"
                  value={form.security.apiKeyMaxDays}
                  onChange={(e) =>
                    setGroup("security", {
                      ...form.security,
                      apiKeyMaxDays: Number(e.target.value),
                    })
                  }
                />
              </Grid>
              <Grid size={{ xs: 12, sm: 6 }}>
                <TextField
                  fullWidth
                  type="number"
                  label="업로드 한도 (MB)"
                  value={form.security.maxUploadMb}
                  onChange={(e) =>
                    setGroup("security", {
                      ...form.security,
                      maxUploadMb: Number(e.target.value),
                    })
                  }
                />
              </Grid>
            </Grid>
            <FormControlLabel
              control={
                <Checkbox
                  checked={form.security.allowPublicLinks}
                  onChange={(_, checked) =>
                    setGroup("security", {
                      ...form.security,
                      allowPublicLinks: checked,
                    })
                  }
                />
              }
              label="공개 링크 공유 허용"
            />
            <FormControlLabel
              control={
                <Checkbox
                  checked={form.security.auditReads}
                  onChange={(_, checked) =>
                    setGroup("security", {
                      ...form.security,
                      auditReads: checked,
                    })
                  }
                />
              }
              label="문서 읽기 감사 로그 기록"
            />
            <Typography variant="h3" mt={2}>
              내보내기 정책
            </Typography>
            <FormControlLabel
              control={
                <Checkbox
                  checked={form.export.enablePdf}
                  onChange={(_, checked) =>
                    setGroup("export", { ...form.export, enablePdf: checked })
                  }
                />
              }
              label="PDF 내보내기"
            />
            <FormControlLabel
              control={
                <Checkbox
                  checked={form.export.enableDocx}
                  onChange={(_, checked) =>
                    setGroup("export", { ...form.export, enableDocx: checked })
                  }
                />
              }
              label="DOCX 내보내기"
            />
          </Stack>
        )}
      </Card>
    </Box>
  );
}
function Section({
  title,
  description,
}: {
  title: string;
  description: string;
}) {
  return (
    <Box mb={1}>
      <Typography variant="h3">{title}</Typography>
      <Typography color="text.secondary" mt={0.5}>
        {description}
      </Typography>
    </Box>
  );
}
