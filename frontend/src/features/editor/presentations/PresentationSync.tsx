import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Button,
  Chip,
  CircularProgress,
  Collapse,
  Paper,
  Stack,
  Typography,
} from "@mui/material";
import { SyncAlt } from "@mui/icons-material";
import { api, errorMessage } from "../../../lib/api";

type SlideAction = "keep" | "revise" | "add" | "remove";

type SlideImpact = {
  position?: number;
  title: string;
  action: SlideAction;
  section?: string;
  reason?: string;
};

type SyncPlan = {
  fromRevision: number;
  toRevision: number;
  impacts: SlideImpact[];
  revise: number;
  add: number;
  remove: number;
  keep: number;
};

const actionLabel: Record<SlideAction, string> = {
  keep: "유지",
  revise: "다시 작성",
  add: "추가 필요",
  remove: "삭제 후보",
};

const actionColor: Record<SlideAction, "default" | "warning" | "success" | "error"> = {
  keep: "default",
  revise: "warning",
  add: "success",
  remove: "error",
};

/**
 * PresentationSync shows what a document change means for a deck before
 * anything moves. Rebuilding the whole deck would discard the work someone did
 * in the presentation editor, so only the slides whose source material changed
 * are redrafted, and the plan says exactly which those are.
 */
export function PresentationSync({
  documentId,
  linkId,
  canEdit,
  onDone,
}: {
  documentId: string;
  linkId: string;
  canEdit: boolean;
  onDone: () => void;
}) {
  const client = useQueryClient();
  const [open, setOpen] = useState(false);

  const plan = useQuery({
    queryKey: ["presentation-sync", documentId, linkId],
    queryFn: () =>
      api<SyncPlan>(
        `/api/v1/documents/${documentId}/presentations/${linkId}/sync`,
      ),
    enabled: open,
  });

  const apply = useMutation({
    mutationFn: () =>
      api<{ revised: number; applied: boolean }>(
        `/api/v1/documents/${documentId}/presentations/${linkId}/sync`,
        { method: "POST" },
      ),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: ["presentations", documentId] });
      void client.invalidateQueries({
        queryKey: ["presentation-sync", documentId, linkId],
      });
      onDone();
    },
  });

  if (!open)
    return (
      <Button size="small" startIcon={<SyncAlt />} onClick={() => setOpen(true)}>
        변경 반영
      </Button>
    );

  return (
    <Collapse in>
      <Paper variant="outlined" sx={{ p: 1.5, mt: 1 }}>
        {plan.isPending && (
          <Stack alignItems="center" py={1}>
            <CircularProgress size={18} />
          </Stack>
        )}
        {plan.error && <Alert severity="error">{errorMessage(plan.error)}</Alert>}
        {plan.data && (
          <Stack gap={1}>
            <Typography variant="body2" fontWeight={700}>
              Revision {plan.data.fromRevision} → {plan.data.toRevision}
            </Typography>
            {plan.data.revise + plan.data.add + plan.data.remove === 0 ? (
              <Typography variant="body2" color="text.secondary">
                문서 변경이 이 발표자료에 영향을 주지 않습니다.
              </Typography>
            ) : (
              <>
                <Typography variant="caption" color="text.secondary">
                  다시 작성 {plan.data.revise} · 추가 필요 {plan.data.add} · 삭제 후보{" "}
                  {plan.data.remove} · 유지 {plan.data.keep}
                </Typography>
                <Stack gap={0.5}>
                  {plan.data.impacts
                    .filter((impact) => impact.action !== "keep")
                    .map((impact, index) => (
                      <Stack
                        key={`${impact.position ?? "new"}-${index}`}
                        direction="row"
                        gap={0.75}
                        alignItems="flex-start"
                      >
                        <Chip
                          size="small"
                          color={actionColor[impact.action]}
                          label={actionLabel[impact.action]}
                        />
                        <div>
                          <Typography variant="body2">
                            {impact.position ? `${impact.position}. ` : ""}
                            {impact.title}
                          </Typography>
                          {impact.reason && (
                            <Typography
                              variant="caption"
                              color="text.secondary"
                              sx={{ whiteSpace: "pre-wrap" }}
                            >
                              {impact.reason}
                            </Typography>
                          )}
                        </div>
                      </Stack>
                    ))}
                </Stack>
                <Typography variant="caption" color="text.secondary">
                  유지로 표시된 슬라이드는 손대지 않으므로 Ptium에서 편집한 내용이
                  그대로 남습니다.
                </Typography>
              </>
            )}
            {apply.error && (
              <Alert severity="error">{errorMessage(apply.error)}</Alert>
            )}
            <Stack direction="row" gap={1}>
              {canEdit && plan.data.revise > 0 && (
                <Button
                  size="small"
                  variant="contained"
                  disabled={apply.isPending}
                  onClick={() => apply.mutate()}
                >
                  {apply.isPending
                    ? "반영 중…"
                    : `${plan.data.revise}장 다시 작성`}
                </Button>
              )}
              <Button size="small" color="inherit" onClick={() => setOpen(false)}>
                닫기
              </Button>
            </Stack>
          </Stack>
        )}
      </Paper>
    </Collapse>
  );
}
