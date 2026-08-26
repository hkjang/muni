import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Box,
  Button,
  Card,
  Chip,
  CircularProgress,
  MenuItem,
  Stack,
  TextField,
  Typography,
} from "@mui/material";
import { DownloadOutlined, ExpandMoreOutlined } from "@mui/icons-material";
import { api, formatDate } from "../../lib/api";

type Audit = {
  id: number;
  actorId?: string;
  actorName?: string;
  action: string;
  resourceType: string;
  resourceId?: string;
  ip?: string;
  metadata: unknown;
  createdAt: string;
};

const resourceTypes = [
  { value: "", label: "전체" },
  { value: "DOCUMENT", label: "문서" },
  { value: "USER", label: "사용자" },
  { value: "WORKSPACE", label: "워크스페이스" },
  { value: "SETTINGS", label: "설정" },
  { value: "AI", label: "AI" },
];

type Filters = {
  q: string;
  resourceType: string;
  action: string;
  from: string;
  to: string;
};

const empty: Filters = { q: "", resourceType: "", action: "", from: "", to: "" };

function toQuery(filters: Filters, before?: number): string {
  const params = new URLSearchParams();
  if (filters.q.trim()) params.set("q", filters.q.trim());
  if (filters.resourceType) params.set("resourceType", filters.resourceType);
  if (filters.action.trim()) params.set("action", filters.action.trim());
  if (filters.from) params.set("from", filters.from);
  if (filters.to) params.set("to", filters.to);
  if (before) params.set("before", String(before));
  params.set("limit", "100");
  return params.toString();
}

/**
 * AdminAuditPage reads the activity log.
 *
 * It used to show the last hundred rows and nothing else, which cannot answer
 * the question an audit log exists for: what did this person do, or what
 * happened to this document, between these two dates. Paging is by row id
 * rather than by offset, so a log that is still being written to does not
 * shift rows under someone reading through it.
 */
export function AdminAuditPage() {
  const [filters, setFilters] = useState<Filters>(empty);
  const [pages, setPages] = useState<number[]>([]);

  const before = pages[pages.length - 1];
  const query = useQuery({
    queryKey: ["audit", filters, before ?? 0],
    queryFn: () =>
      api<{ items: Audit[] }>(`/api/v1/admin/audit?${toQuery(filters, before)}`),
  });

  const set = (patch: Partial<Filters>) => {
    setFilters((current) => ({ ...current, ...patch }));
    setPages([]);
  };

  const items = query.data?.items ?? [];
  const filtered = Object.values(filters).some((value) => value !== "");

  return (
    <Box sx={{ p: { xs: 2.5, sm: 4, lg: 5 }, maxWidth: 1200, mx: "auto" }}>
      <Typography variant="h1">감사 로그</Typography>
      <Typography color="text.secondary" mt={0.7} mb={3}>
        로그인, 문서 접근, 공유, AI, MCP와 관리자 동작을 추적합니다.
      </Typography>

      <Stack direction="row" gap={1.5} mb={1} flexWrap="wrap" alignItems="center">
        <TextField
          size="small"
          label="검색"
          placeholder="동작 또는 사용자 이름"
          value={filters.q}
          onChange={(event) => set({ q: event.target.value })}
          sx={{ minWidth: 220 }}
        />
        <TextField
          select
          size="small"
          label="대상"
          value={filters.resourceType}
          onChange={(event) => set({ resourceType: event.target.value })}
          sx={{ minWidth: 140 }}
        >
          {resourceTypes.map((type) => (
            <MenuItem key={type.value} value={type.value}>
              {type.label}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          size="small"
          label="정확한 동작"
          placeholder="UPDATE_SETTINGS"
          value={filters.action}
          onChange={(event) => set({ action: event.target.value })}
          sx={{ minWidth: 190 }}
        />
        <TextField
          size="small"
          type="date"
          label="시작"
          value={filters.from}
          onChange={(event) => set({ from: event.target.value })}
          InputLabelProps={{ shrink: true }}
        />
        <TextField
          size="small"
          type="date"
          label="끝"
          value={filters.to}
          onChange={(event) => set({ to: event.target.value })}
          InputLabelProps={{ shrink: true }}
        />
        {filtered && (
          <Button color="inherit" onClick={() => set(empty)}>
            조건 지우기
          </Button>
        )}
        <Box sx={{ flex: 1 }} />
        <Button
          component="a"
          href={`/api/v1/admin/audit.csv?${toQuery(filters)}`}
          startIcon={<DownloadOutlined />}
          variant="outlined"
        >
          CSV 내려받기
        </Button>
      </Stack>

      {query.isLoading && <CircularProgress sx={{ my: 2 }} />}

      <Stack gap={1} mt={2}>
        {items.map((item) => (
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
                    : item.action.startsWith("UPDATE_") ||
                        item.action.startsWith("EXPORT_")
                      ? "primary"
                      : "default"
                }
                label={item.action}
              />
              <Box sx={{ flex: 1, minWidth: 0 }}>
                <Typography fontWeight={650}>
                  {item.actorName ?? "시스템"} · {item.resourceType}
                </Typography>
                <Typography variant="body2" color="text.secondary" noWrap>
                  {item.resourceId ?? "—"} · {item.ip ?? "—"}
                </Typography>
              </Box>
              <Typography variant="body2" color="text.secondary">
                {formatDate(item.createdAt)}
              </Typography>
            </Stack>
          </Card>
        ))}
        {!query.isLoading && items.length === 0 && (
          <Typography color="text.secondary" textAlign="center" py={5}>
            조건에 맞는 기록이 없습니다.
          </Typography>
        )}
      </Stack>

      <Stack direction="row" gap={1} justifyContent="center" mt={2}>
        {pages.length > 0 && (
          <Button
            color="inherit"
            onClick={() => setPages((current) => current.slice(0, -1))}
          >
            이전
          </Button>
        )}
        {items.length === 100 && (
          <Button
            startIcon={<ExpandMoreOutlined />}
            onClick={() =>
              setPages((current) => [...current, items[items.length - 1]!.id])
            }
          >
            다음 100건
          </Button>
        )}
      </Stack>
    </Box>
  );
}
