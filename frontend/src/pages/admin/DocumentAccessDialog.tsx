import { useQuery } from "@tanstack/react-query";
import {
  Alert,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  LinearProgress,
  Stack,
  Typography,
  Button,
} from "@mui/material";
import { api, formatDate } from "../../lib/api";

type Entry = {
  userId: string;
  displayName: string;
  email: string;
  role: string;
  via: "OWNER" | "DIRECT" | "WORKSPACE";
  expiresAt?: string | null;
  suspended: boolean;
  alsoAdmin: boolean;
};

type Access = {
  document: {
    title: string;
    workspace: string;
    visibility: string;
    ownerName: string;
    trashed: boolean;
  };
  entries: Entry[];
  everyone: { applies: boolean; role?: string; people?: number; reason?: string };
  admins: number;
  notes: string[];
};

const viaLabel: Record<Entry["via"], string> = {
  OWNER: "소유자",
  DIRECT: "직접 공유",
  WORKSPACE: "워크스페이스 구성원",
};

const roleLabel: Record<string, string> = {
  OWNER: "모든 권한",
  EDITOR: "편집",
  COMMENTER: "댓글",
  VIEWER: "읽기",
};

const visibilityLabel: Record<string, string> = {
  RESTRICTED: "제한됨",
  WORKSPACE: "워크스페이스 공개",
  ORGANIZATION: "조직 전체 공개",
  LINK: "링크 공개",
};

/**
 * Who can open this document right now.
 *
 * The audit log already answered who did open it. This is the other question,
 * the one an audit actually starts with, and it had no screen: the answer
 * lives across three tables and a visibility column, combined by precedence
 * rules that only existed in Go.
 */
export function DocumentAccessDialog({
  documentId,
  onClose,
}: {
  documentId: string | null;
  onClose: () => void;
}) {
  const query = useQuery({
    queryKey: ["document-access", documentId],
    queryFn: () =>
      api<Access>(`/api/v1/admin/documents/${documentId}/access`),
    enabled: Boolean(documentId),
  });
  const access = query.data;

  return (
    <Dialog open={Boolean(documentId)} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>지금 이 문서를 열 수 있는 사람</DialogTitle>
      <DialogContent>
        {query.isLoading && <LinearProgress sx={{ my: 2 }} />}
        {access && (
          <Stack gap={2} mt={1}>
            <Stack direction="row" gap={1} alignItems="center" flexWrap="wrap">
              <Typography fontWeight={700}>{access.document.title}</Typography>
              <Chip
                size="small"
                label={
                  visibilityLabel[access.document.visibility] ??
                  access.document.visibility
                }
                color={
                  access.document.visibility === "ORGANIZATION"
                    ? "warning"
                    : "default"
                }
              />
            </Stack>

            {access.notes.map((note) => (
              <Alert key={note} severity="warning">
                {note}
              </Alert>
            ))}

            {access.everyone.applies && (
              <Alert severity="warning">
                {access.everyone.reason} 지금 사용 중인 계정{" "}
                <b>{access.everyone.people}명</b>이 해당합니다.
              </Alert>
            )}

            <Stack gap={0.5}>
              {access.entries.map((entry) => (
                <Stack
                  key={entry.userId}
                  direction="row"
                  gap={1.2}
                  alignItems="center"
                  sx={{ py: 0.8, borderBottom: 1, borderColor: "divider" }}
                >
                  <Stack sx={{ flex: 1, minWidth: 0 }}>
                    <Stack direction="row" gap={0.8} alignItems="center">
                      <Typography variant="body2" fontWeight={640} noWrap>
                        {entry.displayName}
                      </Typography>
                      {entry.suspended && (
                        <Chip size="small" color="error" variant="outlined" label="정지" />
                      )}
                      {entry.alsoAdmin && (
                        <Chip size="small" variant="outlined" label="관리자" />
                      )}
                    </Stack>
                    <Typography variant="caption" color="text.secondary" noWrap>
                      {entry.email} · {viaLabel[entry.via]}
                      {entry.expiresAt
                        ? ` · ${formatDate(entry.expiresAt, false)}까지`
                        : ""}
                    </Typography>
                  </Stack>
                  <Chip size="small" label={roleLabel[entry.role] ?? entry.role} />
                </Stack>
              ))}
              {access.entries.length === 0 && !access.everyone.applies && (
                <Typography variant="body2" color="text.secondary" py={1}>
                  소유자와 관리자 외에는 아무도 열 수 없습니다.
                </Typography>
              )}
            </Stack>

            <Divider />
            <Typography variant="body2" color="text.secondary">
              여기에 더해 <b>서비스 관리자 {access.admins}명</b>이 모든 문서를
              열 수 있습니다. 문서마다 다른 것이 아니라 역할에 따라오는 것이라
              목록에 넣지 않았습니다.
            </Typography>
          </Stack>
        )}
      </DialogContent>
      <DialogActions>
        <Button variant="contained" onClick={onClose}>
          닫기
        </Button>
      </DialogActions>
    </Dialog>
  );
}
