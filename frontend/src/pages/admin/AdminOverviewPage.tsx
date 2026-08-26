import { useQuery } from "@tanstack/react-query";
import { Link as RouterLink } from "react-router-dom";
import {
  Box,
  Card,
  Chip,
  CircularProgress,
  Grid,
  Link,
  Stack,
  Typography,
} from "@mui/material";
import {
  CheckCircleOutline,
  ErrorOutline,
  RemoveCircleOutline,
} from "@mui/icons-material";
import { api, formatDate } from "../../lib/api";

type Overview = {
  users: {
    total: number;
    active: number;
    suspended: number;
    admins: number;
    recentLogins: number;
  };
  documents: {
    total: number;
    trashed: number;
    createdThisWeek: number;
    editedThisWeek: number;
    pendingApproval: number;
    revisions: number;
  };
  storage: {
    workspaces: number;
    attachments: number;
    bytes: number;
    sessions: number;
    apiKeys: number;
  };
  checks: {
    key: string;
    label: string;
    state: "ok" | "off" | "warn";
    detail: string;
    setting?: string;
  }[];
  activity: { action: string; actorName?: string; createdAt: string }[];
  build: { version: string; commit: string };
};

/**
 * AdminOverviewPage is what an administrator now lands on.
 *
 * The settings form used to be the first screen, which answers none of the
 * questions an operator is actually asked — how much is in the system, who is
 * using it, whether the parts that reach outside are working — and invites
 * poking at configuration to find out.
 */
export function AdminOverviewPage() {
  const query = useQuery({
    queryKey: ["admin-overview"],
    queryFn: () => api<Overview>("/api/v1/admin/overview"),
    refetchInterval: 60000,
  });

  if (query.isLoading)
    return (
      <Box p={5}>
        <CircularProgress />
      </Box>
    );

  const data = query.data;
  if (!data) return null;

  return (
    <Box sx={{ p: { xs: 2.5, sm: 4, lg: 5 }, maxWidth: 1200, mx: "auto" }}>
      <Typography variant="h1">운영 현황</Typography>
      <Typography color="text.secondary" mt={0.7} mb={3}>
        muni {data.build.version} · commit {data.build.commit?.slice(0, 8) || "none"}
      </Typography>

      <Typography variant="h3" mb={1.5}>
        사용
      </Typography>
      <Grid container spacing={2} mb={3}>
        <Metric label="사용자" value={data.users.total} detail={`활성 ${data.users.active} · 관리자 ${data.users.admins}`} />
        <Metric
          label="정지된 계정"
          value={data.users.suspended}
          tone={data.users.suspended > 0 ? "warning.main" : undefined}
        />
        <Metric label="최근 7일 로그인" value={data.users.recentLogins} />
        <Metric label="워크스페이스" value={data.storage.workspaces} to="/admin/workspaces" />
        <Metric
          label="문서"
          value={data.documents.total}
          detail={`휴지통 ${data.documents.trashed} · 버전 ${data.documents.revisions.toLocaleString()}`}
        />
        <Metric
          label="이번 주"
          value={data.documents.editedThisWeek}
          detail={`새 문서 ${data.documents.createdThisWeek}`}
        />
      </Grid>

      <Typography variant="h3" mb={1.5}>
        저장과 접근
      </Typography>
      <Grid container spacing={2} mb={3}>
        <Metric
          label="첨부 파일"
          value={data.storage.attachments}
          detail={humanBytes(data.storage.bytes)}
        />
        <Metric label="열린 세션" value={data.storage.sessions} />
        <Metric label="사용 중인 API key" value={data.storage.apiKeys} />
        <Metric
          label="승인 대기 문서"
          value={data.documents.pendingApproval}
          to="/admin/audit"
          tone={data.documents.pendingApproval > 0 ? "warning.main" : undefined}
        />
      </Grid>

      <Grid container spacing={2}>
        <Grid size={{ xs: 12, md: 7 }}>
          <Card sx={{ p: 2.5, height: "100%" }}>
            <Typography variant="h3" mb={1.5}>
              연결 상태
            </Typography>
            <Typography variant="body2" color="text.secondary" mb={2}>
              설정에 적힌 내용을 확인한 결과입니다. 실제 연결 시험은 각 설정
              화면의 연결 테스트 버튼으로 합니다.
            </Typography>
            <Stack gap={1.25}>
              {data.checks.map((check) => (
                <Stack key={check.key} direction="row" alignItems="center" gap={1.25}>
                  <StateIcon state={check.state} />
                  <Typography sx={{ minWidth: 128 }}>{check.label}</Typography>
                  <Typography
                    variant="body2"
                    color={check.state === "warn" ? "warning.main" : "text.secondary"}
                    sx={{ flex: 1, wordBreak: "break-all" }}
                  >
                    {check.detail}
                  </Typography>
                  {check.setting && (
                    <Link component={RouterLink} to={check.setting} variant="body2">
                      설정
                    </Link>
                  )}
                </Stack>
              ))}
            </Stack>
          </Card>
        </Grid>
        <Grid size={{ xs: 12, md: 5 }}>
          <Card sx={{ p: 2.5, height: "100%" }}>
            <Stack direction="row" alignItems="baseline" gap={1} mb={1.5}>
              <Typography variant="h3" sx={{ flex: 1 }}>
                최근 활동
              </Typography>
              <Link component={RouterLink} to="/admin/audit" variant="body2">
                전체 보기
              </Link>
            </Stack>
            <Stack gap={1}>
              {data.activity.length === 0 && (
                <Typography color="text.secondary" variant="body2">
                  기록된 활동이 없습니다.
                </Typography>
              )}
              {data.activity.map((item, index) => (
                <Stack
                  key={`${item.action}-${index}`}
                  direction="row"
                  alignItems="center"
                  gap={1}
                >
                  <Chip size="small" label={item.action} sx={{ maxWidth: 190 }} />
                  <Typography variant="body2" sx={{ flex: 1 }} noWrap>
                    {item.actorName ?? "시스템"}
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    {formatDate(item.createdAt)}
                  </Typography>
                </Stack>
              ))}
            </Stack>
          </Card>
        </Grid>
      </Grid>
    </Box>
  );
}

function StateIcon({ state }: { state: "ok" | "off" | "warn" }) {
  if (state === "ok") return <CheckCircleOutline fontSize="small" color="success" />;
  if (state === "warn") return <ErrorOutline fontSize="small" color="warning" />;
  return <RemoveCircleOutline fontSize="small" color="disabled" />;
}

function Metric({
  label,
  value,
  detail,
  tone,
  to,
}: {
  label: string;
  value: number;
  detail?: string;
  tone?: string;
  to?: string;
}) {
  const card = (
    <Card sx={{ p: 2, height: "100%" }}>
      <Typography variant="caption" color="text.secondary">
        {label}
      </Typography>
      <Typography variant="h3" sx={{ mt: 0.5, color: tone }}>
        {value.toLocaleString()}
      </Typography>
      {detail && (
        <Typography variant="caption" color="text.secondary">
          {detail}
        </Typography>
      )}
    </Card>
  );
  return (
    <Grid size={{ xs: 6, sm: 4, md: 2 }}>
      {to ? (
        <Link component={RouterLink} to={to} underline="none" color="inherit">
          {card}
        </Link>
      ) : (
        card
      )}
    </Grid>
  );
}

/** humanBytes reads a size the way an operator would say it out loud. */
export function humanBytes(bytes: number): string {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value >= 100 || unit === 0 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`;
}
