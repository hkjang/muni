import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApprovalOutlined, Check, Close } from "@mui/icons-material";
import {
  Alert,
  Box,
  Button,
  Card,
  Chip,
  Stack,
  Typography,
} from "@mui/material";
import { useNavigate } from "react-router-dom";
import { api, errorMessage, formatDate, jsonBody } from "../lib/api";
import { EmptyState } from "../components/EmptyState";

type Approval = {
  id: string;
  documentId: string;
  documentTitle: string;
  revision: number;
  requester: { id: string; displayName: string };
  status: string;
  requiredApprovals: number;
  approvedCount: number;
  createdAt: string;
};
export function ApprovalsPage() {
  const navigate = useNavigate();
  const client = useQueryClient();
  const { data: items = [], error } = useQuery({
    queryKey: ["approvals"],
    queryFn: () => api<Approval[]>("/api/v1/approvals"),
  });
  const decide = useMutation({
    mutationFn: ({ id, decision }: { id: string; decision: string }) =>
      api<void>(`/api/v1/approvals/${id}/decision`, {
        method: "POST",
        ...jsonBody({ decision, comment: "" }),
      }),
    onSuccess: () => client.invalidateQueries({ queryKey: ["approvals"] }),
  });
  return (
    <Box sx={{ p: { xs: 2.5, sm: 4, lg: 5 }, maxWidth: 1100, mx: "auto" }}>
      <Typography variant="h1">검토 및 승인</Typography>
      <Typography color="text.secondary" mt={0.7} mb={4}>
        관리자가 승인 프로세스를 켠 경우에만 요청이 표시됩니다.
      </Typography>
      {error && (
        <Alert severity="info" sx={{ mb: 3 }}>
          {errorMessage(error)}
        </Alert>
      )}
      {items.length ? (
        <Stack gap={1.5}>
          {items.map((item) => (
            <Card key={item.id} sx={{ p: 2.25 }}>
              <Stack
                direction={{ xs: "column", sm: "row" }}
                alignItems={{ sm: "center" }}
                justifyContent="space-between"
                gap={2}
              >
                <Box
                  onClick={() => navigate(`/docs/${item.documentId}`)}
                  sx={{ cursor: "pointer" }}
                >
                  <Stack direction="row" gap={1} alignItems="center">
                    <Typography fontWeight={720} fontSize={17}>
                      {item.documentTitle}
                    </Typography>
                    <Chip size="small" label={`v${item.revision}`} />
                  </Stack>
                  <Typography variant="body2" color="text.secondary" mt={0.6}>
                    {item.requester.displayName} 요청 ·{" "}
                    {formatDate(item.createdAt)} · 승인 {item.approvedCount}/
                    {item.requiredApprovals}
                  </Typography>
                </Box>
                <Stack direction="row" gap={1}>
                  <Button
                    color="error"
                    variant="outlined"
                    startIcon={<Close />}
                    onClick={() =>
                      decide.mutate({ id: item.id, decision: "REJECTED" })
                    }
                  >
                    반려
                  </Button>
                  <Button
                    color="success"
                    variant="contained"
                    startIcon={<Check />}
                    onClick={() =>
                      decide.mutate({ id: item.id, decision: "APPROVED" })
                    }
                  >
                    승인
                  </Button>
                </Stack>
              </Stack>
            </Card>
          ))}
        </Stack>
      ) : (
        <EmptyState
          icon={ApprovalOutlined}
          title="대기 중인 검토가 없습니다"
          description="새 승인 요청이 제출되면 이곳에 나타납니다. 승인 기능이 꺼진 환경에서는 프로세스가 완전히 제외됩니다."
        />
      )}
    </Box>
  );
}
