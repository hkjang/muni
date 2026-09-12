import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Box,
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
import type {
  TrackingProvider,
  TrackingSettings as TrackingValue,
  TrackingViolation,
} from "../../types";

/** MAX_SNIPPET_BYTES mirrors the server's limit so the form can say so
 * before the save is refused. */
export const MAX_SNIPPET_BYTES = 8 * 1024;

const providers: { value: TrackingProvider; label: string }[] = [
  { value: "momento", label: "Momento (사내 수집기)" },
  { value: "matomo", label: "Matomo" },
  { value: "ga4", label: "Google Analytics 4" },
  { value: "gtm", label: "Google Tag Manager" },
  { value: "custom", label: "직접 붙여넣기" },
  { value: "none", label: "없음" },
];

export function snippetBytes(snippet: string): number {
  return new TextEncoder().encode(snippet).length;
}

type Props = {
  value: TrackingValue;
  onChange: (value: TrackingValue) => void;
  /** Called when the allow list changed on the server behind the form's
   * back, so an unsaved form does not overwrite it on the next save. */
  onAllowedHosts: (allowedHosts: string) => void;
};

/**
 * TrackingSettings is the 방문 추적 tab of the settings screen: which tracker
 * to attach, and the list of what the content security policy blocked so the
 * administrator can allow it with one click instead of reading the console.
 */
export function TrackingSettings({ value, onChange, onAllowedHosts }: Props) {
  const client = useQueryClient();
  const set = (patch: Partial<TrackingValue>) =>
    onChange({ ...value, ...patch });
  const violations = useQuery({
    queryKey: ["tracking-violations"],
    queryFn: () =>
      api<TrackingViolation[]>("/api/v1/admin/tracking/violations"),
    refetchInterval: 15_000,
  });
  const allow = useMutation({
    mutationFn: (origin: string) =>
      api<{ allowedHosts: string }>("/api/v1/admin/tracking/allow", {
        method: "POST",
        ...jsonBody({ origin }),
      }),
    onSuccess: (data) => {
      onAllowedHosts(data.allowedHosts);
      void client.invalidateQueries({ queryKey: ["tracking-violations"] });
      void client.invalidateQueries({ queryKey: ["admin-settings"] });
    },
  });
  const clear = useMutation({
    mutationFn: () =>
      api("/api/v1/admin/tracking/violations", { method: "DELETE" }),
    onSuccess: () =>
      void client.invalidateQueries({ queryKey: ["tracking-violations"] }),
  });
  const bytes = snippetBytes(value.customSnippet);
  const tooLong = bytes > MAX_SNIPPET_BYTES;
  return (
    <Stack gap={2} maxWidth={760}>
      <Box mb={1}>
        <Typography variant="h3">방문 추적</Typography>
        <Typography color="text.secondary" mt={0.5}>
          어떤 화면이 실제로 쓰이는지 세는 스크립트를 페이지에 붙입니다. 기본은
          꺼짐이고, 켜도 문서 내용은 보내지 않습니다.
        </Typography>
      </Box>
      <FormControlLabel
        control={
          <Checkbox
            checked={value.enabled}
            onChange={(_, checked) => set({ enabled: checked })}
          />
        }
        label="방문 추적 켜기"
      />
      <Grid container spacing={2}>
        <Grid size={{ xs: 12, sm: 7 }}>
          <FormControl fullWidth>
            <InputLabel>제공자</InputLabel>
            <Select
              label="제공자"
              value={value.provider}
              onChange={(e) =>
                set({ provider: e.target.value as TrackingProvider })
              }
            >
              {providers.map((provider) => (
                <MenuItem key={provider.value} value={provider.value}>
                  {provider.label}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        </Grid>
        <Grid size={{ xs: 12, sm: 5 }}>
          <FormControl fullWidth>
            <InputLabel>넣을 자리</InputLabel>
            <Select
              label="넣을 자리"
              value={value.placement}
              onChange={(e) =>
                set({ placement: e.target.value as "head" | "body" })
              }
            >
              <MenuItem value="head">&lt;head&gt; 끝</MenuItem>
              <MenuItem value="body">&lt;body&gt; 끝</MenuItem>
            </Select>
          </FormControl>
        </Grid>
      </Grid>
      {value.provider === "momento" && (
        <>
          <TextField
            label="Momento 수집기 주소"
            placeholder="https://momento.internal"
            value={value.momentoUrl}
            onChange={(e) => set({ momentoUrl: e.target.value })}
            helperText="사내에서 띄운 Momento 의 주소입니다. 데이터가 밖으로 나가지 않는 유일한 선택지입니다."
          />
          <TextField
            label="사이트 id"
            value={value.momentoSiteId}
            onChange={(e) => set({ momentoSiteId: e.target.value })}
          />
          <FormControlLabel
            control={
              <Checkbox
                checked={value.momentoProxy}
                onChange={(_, checked) => set({ momentoProxy: checked })}
              />
            }
            label="muni 를 거쳐 보내기 (같은 오리진 프록시, /momento/*)"
          />
          <Typography variant="body2" color="text.secondary">
            프록시를 거치면 브라우저는 muni 하고만 이야기하므로 보안 정책에 바깥
            주소가 등장하지 않습니다. 끄면 수집기 주소가 정책에 더해집니다.
          </Typography>
        </>
      )}
      {(value.provider === "ga4" || value.provider === "gtm") && (
        <TextField
          label={value.provider === "ga4" ? "측정 ID (G-…)" : "컨테이너 ID (GTM-…)"}
          value={value.measurementId}
          onChange={(e) => set({ measurementId: e.target.value })}
          helperText="구글 서버로 나갑니다. 폐쇄망에서는 닿지 않습니다."
        />
      )}
      {value.provider === "matomo" && (
        <>
          <TextField
            label="Matomo 주소"
            placeholder="https://matomo.internal"
            value={value.matomoUrl}
            onChange={(e) => set({ matomoUrl: e.target.value })}
          />
          <TextField
            label="사이트 id"
            value={value.matomoSiteId}
            onChange={(e) => set({ matomoSiteId: e.target.value })}
          />
        </>
      )}
      {value.provider === "custom" && (
        <TextField
          multiline
          minRows={5}
          label="추적 코드"
          placeholder={'<script async src="https://…/tracker.js"></script>'}
          value={value.customSnippet}
          onChange={(e) => set({ customSnippet: e.target.value })}
          error={tooLong}
          helperText={`${bytes.toLocaleString()} / ${MAX_SNIPPET_BYTES.toLocaleString()} 바이트. 코드 안에 적힌 http(s) 주소는 보안 정책에 자동으로 더해집니다.`}
          slotProps={{ htmlInput: { spellCheck: false } }}
        />
      )}
      <TextField
        label="추가로 허용할 출처"
        placeholder="https://cdn.internal, https://pixel.internal"
        value={value.allowedHosts}
        onChange={(e) => set({ allowedHosts: e.target.value })}
        helperText="코드에서 자동으로 읽지 못한 주소를 쉼표로 구분해 적습니다. 아래 차단 목록의 「허용」이 여기에 더합니다."
      />
      <FormControlLabel
        control={
          <Checkbox
            checked={value.includeAdmin}
            onChange={(_, checked) => set({ includeAdmin: checked })}
          />
        }
        label="서비스 관리 화면에서도 추적"
      />
      <Alert severity="info">
        muni 의 보안 정책은 자기 오리진의 스크립트만 허용합니다. 추적 코드는
        요청마다 새로 만든 nonce 를 달고 나가며, 정책을{" "}
        <code>'unsafe-inline'</code> 으로 풀지 않습니다. 켠 동안 브라우저가
        차단한 주소가 아래에 쌓입니다.
      </Alert>
      <Stack direction="row" alignItems="center" justifyContent="space-between">
        <Typography variant="h3">정책이 차단한 출처</Typography>
        <Button
          size="small"
          onClick={() => clear.mutate()}
          disabled={clear.isPending || !violations.data?.length}
        >
          목록 비우기
        </Button>
      </Stack>
      {(allow.error || clear.error) && (
        <Alert severity="error">{errorMessage(allow.error || clear.error)}</Alert>
      )}
      {violations.data && violations.data.length === 0 && (
        <Typography variant="body2" color="text.secondary">
          기록된 차단이 없습니다. 추적을 켜고 화면을 새로 고친 뒤에도 비어
          있으면 정책이 코드를 막지 않은 것입니다.
        </Typography>
      )}
      {!!violations.data?.length && (
        <Table size="small" aria-label="정책이 차단한 출처">
          <TableHead>
            <TableRow>
              <TableCell>출처</TableCell>
              <TableCell>지시어</TableCell>
              <TableCell align="right">횟수</TableCell>
              <TableCell />
            </TableRow>
          </TableHead>
          <TableBody>
            {violations.data.map((item) => (
              <TableRow key={`${item.directive} ${item.origin}`}>
                <TableCell sx={{ fontFamily: "monospace" }}>
                  {item.origin}
                </TableCell>
                <TableCell>{item.directive}</TableCell>
                <TableCell align="right">{item.count}</TableCell>
                <TableCell align="right">
                  {item.allowed ? (
                    <Chip size="small" label="허용됨" color="success" />
                  ) : (
                    <Button
                      size="small"
                      variant="outlined"
                      onClick={() => allow.mutate(item.origin)}
                      disabled={allow.isPending}
                    >
                      허용
                    </Button>
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
