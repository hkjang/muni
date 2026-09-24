import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Button,
  Checkbox,
  Chip,
  FormControl,
  FormControlLabel,
  Grid,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from "@mui/material";
import { api, errorMessage, jsonBody } from "../../lib/api";
import type { MailDelivery, MailSettings as MailValue } from "../../types";

/** mailEvents is what muni mails about, one switch each. The list is the
 * server's catalogue; a notification type not here is never mailed. */
export const mailEvents: {
  key: keyof MailValue["notify"];
  label: string;
  hint: string;
}[] = [
  {
    key: "approvalRequest",
    label: "검토·결재 요청",
    hint: "검토할 문서가 생겼거나 결재 차례가 되었을 때",
  },
  {
    key: "approvalDecision",
    label: "검토 결과",
    hint: "내가 올린 문서가 승인·반려되었을 때",
  },
  { key: "mention", label: "댓글 멘션", hint: "댓글에서 나를 @로 불렀을 때" },
  {
    key: "apiKeyExpiring",
    label: "API 키 만료 임박",
    hint: "내 API 키가 7일 안에 만료될 때 (한 번만)",
  },
];

/** eventLabel is how a delivery record names what it was about. */
export function eventLabel(event: string): string {
  switch (event) {
    case "APPROVAL_REQUEST":
      return "검토·결재 요청";
    case "APPROVAL_DECISION":
      return "검토 결과";
    case "MENTION":
      return "댓글 멘션";
    case "API_KEY_EXPIRING":
      return "API 키 만료 임박";
    case "DIGEST":
      return "묶음";
    case "TEST":
      return "시험 발송";
    default:
      return event;
  }
}

type Props = {
  value: MailValue;
  onChange: (value: MailValue) => void;
};

/**
 * MailSettings is the 메일 알림 tab: the relay, which events go out, a test
 * send that proves the relay with the form as it stands, and the record of
 * what left — every attempt, so "it never arrived" has an answer.
 */
export function MailSettings({ value, onChange }: Props) {
  const client = useQueryClient();
  const set = (patch: Partial<MailValue>) => onChange({ ...value, ...patch });
  const test = useMutation({
    mutationFn: () =>
      api<{ sentTo: string }>("/api/v1/admin/settings/test-mail", {
        method: "POST",
        ...jsonBody(value),
      }),
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["mail-deliveries"] });
    },
  });
  const deliveries = useQuery({
    queryKey: ["mail-deliveries"],
    queryFn: () =>
      api<{
        items: MailDelivery[];
        summary: { total: number; sent: number; failed: number };
      }>("/api/v1/admin/mail/deliveries?limit=50"),
  });

  return (
    <Stack gap={2} maxWidth={760}>
      <Typography variant="h6">사내 메일 서버</Typography>
      <Typography variant="body2" color="text.secondary">
        muni가 쓰는 메일 서버는 여기에 적은 것 하나뿐입니다. 외부 발송 서비스로
        나가는 연결은 없습니다. 사내 릴레이는 대개 25번 포트에 인증도 암호화도
        없이 받으므로 그것이 기본값이고, 계정과 보안은 릴레이가 요구할 때만
        채웁니다.
      </Typography>
      <FormControlLabel
        control={
          <Checkbox
            checked={value.enabled}
            onChange={(_, checked) => set({ enabled: checked })}
          />
        }
        label="메일 알림 보내기"
      />
      <Grid container spacing={2}>
        <Grid size={{ xs: 12, sm: 7 }}>
          <TextField
            fullWidth
            label="릴레이 주소"
            placeholder="relay.company.co.kr"
            value={value.smtpHost}
            onChange={(e) => set({ smtpHost: e.target.value })}
          />
        </Grid>
        <Grid size={{ xs: 6, sm: 2 }}>
          <TextField
            fullWidth
            type="number"
            label="포트"
            value={value.smtpPort}
            slotProps={{ htmlInput: { min: 1, max: 65535 } }}
            onChange={(e) => set({ smtpPort: Number(e.target.value) })}
          />
        </Grid>
        <Grid size={{ xs: 6, sm: 3 }}>
          <FormControl fullWidth>
            <InputLabel id="mail-security">보안</InputLabel>
            <Select
              labelId="mail-security"
              label="보안"
              value={value.security}
              onChange={(e) => set({ security: e.target.value })}
            >
              <MenuItem value="auto">자동 (서버가 알리는 대로)</MenuItem>
              <MenuItem value="none">사용 안 함</MenuItem>
              <MenuItem value="starttls">STARTTLS (587)</MenuItem>
              <MenuItem value="tls">TLS (465)</MenuItem>
            </Select>
          </FormControl>
        </Grid>
      </Grid>
      <Grid container spacing={2}>
        <Grid size={{ xs: 12, sm: 6 }}>
          <TextField
            fullWidth
            label="계정 (릴레이가 요구하는 경우)"
            value={value.username}
            onChange={(e) => set({ username: e.target.value })}
          />
        </Grid>
        <Grid size={{ xs: 12, sm: 6 }}>
          <TextField
            fullWidth
            type="password"
            label={`비밀번호${value.passwordSet ? " (설정됨)" : ""}`}
            placeholder={value.passwordSet ? "변경할 때만 입력" : ""}
            value={value.password ?? ""}
            onChange={(e) => set({ password: e.target.value })}
            helperText="저장한 비밀번호는 화면에 다시 나오지 않습니다."
          />
        </Grid>
        <Grid size={{ xs: 12, sm: 6 }}>
          <TextField
            fullWidth
            label="보내는 주소"
            placeholder="muni-noreply@company.co.kr"
            value={value.fromAddress}
            onChange={(e) => set({ fromAddress: e.target.value })}
            helperText="비우면 계정 주소를 사용합니다."
          />
        </Grid>
        <Grid size={{ xs: 12, sm: 6 }}>
          <TextField
            fullWidth
            label="보내는 사람 이름"
            placeholder="muni 알림"
            value={value.fromName}
            onChange={(e) => set({ fromName: e.target.value })}
          />
        </Grid>
        <Grid size={{ xs: 12, sm: 8 }}>
          <TextField
            fullWidth
            label="메일에 넣을 서비스 주소"
            placeholder="https://muni.company.co.kr"
            value={value.baseUrl}
            onChange={(e) => set({ baseUrl: e.target.value })}
            helperText="알림 메일의 링크가 향하는 곳입니다. 비우면 링크 없이 보냅니다."
          />
        </Grid>
        <Grid size={{ xs: 12, sm: 4 }}>
          <TextField
            fullWidth
            type="number"
            label="제한 시간 (초)"
            value={value.timeoutSeconds}
            slotProps={{ htmlInput: { min: 1, max: 300 } }}
            onChange={(e) => set({ timeoutSeconds: Number(e.target.value) })}
            helperText="비우면 10초"
          />
        </Grid>
      </Grid>
      <FormControlLabel
        control={
          <Checkbox
            checked={value.skipTlsVerify}
            onChange={(_, checked) => set({ skipTlsVerify: checked })}
          />
        }
        label="서버 인증서를 검증하지 않음 (사설 인증기관을 쓰는 경우)"
      />

      <Typography variant="subtitle1" sx={{ mt: 1 }}>
        보낼 이벤트
      </Typography>
      <Typography variant="body2" color="text.secondary">
        사람이 실제로 기다리는 일만 보냅니다. 자기가 한 일은 자기에게 가지 않고,
        한 사람에게 여러 알림이 쌓이면 한 통으로 묶습니다.{" "}
        <strong>문서 내용은 담기지 않습니다.</strong>
      </Typography>
      <Grid container spacing={1}>
        {mailEvents.map((event) => (
          <Grid size={{ xs: 12, sm: 6 }} key={event.key}>
            <FormControlLabel
              control={
                <Checkbox
                  checked={value.notify[event.key]}
                  onChange={(_, checked) =>
                    set({ notify: { ...value.notify, [event.key]: checked } })
                  }
                />
              }
              label={
                <span>
                  {event.label}
                  <Typography
                    component="span"
                    variant="body2"
                    color="text.secondary"
                    sx={{ ml: 1 }}
                  >
                    {event.hint}
                  </Typography>
                </span>
              }
            />
          </Grid>
        ))}
      </Grid>

      <Stack direction="row" gap={2} alignItems="center" flexWrap="wrap">
        <Button
          variant="outlined"
          onClick={() => test.mutate()}
          disabled={test.isPending || !value.smtpHost.trim()}
        >
          내 주소로 시험 메일 보내기
        </Button>
        <Typography variant="body2" color="text.secondary">
          지금 폼에 적힌 값으로 보냅니다. 저장하지 않아도 릴레이를 확인할 수
          있습니다.
        </Typography>
      </Stack>
      {test.isSuccess && (
        <Alert severity="success" onClose={() => test.reset()}>
          {test.data.sentTo} 주소로 시험 메일을 보냈습니다. 받은 편지함을
          확인해 주세요.
        </Alert>
      )}
      {test.error && (
        <Alert severity="error" onClose={() => test.reset()}>
          {errorMessage(test.error)}
        </Alert>
      )}

      <Stack
        direction="row"
        alignItems="center"
        justifyContent="space-between"
        sx={{ mt: 2 }}
      >
        <Typography variant="subtitle1">발송 기록</Typography>
        <Stack direction="row" gap={1} alignItems="center">
          {deliveries.data && (
            <Typography variant="body2" color="text.secondary">
              전체 {deliveries.data.summary.total}건 · 성공{" "}
              {deliveries.data.summary.sent} · 실패{" "}
              {deliveries.data.summary.failed}
            </Typography>
          )}
          <Button size="small" onClick={() => void deliveries.refetch()}>
            새로 고침
          </Button>
        </Stack>
      </Stack>
      <Typography variant="body2" color="text.secondary">
        시도마다 한 줄입니다 — 성공도 남겨서 「안 왔다」는 문의에 답할 수 있고,
        본문은 담지 않습니다. 보존 기간은 「보존 정책」의 감사 로그와 같습니다.
      </Typography>
      {deliveries.data && deliveries.data.items.length === 0 && (
        <Typography variant="body2" color="text.secondary">
          아직 보낸 메일이 없습니다.
        </Typography>
      )}
      {deliveries.data && deliveries.data.items.length > 0 && (
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>시각</TableCell>
              <TableCell>이벤트</TableCell>
              <TableCell>받는 사람</TableCell>
              <TableCell>제목</TableCell>
              <TableCell>결과</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {deliveries.data.items.map((item) => (
              <TableRow key={item.id}>
                <TableCell sx={{ whiteSpace: "nowrap" }}>
                  {new Date(item.createdAt).toLocaleString("ko-KR")}
                </TableCell>
                <TableCell sx={{ whiteSpace: "nowrap" }}>
                  {eventLabel(item.event)}
                  {item.notifications > 1 ? ` (${item.notifications}건)` : ""}
                </TableCell>
                <TableCell>{item.recipient}</TableCell>
                <TableCell>{item.subject}</TableCell>
                <TableCell>
                  {item.status === "sent" ? (
                    <Chip size="small" color="success" label="보냄" />
                  ) : (
                    <Chip
                      size="small"
                      color="error"
                      label="실패"
                      title={item.error}
                    />
                  )}
                  {item.status === "failed" && item.error && (
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      display="block"
                    >
                      {item.error}
                    </Typography>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </Stack>
  );
}
