import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Button,
  Chip,
  Collapse,
  Divider,
  FormControlLabel,
  Paper,
  Stack,
  Switch,
  TextField,
  Typography,
} from "@mui/material";
import {
  ContentCopyOutlined,
  LinkOutlined,
  LockOutlined,
} from "@mui/icons-material";
import { api, errorMessage, formatDate, jsonBody } from "../../../lib/api";

type ShareLink = {
  id: string;
  label: string;
  prefix: string;
  hasPassword: boolean;
  expiresAt?: string | null;
  maxViews?: number | null;
  viewCount: number;
  lastViewedAt?: string | null;
  revokedAt?: string | null;
  createdAt: string;
  createdBy: string;
  active: boolean;
};

/**
 * Read-only links for people who do not have an account here.
 *
 * The token is shown once, when the link is made, and never again — muni
 * stores only a hash of it, so no later request can produce it. Losing it
 * means making another link and revoking this one, which is the right trade:
 * the alternative is a table that is a list of working keys.
 */
export function PublicLinks({
  documentId,
  allowed,
}: {
  documentId: string;
  allowed: boolean;
}) {
  const client = useQueryClient();
  const [creating, setCreating] = useState(false);
  const [label, setLabel] = useState("");
  const [usePassword, setUsePassword] = useState(false);
  const [password, setPassword] = useState("");
  const [useExpiry, setUseExpiry] = useState(false);
  const [expiresAt, setExpiresAt] = useState("");
  const [useLimit, setUseLimit] = useState(false);
  const [maxViews, setMaxViews] = useState("1");
  const [fresh, setFresh] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const links = useQuery({
    queryKey: ["document-links", documentId],
    queryFn: () => api<ShareLink[]>(`/api/v1/documents/${documentId}/links`),
    enabled: allowed,
  });

  const create = useMutation({
    mutationFn: () =>
      api<{ token: string }>(`/api/v1/documents/${documentId}/links`, {
        method: "POST",
        ...jsonBody({
          label: label.trim(),
          ...(usePassword && password ? { password } : {}),
          ...(useExpiry && expiresAt
            ? { expiresAt: new Date(expiresAt).toISOString() }
            : {}),
          ...(useLimit ? { maxViews: Number(maxViews) || 1 } : {}),
        }),
      }),
    onSuccess: (result) => {
      setFresh(`${window.location.origin}/s/${result.token}`);
      setCreating(false);
      setLabel("");
      setPassword("");
      setUsePassword(false);
      setUseExpiry(false);
      setUseLimit(false);
      void client.invalidateQueries({ queryKey: ["document-links", documentId] });
    },
  });

  const revoke = useMutation({
    mutationFn: (id: string) =>
      api(`/api/v1/documents/${documentId}/links/${id}`, { method: "DELETE" }),
    onSuccess: () =>
      client.invalidateQueries({ queryKey: ["document-links", documentId] }),
  });

  if (!allowed) return null;

  return (
    <Stack gap={1.5}>
      <Divider />
      <Stack direction="row" alignItems="center" gap={1}>
        <LinkOutlined fontSize="small" color="disabled" />
        <Typography fontWeight={650} flex={1}>
          공개 링크
        </Typography>
        <Button size="small" onClick={() => setCreating((on) => !on)}>
          {creating ? "취소" : "링크 만들기"}
        </Button>
      </Stack>
      <Typography variant="body2" color="text.secondary">
        계정이 없는 사람도 이 링크로 문서를 <b>읽을</b> 수 있습니다. 링크를 가진
        사람이 누구인지 muni는 알지 못합니다.
      </Typography>

      {fresh && (
        <Alert severity="success" sx={{ alignItems: "flex-start" }}>
          <Stack gap={1}>
            <Typography variant="body2" fontWeight={650}>
              링크를 만들었습니다. 이 주소는 지금만 볼 수 있습니다.
            </Typography>
            <Stack direction="row" gap={1} alignItems="center" flexWrap="wrap">
              <Typography
                component="code"
                sx={{
                  fontFamily: "monospace",
                  fontSize: 13,
                  wordBreak: "break-all",
                  userSelect: "all",
                }}
              >
                {fresh}
              </Typography>
              <Button
                size="small"
                startIcon={<ContentCopyOutlined />}
                onClick={async () => {
                  try {
                    await navigator.clipboard.writeText(fresh);
                    setCopied(true);
                    setTimeout(() => setCopied(false), 1600);
                  } catch {
                    // Clipboard access can be refused; the text is selectable.
                  }
                }}
              >
                {copied ? "복사함" : "복사"}
              </Button>
            </Stack>
            <Typography variant="caption" color="text.secondary">
              muni는 링크를 해시로만 보관하므로 나중에 다시 보여줄 수 없습니다.
              잃어버리면 이 링크를 해지하고 새로 만드세요.
            </Typography>
          </Stack>
        </Alert>
      )}

      <Collapse in={creating}>
        <Stack gap={1.5} sx={{ pb: 1 }}>
          <TextField
            size="small"
            label="이름 (나중에 구분용)"
            placeholder="예: 고객사 검토용"
            value={label}
            onChange={(event) => setLabel(event.target.value)}
          />
          <FormControlLabel
            control={
              <Switch
                size="small"
                checked={usePassword}
                onChange={(event) => setUsePassword(event.target.checked)}
              />
            }
            label={<Typography variant="body2">비밀번호 걸기</Typography>}
          />
          {usePassword && (
            <TextField
              size="small"
              label="링크 비밀번호"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              helperText="4자 이상. 링크와 다른 경로로 전달하세요."
            />
          )}
          <FormControlLabel
            control={
              <Switch
                size="small"
                checked={useExpiry}
                onChange={(event) => setUseExpiry(event.target.checked)}
              />
            }
            label={<Typography variant="body2">만료 시각 정하기</Typography>}
          />
          {useExpiry && (
            <TextField
              size="small"
              type="datetime-local"
              value={expiresAt}
              onChange={(event) => setExpiresAt(event.target.value)}
              InputLabelProps={{ shrink: true }}
              label="만료"
            />
          )}
          <FormControlLabel
            control={
              <Switch
                size="small"
                checked={useLimit}
                onChange={(event) => setUseLimit(event.target.checked)}
              />
            }
            label={<Typography variant="body2">열람 횟수 제한</Typography>}
          />
          {useLimit && (
            <TextField
              size="small"
              type="number"
              label="최대 열람 횟수"
              value={maxViews}
              onChange={(event) => setMaxViews(event.target.value)}
              inputProps={{ min: 1, max: 100000 }}
            />
          )}
          {create.error && (
            <Alert severity="error">{errorMessage(create.error)}</Alert>
          )}
          <Button
            variant="contained"
            size="small"
            sx={{ alignSelf: "flex-start" }}
            disabled={create.isPending || (usePassword && password.length < 4)}
            onClick={() => create.mutate()}
          >
            {create.isPending ? "만드는 중…" : "만들기"}
          </Button>
        </Stack>
      </Collapse>

      <Stack gap={1}>
        {(links.data ?? []).map((link) => (
          <Paper key={link.id} variant="outlined" sx={{ p: 1.5 }}>
            <Stack direction="row" gap={1} alignItems="center" flexWrap="wrap">
              <Typography variant="body2" fontWeight={640} flex={1}>
                {link.label || "이름 없는 링크"}
                <Typography
                  component="span"
                  variant="caption"
                  color="text.secondary"
                  ml={0.8}
                >
                  {link.prefix}…
                </Typography>
              </Typography>
              {link.hasPassword && (
                <Chip
                  size="small"
                  variant="outlined"
                  icon={<LockOutlined fontSize="small" />}
                  label="비밀번호"
                />
              )}
              {!link.active && (
                <Chip size="small" color="default" label="사용 불가" />
              )}
              {link.active && (
                <Button
                  size="small"
                  color="error"
                  disabled={revoke.isPending}
                  onClick={() => revoke.mutate(link.id)}
                >
                  해지
                </Button>
              )}
            </Stack>
            <Typography variant="caption" color="text.secondary">
              {link.viewCount}회 열람
              {link.maxViews ? ` / ${link.maxViews}회까지` : ""}
              {link.lastViewedAt
                ? ` · 마지막 ${formatDate(link.lastViewedAt)}`
                : " · 아직 열람 없음"}
              {link.expiresAt ? ` · ${formatDate(link.expiresAt)} 만료` : ""}
              {link.revokedAt ? ` · ${formatDate(link.revokedAt)} 해지됨` : ""}
            </Typography>
          </Paper>
        ))}
        {links.data && links.data.length === 0 && !creating && (
          <Typography variant="body2" color="text.secondary">
            아직 만든 링크가 없습니다.
          </Typography>
        )}
      </Stack>
    </Stack>
  );
}
