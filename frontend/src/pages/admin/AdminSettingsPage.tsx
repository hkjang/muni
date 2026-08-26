import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CheckCircleOutline,
  PsychologyOutlined,
  SaveOutlined,
  SecurityOutlined,
  SettingsOutlined,
  AutoDeleteOutlined,
  MailOutlined,
  SlideshowOutlined,
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
  smtp: {
    enabled: false,
    host: "",
    port: 587,
    username: "",
    passwordSet: false,
    security: "starttls",
    from: "",
    fromName: "",
    skipVerify: false,
    baseUrl: "",
  },
  retention: {
    trashDays: 0,
    revisionDays: 0,
    revisionKeep: 5,
    auditDays: 0,
    aiAuditDays: 0,
  },
  ptium: {
    enabled: false,
    baseUrl: "",
    webUrl: "",
    apiKeySet: false,
    defaultTheme: "",
    defaultLocale: "ko",
    timeoutSeconds: 120,
  },
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
  const testSMTP = useMutation({
    mutationFn: () =>
      api<{ sentTo: string }>("/api/v1/admin/settings/test-smtp", {
        method: "POST",
        ...jsonBody(form.smtp),
      }),
    onSuccess: (data) =>
      setNotice(`${data.sentTo} 주소로 시험 메일을 보냈습니다. 받은 편지함을 확인해 주세요.`),
  });
  const retention = useQuery({
    queryKey: ["retention-preview"],
    queryFn: () =>
      api<{ pending: RetentionCounts }>("/api/v1/admin/retention/preview"),
  });
  const runRetention = useMutation({
    mutationFn: () =>
      api<RetentionCounts>("/api/v1/admin/retention/run", { method: "POST" }),
    onSuccess: (data) => {
      setNotice(
        `정리했습니다. 문서 ${data.documents}건, 버전 ${data.revisions}건, 감사 로그 ${data.audit}건, AI 기록 ${data.aiAudit}건, 만료 세션 ${data.sessions}건.`,
      );
      void retention.refetch();
      void client.invalidateQueries({ queryKey: ["admin-overview"] });
    },
  });
  const testPtium = useMutation({
    mutationFn: () =>
      api<{ baseUrl: string }>("/api/v1/admin/settings/test-ptium", {
        method: "POST",
        ...jsonBody(form.ptium),
      }),
    onSuccess: (data) =>
      setNotice(`Ptium 서버(${data.baseUrl})에 연결했습니다.`),
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
      {(save.error ||
        testOIDC.error ||
        testAI.error ||
        testPtium.error ||
        testSMTP.error ||
        runRetention.error) && (
        <Alert severity="error" sx={{ mt: 2 }}>
          {errorMessage(
            save.error ||
              testOIDC.error ||
              testAI.error ||
              testPtium.error ||
              testSMTP.error ||
              runRetention.error,
          )}
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
        <Tab
          icon={<SlideshowOutlined />}
          iconPosition="start"
          label="발표자료 연동"
        />
        <Tab icon={<MailOutlined />} iconPosition="start" label="메일 알림" />
        <Tab
          icon={<AutoDeleteOutlined />}
          iconPosition="start"
          label="보존 정책"
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
        {tab === 5 && (
          <Stack gap={2} maxWidth={760}>
            <Section
              title="Ptium 발표자료 연동"
              description="문서에서 발표자료를 만들고, 문서가 바뀌면 슬라이드를 다시 맞춥니다. muni는 REST로만 연결하며 Ptium의 데이터베이스를 직접 읽지 않습니다."
            />
            <FormControlLabel
              control={
                <Checkbox
                  checked={form.ptium.enabled}
                  onChange={(_, checked) =>
                    setGroup("ptium", { ...form.ptium, enabled: checked })
                  }
                />
              }
              label="발표자료 연동 활성화"
            />
            <TextField
              label="API Base URL"
              placeholder="http://ptium.internal/api"
              value={form.ptium.baseUrl}
              onChange={(e) =>
                setGroup("ptium", { ...form.ptium, baseUrl: e.target.value })
              }
              helperText="muni 서버에서 호출하는 주소입니다."
            />
            <TextField
              label="편집 화면 주소 (비우면 API 주소와 동일)"
              placeholder="https://ptium.example.com"
              value={form.ptium.webUrl}
              onChange={(e) =>
                setGroup("ptium", { ...form.ptium, webUrl: e.target.value })
              }
              helperText="'Ptium에서 편집' 링크가 향하는 곳입니다. 브라우저에서 열 수 있는 주소여야 합니다."
            />
            <TextField
              type="password"
              label={`API key${form.ptium.apiKeySet ? " (설정됨)" : ""}`}
              placeholder={
                form.ptium.apiKeySet ? "변경할 때만 입력" : "Ptium 발급 키"
              }
              value={form.ptium.apiKey ?? ""}
              onChange={(e) =>
                setGroup("ptium", { ...form.ptium, apiKey: e.target.value })
              }
            />
            <Grid container spacing={2}>
              <Grid size={{ xs: 12, sm: 4 }}>
                <TextField
                  fullWidth
                  label="기본 테마"
                  placeholder="default"
                  value={form.ptium.defaultTheme}
                  onChange={(e) =>
                    setGroup("ptium", {
                      ...form.ptium,
                      defaultTheme: e.target.value,
                    })
                  }
                />
              </Grid>
              <Grid size={{ xs: 12, sm: 4 }}>
                <TextField
                  fullWidth
                  label="기본 언어"
                  placeholder="ko"
                  value={form.ptium.defaultLocale}
                  onChange={(e) =>
                    setGroup("ptium", {
                      ...form.ptium,
                      defaultLocale: e.target.value,
                    })
                  }
                />
              </Grid>
              <Grid size={{ xs: 12, sm: 4 }}>
                <TextField
                  fullWidth
                  type="number"
                  label="Timeout (초)"
                  value={form.ptium.timeoutSeconds}
                  slotProps={{ htmlInput: { min: 5, max: 900 } }}
                  onChange={(e) =>
                    setGroup("ptium", {
                      ...form.ptium,
                      timeoutSeconds: Number(e.target.value),
                    })
                  }
                />
              </Grid>
            </Grid>
            <Button
              variant="outlined"
              onClick={() => testPtium.mutate()}
              disabled={testPtium.isPending || !form.ptium.baseUrl.trim()}
            >
              Ptium 연결 테스트
            </Button>
            <Typography variant="body2" color="text.secondary">
              연결이 켜지면 문서 편집 화면 오른쪽에 '발표자료' 탭이 나타납니다.
            </Typography>
          </Stack>
        )}
        {tab === 6 && (
          <Stack gap={2} maxWidth={760}>
            <Section
              title="사내 메일 서버"
              description="muni가 쓰는 메일 서버는 여기에 적은 것 하나뿐입니다. 외부 발송 서비스로 나가는 연결은 없습니다."
            />
            <FormControlLabel
              control={
                <Checkbox
                  checked={form.smtp.enabled}
                  onChange={(_, checked) =>
                    setGroup("smtp", { ...form.smtp, enabled: checked })
                  }
                />
              }
              label="메일 알림 보내기"
            />
            <Grid container spacing={2}>
              <Grid size={{ xs: 12, sm: 7 }}>
                <TextField
                  fullWidth
                  label="메일 서버 주소"
                  placeholder="smtp.company.co.kr"
                  value={form.smtp.host}
                  onChange={(e) =>
                    setGroup("smtp", { ...form.smtp, host: e.target.value })
                  }
                />
              </Grid>
              <Grid size={{ xs: 6, sm: 2 }}>
                <TextField
                  fullWidth
                  type="number"
                  label="포트"
                  value={form.smtp.port}
                  slotProps={{ htmlInput: { min: 1, max: 65535 } }}
                  onChange={(e) =>
                    setGroup("smtp", { ...form.smtp, port: Number(e.target.value) })
                  }
                />
              </Grid>
              <Grid size={{ xs: 6, sm: 3 }}>
                <FormControl fullWidth>
                  <InputLabel>보안</InputLabel>
                  <Select
                    label="보안"
                    value={form.smtp.security}
                    onChange={(e) =>
                      setGroup("smtp", { ...form.smtp, security: e.target.value })
                    }
                  >
                    <MenuItem value="starttls">STARTTLS (587)</MenuItem>
                    <MenuItem value="tls">TLS (465)</MenuItem>
                    <MenuItem value="none">사용 안 함</MenuItem>
                  </Select>
                </FormControl>
              </Grid>
            </Grid>
            <Grid container spacing={2}>
              <Grid size={{ xs: 12, sm: 6 }}>
                <TextField
                  fullWidth
                  label="계정 (필요한 경우)"
                  value={form.smtp.username}
                  onChange={(e) =>
                    setGroup("smtp", { ...form.smtp, username: e.target.value })
                  }
                />
              </Grid>
              <Grid size={{ xs: 12, sm: 6 }}>
                <TextField
                  fullWidth
                  type="password"
                  label={`비밀번호${form.smtp.passwordSet ? " (설정됨)" : ""}`}
                  placeholder={form.smtp.passwordSet ? "변경할 때만 입력" : ""}
                  value={form.smtp.password ?? ""}
                  onChange={(e) =>
                    setGroup("smtp", { ...form.smtp, password: e.target.value })
                  }
                />
              </Grid>
              <Grid size={{ xs: 12, sm: 6 }}>
                <TextField
                  fullWidth
                  label="보내는 주소"
                  placeholder="muni-noreply@company.co.kr"
                  value={form.smtp.from}
                  onChange={(e) =>
                    setGroup("smtp", { ...form.smtp, from: e.target.value })
                  }
                  helperText="비우면 계정 주소를 사용합니다."
                />
              </Grid>
              <Grid size={{ xs: 12, sm: 6 }}>
                <TextField
                  fullWidth
                  label="보내는 사람 이름"
                  placeholder="muni 알림"
                  value={form.smtp.fromName}
                  onChange={(e) =>
                    setGroup("smtp", { ...form.smtp, fromName: e.target.value })
                  }
                />
              </Grid>
            </Grid>
            <TextField
              label="메일에 넣을 서비스 주소"
              placeholder="https://muni.company.co.kr"
              value={form.smtp.baseUrl}
              onChange={(e) =>
                setGroup("smtp", { ...form.smtp, baseUrl: e.target.value })
              }
              helperText="알림 메일의 링크가 향하는 곳입니다. 비우면 링크 없이 보냅니다."
            />
            <FormControlLabel
              control={
                <Checkbox
                  checked={form.smtp.skipVerify}
                  onChange={(_, checked) =>
                    setGroup("smtp", { ...form.smtp, skipVerify: checked })
                  }
                />
              }
              label="서버 인증서를 검증하지 않음 (사설 인증기관을 쓰는 경우)"
            />
            <Alert severity="info">
              보내는 내용은 muni가 이미 기록하는 알림입니다 — 검토 요청, 승인
              결과, 멘션. <strong>문서 내용은 담기지 않습니다.</strong> 알림
              메일이 문서를 실어 나르면, 받는 사람이 메일을 어디로 전달하든
              그 내용도 함께 갑니다.
            </Alert>
            <Button
              variant="outlined"
              onClick={() => testSMTP.mutate()}
              disabled={testSMTP.isPending || !form.smtp.host.trim()}
            >
              내 주소로 시험 메일 보내기
            </Button>
          </Stack>
        )}
        {tab === 7 && (
          <Stack gap={2} maxWidth={760}>
            <Section
              title="보존 정책"
              description="0일은 '계속 보관'입니다. 값을 넣기 전까지는 아무것도 지워지지 않습니다. 매일 한 번 적용되며, 지워진 것은 되돌릴 수 없습니다."
            />
            <Grid container spacing={2}>
              <Grid size={{ xs: 12, sm: 6 }}>
                <TextField
                  fullWidth
                  type="number"
                  label="휴지통 보관 기간 (일)"
                  value={form.retention.trashDays}
                  slotProps={{ htmlInput: { min: 0, max: 3650 } }}
                  onChange={(e) =>
                    setGroup("retention", {
                      ...form.retention,
                      trashDays: Number(e.target.value),
                    })
                  }
                  helperText="지나면 문서와 그 버전·댓글·첨부가 함께 사라집니다."
                />
              </Grid>
              <Grid size={{ xs: 12, sm: 6 }}>
                <TextField
                  fullWidth
                  type="number"
                  label="감사 로그 보관 기간 (일)"
                  value={form.retention.auditDays}
                  slotProps={{ htmlInput: { min: 0, max: 3650 } }}
                  onChange={(e) =>
                    setGroup("retention", {
                      ...form.retention,
                      auditDays: Number(e.target.value),
                    })
                  }
                  helperText="법령이나 사내 규정이 정한 기간을 확인하세요."
                />
              </Grid>
              <Grid size={{ xs: 12, sm: 4 }}>
                <TextField
                  fullWidth
                  type="number"
                  label="버전 보관 기간 (일)"
                  value={form.retention.revisionDays}
                  slotProps={{ htmlInput: { min: 0, max: 3650 } }}
                  onChange={(e) =>
                    setGroup("retention", {
                      ...form.retention,
                      revisionDays: Number(e.target.value),
                    })
                  }
                />
              </Grid>
              <Grid size={{ xs: 12, sm: 4 }}>
                <TextField
                  fullWidth
                  type="number"
                  label="항상 남길 버전 수"
                  value={form.retention.revisionKeep}
                  slotProps={{ htmlInput: { min: 5, max: 1000 } }}
                  onChange={(e) =>
                    setGroup("retention", {
                      ...form.retention,
                      revisionKeep: Number(e.target.value),
                    })
                  }
                  helperText="최소 5개"
                />
              </Grid>
              <Grid size={{ xs: 12, sm: 4 }}>
                <TextField
                  fullWidth
                  type="number"
                  label="AI 호출 기록 (일)"
                  value={form.retention.aiAuditDays}
                  slotProps={{ htmlInput: { min: 0, max: 3650 } }}
                  onChange={(e) =>
                    setGroup("retention", {
                      ...form.retention,
                      aiAuditDays: Number(e.target.value),
                    })
                  }
                />
              </Grid>
            </Grid>
            <Alert severity="info">
              기간이 지나도 <strong>이름을 붙인 버전</strong>, 문서의 현재
              버전, 그리고 가장 최근 {form.retention.revisionKeep}개 버전은
              남습니다. 시계로 비울 수 있는 기록은 기록이 아니니까요.
            </Alert>
            {retention.data && (
              <Alert severity={pendingTotal(retention.data.pending) > 0 ? "warning" : "success"}>
                지금 정책으로 정리하면 문서 {retention.data.pending.documents}건,
                버전 {retention.data.pending.revisions}건, 감사 로그{" "}
                {retention.data.pending.audit}건, AI 기록{" "}
                {retention.data.pending.aiAudit}건, 만료 세션{" "}
                {retention.data.pending.sessions}건이 지워집니다.
              </Alert>
            )}
            <Typography variant="body2" color="text.secondary">
              미리보기는 <strong>저장된</strong> 정책 기준입니다. 위 값을 바꿨다면
              먼저 전체 저장을 눌러 주세요.
            </Typography>
            <Stack direction="row" gap={1}>
              <Button
                variant="outlined"
                onClick={() => void retention.refetch()}
                disabled={retention.isFetching}
              >
                다시 계산
              </Button>
              <Button
                variant="outlined"
                color="error"
                onClick={() => {
                  if (
                    window.confirm(
                      "저장된 보존 정책을 지금 적용합니다. 지워진 항목은 되돌릴 수 없습니다.",
                    )
                  )
                    runRetention.mutate();
                }}
                disabled={runRetention.isPending}
              >
                지금 정리 실행
              </Button>
            </Stack>
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

type RetentionCounts = {
  documents: number;
  revisions: number;
  audit: number;
  aiAudit: number;
  sessions: number;
};

/** pendingTotal is what the policy would remove, ignoring expired sessions
 * — those are dead weight either way and their count is never a warning. */
function pendingTotal(counts: RetentionCounts): number {
  return counts.documents + counts.revisions + counts.audit + counts.aiAudit;
}
