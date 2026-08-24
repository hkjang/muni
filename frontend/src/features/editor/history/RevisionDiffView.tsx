import {
  Alert,
  Box,
  Chip,
  CircularProgress,
  Paper,
  Stack,
  Typography,
} from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { api, errorMessage, formatDate } from "../../../lib/api";
import {
  blockStatusLabel,
  blockTypeLabel,
  describeChanges,
  hasChange,
  type BlockDiff,
  type BlockStatus,
  type RevisionDiff,
} from "./revisionDiff";

const statusColor: Record<BlockStatus, "default" | "success" | "error" | "warning" | "info"> = {
  unchanged: "default",
  added: "success",
  removed: "error",
  changed: "warning",
  moved: "info",
};

export function RevisionDiffView({
  documentId,
  from,
  to,
}: {
  documentId: string;
  from: number;
  to: number;
}) {
  const query = useQuery({
    queryKey: ["revision-diff", documentId, from, to],
    queryFn: () =>
      api<RevisionDiff>(
        `/api/v1/documents/${documentId}/revisions/${from}/diff/${to}`,
      ),
  });

  if (query.isPending)
    return (
      <Stack alignItems="center" py={4}>
        <CircularProgress size={22} />
      </Stack>
    );
  if (query.error)
    return <Alert severity="error">{errorMessage(query.error)}</Alert>;

  const diff = query.data;
  const changed = diff.blocks.filter(hasChange);

  return (
    <Stack gap={1.25}>
      <Paper variant="outlined" sx={{ p: 1.5 }}>
        <Typography variant="body2" fontWeight={700}>
          Revision {diff.from.revision} → {diff.to.revision}
        </Typography>
        <Typography variant="caption" color="text.secondary" display="block">
          {diff.to.author.displayName} · {formatDate(diff.to.createdAt)}
        </Typography>
        <Typography variant="body2" sx={{ mt: 0.75 }}>
          {describeChanges(diff.summary)}
        </Typography>
      </Paper>

      {changed.length === 0 && (
        <Typography color="text.secondary" textAlign="center" py={3}>
          두 버전의 내용이 같습니다.
        </Typography>
      )}

      {changed.map((block, index) => (
        <BlockDiffCard key={`${block.blockId ?? block.type}-${index}`} block={block} />
      ))}

      {changed.length > 0 && diff.summary.unchanged > 0 && (
        <Typography variant="caption" color="text.secondary" textAlign="center">
          변경되지 않은 블록 {diff.summary.unchanged}개는 숨겼습니다.
        </Typography>
      )}
    </Stack>
  );
}

function BlockDiffCard({ block }: { block: BlockDiff }) {
  return (
    <Paper variant="outlined" sx={{ p: 1.5 }}>
      <Stack direction="row" gap={0.75} alignItems="center" mb={0.75}>
        <Chip
          size="small"
          color={statusColor[block.status]}
          label={blockStatusLabel(block.status)}
        />
        <Typography variant="caption" color="text.secondary">
          {blockTypeLabel(block.type)}
        </Typography>
      </Stack>
      <Box sx={{ fontSize: 14, lineHeight: 1.7, whiteSpace: "pre-wrap", wordBreak: "break-word" }}>
        {renderBody(block)}
      </Box>
    </Paper>
  );
}

function renderBody(block: BlockDiff) {
  if (block.status === "changed" && block.inline?.length)
    return block.inline.map((op, index) => {
      if (op.op === "equal") return <span key={index}>{op.text}</span>;
      const inserted = op.op === "insert";
      return (
        <Box
          key={index}
          component="span"
          sx={{
            bgcolor: inserted ? "success.light" : "error.light",
            color: inserted ? "success.contrastText" : "error.contrastText",
            textDecoration: inserted ? "none" : "line-through",
            borderRadius: 0.5,
            px: 0.25,
          }}
        >
          {op.text}
        </Box>
      );
    });

  if (block.status === "added") return block.after;
  if (block.status === "removed")
    return (
      <Box component="span" sx={{ textDecoration: "line-through", opacity: 0.75 }}>
        {block.before}
      </Box>
    );
  // A moved block kept its text; showing where it went is the useful part.
  return (
    <>
      {block.after}
      <Typography variant="caption" color="text.secondary" display="block" mt={0.5}>
        {block.fromIndex + 1}번째 → {block.toIndex + 1}번째
      </Typography>
    </>
  );
}
