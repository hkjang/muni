import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ApprovalOutlined,
  Check,
  CheckCircle,
  Close,
  RadioButtonUnchecked,
  RemoveCircleOutline,
} from "@mui/icons-material";
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
  mode: "ANY" | "SEQUENTIAL";
  /** True when the reader is the person this request is waiting on. */
  myTurn: boolean;
  currentApprover?: { id: string; displayName: string };
  stepCount?: number;
  stepsApproved?: number;
  steps: ApprovalStep[];
};

type ApprovalStep = {
  position: number;
  approver: { id: string; displayName: string };
  status: "PENDING" | "APPROVED" | "REJECTED" | "SKIPPED";
  isFinal: boolean;
  comment: string;
  decidedAt?: string;
};

/** The line, drawn so the reader can see where a document is sitting. */
function ApprovalLine({ steps }: { steps: ApprovalStep[] }) {
  if (steps.length === 0) return null;
  return (
    <Stack direction="row" gap={0.5} flexWrap="wrap" alignItems="center" mt={1}>
      {steps.map((step, index) => {
        const Icon =
          step.status === "APPROVED"
            ? CheckCircle
            : step.status === "REJECTED"
              ? Close
              : step.status === "SKIPPED"
                ? RemoveCircleOutline
                : RadioButtonUnchecked;
        const color =
          step.status === "APPROVED"
            ? "success.main"
            : step.status === "REJECTED"
              ? "error.main"
              : step.status === "SKIPPED"
                ? "text.disabled"
                : "primary.main";
        return (
          <Stack key={step.position} direction="row" gap={0.4} alignItems="center">
            {index > 0 && (
              <Typography color="text.disabled" sx={{ mx: 0.25 }}>
                ›
              </Typography>
            )}
            <Icon sx={{ fontSize: 16, color }} />
            <Typography
              variant="caption"
              sx={{
                color,
                fontWeight: step.status === "PENDING" ? 700 : 400,
                textDecoration: step.status === "SKIPPED" ? "line-through" : "none",
              }}
            >
              {step.approver.displayName}
              {step.isFinal ? " (전결)" : ""}
            </Typography>
          </Stack>
        );
      })}
    </Stack>
  );
}
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
                    {formatDate(item.createdAt)} ·{" "}
                    {item.mode === "SEQUENTIAL"
                      ? `결재 ${item.stepsApproved ?? 0}/${item.stepCount ?? 0}`
                      : `승인 ${item.approvedCount}/${item.requiredApprovals}`}
                    {item.mode === "SEQUENTIAL" && item.currentApprover
                      ? ` · ${item.currentApprover.displayName} 차례`
                      : ""}
                  </Typography>
                  <ApprovalLine steps={item.steps} />
                </Box>
                <Stack direction="row" gap={1} alignItems="center">
                  {item.mode === "SEQUENTIAL" && !item.myTurn && (
                    <Typography variant="body2" color="text.secondary">
                      내 차례가 아닙니다
                    </Typography>
                  )}
                  <Button
                    color="error"
                    disabled={item.mode === "SEQUENTIAL" && !item.myTurn}
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
                    disabled={item.mode === "SEQUENTIAL" && !item.myTurn}
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
