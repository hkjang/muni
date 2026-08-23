import { useQuery } from "@tanstack/react-query";
import { Box, Card, Chip, Stack, Typography } from "@mui/material";
import { api, formatDate } from "../../lib/api";
type Audit = {
  id: number;
  actorName?: string;
  action: string;
  resourceType: string;
  resourceId?: string;
  ip?: string;
  metadata: unknown;
  createdAt: string;
};
export function AdminAuditPage() {
  const query = useQuery({
    queryKey: ["audit"],
    queryFn: () => api<Audit[]>("/api/v1/admin/audit?limit=100"),
    refetchInterval: 30000,
  });
  return (
    <Box sx={{ p: { xs: 2.5, sm: 4, lg: 5 }, maxWidth: 1200, mx: "auto" }}>
      <Typography variant="h1">감사 로그</Typography>
      <Typography color="text.secondary" mt={0.7} mb={3}>
        로그인, 문서 접근, 공유, AI, MCP와 관리자 동작을 추적합니다.
      </Typography>
      <Stack gap={1}>
        {(query.data ?? []).map((item) => (
          <Card key={item.id} sx={{ p: 2 }}>
            <Stack
              direction={{ xs: "column", sm: "row" }}
              alignItems={{ sm: "center" }}
              gap={1.5}
            >
              <Chip
                size="small"
                color={
                  item.action.startsWith("AI_")
                    ? "secondary"
                    : item.action.startsWith("UPDATE_")
                      ? "primary"
                      : "default"
                }
                label={item.action}
              />
              <Box sx={{ flex: 1 }}>
                <Typography fontWeight={650}>
                  {item.actorName ?? "시스템"} · {item.resourceType}
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  {item.resourceId ?? "—"} · {item.ip ?? "—"}
                </Typography>
              </Box>
              <Typography variant="body2" color="text.secondary">
                {formatDate(item.createdAt)}
              </Typography>
            </Stack>
          </Card>
        ))}
      </Stack>
    </Box>
  );
}
