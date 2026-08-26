import { useMutation, useQuery } from "@tanstack/react-query";
import {
  DescriptionOutlined,
  FactCheckOutlined,
  GroupOutlined,
  KeyOutlined,
  DevicesOutlined,
  WarningAmberOutlined,
} from "@mui/icons-material";
import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControlLabel,
  LinearProgress,
  Stack,
  Switch,
  TextField,
  Typography,
} from "@mui/material";
import { useState } from "react";
import { api, errorMessage, jsonBody } from "../../lib/api";
import type { User } from "../../types";

type Counts = {
  documents: number;
  trashedDocuments: number;
  sharedWorkspaces: number;
  memberships: number;
  activeApiKeys: number;
  openSessions: number;
  blockingApprovals: number;
  pendingRequests: number;
};

type Belongings = {
  user: { id: string; displayName: string; email: string; status: string };
  counts: Counts;
  documents: {
    id: string;
    title: string;
    workspace: string;
    trashed: boolean;
  }[];
  truncated: boolean;
};

type Moved = Record<string, number>;

export function OffboardDialog({
  user,
  onClose,
  onDone,
}: {
  user: User | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [recipient, setRecipient] = useState<User | null>(null);
  const [includeTrashed, setIncludeTrashed] = useState(true);
  const [reassignApprovals, setReassignApprovals] = useState(true);
  const [revokeApiKeys, setRevokeApiKeys] = useState(true);
  const [endSessions, setEndSessions] = useState(true);
  const [suspend, setSuspend] = useState(true);
  const [moved, setMoved] = useState<Moved | null>(null);

  const belongings = useQuery({
    queryKey: ["offboard", user?.id],
    queryFn: () =>
      api<Belongings>(`/api/v1/admin/users/${user?.id}/belongings`),
    enabled: Boolean(user),
  });

  const candidates = useQuery({
    queryKey: ["admin-users", "offboard-candidates"],
    queryFn: () => api<User[]>("/api/v1/admin/users?limit=200"),
    enabled: Boolean(user),
  });

  const run = useMutation({
    mutationFn: () =>
      api<{ moved: Moved }>(`/api/v1/admin/users/${user?.id}/offboard`, {
        method: "POST",
        ...jsonBody({
          transferTo: recipient?.id,
          includeTrashed,
          reassignApprovals,
          revokeApiKeys,
          endSessions,
          suspend,
        }),
      }),
    onSuccess: (result) => {
      setMoved(result.moved);
      onDone();
    },
  });

  const close = () => {
    setRecipient(null);
    setMoved(null);
    run.reset();
    onClose();
  };

  const counts = belongings.data?.counts;
  const nothingHeld =
    counts &&
    counts.documents === 0 &&
    counts.trashedDocuments === 0 &&
    counts.sharedWorkspaces === 0 &&
    counts.blockingApprovals === 0;

  if (moved) {
    const labels: Record<string, string> = {
      documents: "문서를 넘겼습니다",
      workspaces: "워크스페이스를 넘겼습니다",
      workspacesJoined: "인계받는 사람을 워크스페이스에 넣었습니다",
      approvalsReassigned: "결재 차례를 넘겼습니다",
      approvalsSkipped: "이미 결재선에 있어 건너뛴 단계입니다",
      approvalsCancelled: "결재자가 없어져 취소하고 기안 상태로 되돌린 문서입니다",
      apiKeysRevoked: "API 키를 폐기했습니다",
      sessionsEnded: "세션을 종료했습니다",
      suspended: "계정을 정지했습니다",
    };
    return (
      <Dialog open onClose={close} fullWidth maxWidth="sm">
        <DialogTitle>{user?.displayName} 정리를 마쳤습니다</DialogTitle>
        <DialogContent>
          <Stack gap={1} mt={1}>
            {Object.entries(moved)
              .filter(([, value]) => value > 0)
              .map(([key, value]) => (
                <Stack key={key} direction="row" gap={1.5} alignItems="center">
                  <Chip size="small" label={value} />
                  <Typography variant="body2">
                    {labels[key] ?? key}
                  </Typography>
                </Stack>
              ))}
            {(moved.approvalsCancelled ?? 0) > 0 && (
              <Alert severity="info" sx={{ mt: 1 }}>
                결재선에 남은 사람이 없어진 요청은 <b>승인하지 않고 취소</b>했습니다.
                결재자가 사라진 것이 승인은 아니기 때문입니다. 해당 문서는 기안
                상태로 돌아갔고, 작성자가 새 결재선으로 다시 올리면 됩니다.
              </Alert>
            )}
            <Alert severity="info" sx={{ mt: 1 }}>
              계정은 남겨 둡니다. 감사 로그와 결재 기록이 이 계정을 가리키고
              있어, 지우면 누가 했는지가 사라지거나 틀리게 됩니다.
            </Alert>
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button variant="contained" onClick={close}>
            닫기
          </Button>
        </DialogActions>
      </Dialog>
    );
  }

  return (
    <Dialog open={Boolean(user)} onClose={close} fullWidth maxWidth="sm">
      <DialogTitle>{user?.displayName} 정리</DialogTitle>
      <DialogContent>
        {belongings.isLoading && <LinearProgress sx={{ my: 2 }} />}
        {counts && (
          <Stack gap={2.5} mt={1}>
            {counts.blockingApprovals > 0 && (
              <Alert severity="warning" icon={<WarningAmberOutlined />}>
                이 사람 차례에서 멈춰 있는 결재가{" "}
                <b>{counts.blockingApprovals}건</b> 있습니다. 결재선은 차례가 된
                사람만 처리할 수 있으므로, 이대로 계정을 정지하면 그 문서들은
                영영 진행되지 않습니다.
              </Alert>
            )}

            <Stack gap={1}>
              <Held
                icon={<DescriptionOutlined fontSize="small" />}
                label="문서"
                value={`${counts.documents}건${
                  counts.trashedDocuments
                    ? ` (휴지통 ${counts.trashedDocuments}건)`
                    : ""
                }`}
              />
              <Held
                icon={<GroupOutlined fontSize="small" />}
                label="소유한 공유 워크스페이스"
                value={`${counts.sharedWorkspaces}개`}
              />
              <Held
                icon={<FactCheckOutlined fontSize="small" />}
                label="막고 있는 결재"
                value={`${counts.blockingApprovals}건`}
              />
              <Held
                icon={<KeyOutlined fontSize="small" />}
                label="살아 있는 API 키"
                value={`${counts.activeApiKeys}개`}
              />
              <Held
                icon={<DevicesOutlined fontSize="small" />}
                label="열려 있는 세션"
                value={`${counts.openSessions}개`}
              />
            </Stack>

            {nothingHeld && (
              <Alert severity="success">
                넘길 것이 없습니다. 정지만 해도 됩니다.
              </Alert>
            )}

            <Divider />

            <Autocomplete
              options={(candidates.data ?? []).filter(
                (c) => c.id !== user?.id && c.status === "ACTIVE",
              )}
              getOptionLabel={(option) =>
                `${option.displayName} (${option.email})`
              }
              value={recipient}
              onChange={(_, next) => setRecipient(next)}
              renderInput={(params) => (
                <TextField {...params} label="인계받을 사람" required />
              )}
            />

            <Stack>
              <FormControlLabel
                control={
                  <Switch
                    checked={includeTrashed}
                    onChange={(e) => setIncludeTrashed(e.target.checked)}
                  />
                }
                label="휴지통에 있는 문서도 함께"
              />
              <FormControlLabel
                control={
                  <Switch
                    checked={reassignApprovals}
                    onChange={(e) => setReassignApprovals(e.target.checked)}
                  />
                }
                label="결재 차례 넘기기"
              />
              <FormControlLabel
                control={
                  <Switch
                    checked={revokeApiKeys}
                    onChange={(e) => setRevokeApiKeys(e.target.checked)}
                  />
                }
                label="API 키 폐기"
              />
              <FormControlLabel
                control={
                  <Switch
                    checked={endSessions}
                    onChange={(e) => setEndSessions(e.target.checked)}
                  />
                }
                label="열려 있는 세션 종료"
              />
              <FormControlLabel
                control={
                  <Switch
                    checked={suspend}
                    onChange={(e) => setSuspend(e.target.checked)}
                  />
                }
                label="계정 정지"
              />
            </Stack>

            <Typography variant="body2" color="text.secondary">
              전부 한 트랜잭션에서 처리합니다. 도중에 실패하면 아무것도 바뀌지
              않습니다 — 문서만 넘어가고 세션은 남는 절반짜리 정리가 가장
              곤란하기 때문입니다.
            </Typography>

            {run.error && (
              <Alert severity="error">{errorMessage(run.error)}</Alert>
            )}
          </Stack>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={close}>취소</Button>
        <Button
          variant="contained"
          color="warning"
          disabled={!recipient || run.isPending}
          onClick={() => run.mutate()}
        >
          {run.isPending ? "정리하는 중…" : "정리하기"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

function Held({
  icon,
  label,
  value,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
}) {
  return (
    <Stack direction="row" gap={1.2} alignItems="center">
      <Box sx={{ color: "text.secondary", display: "flex" }}>{icon}</Box>
      <Typography variant="body2" color="text.secondary" sx={{ flex: 1 }}>
        {label}
      </Typography>
      <Typography variant="body2" fontWeight={700}>
        {value}
      </Typography>
    </Stack>
  );
}
