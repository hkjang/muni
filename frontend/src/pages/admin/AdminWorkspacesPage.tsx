import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Box,
  Card,
  Chip,
  CircularProgress,
  InputAdornment,
  Stack,
  TextField,
  Typography,
} from "@mui/material";
import { SearchOutlined } from "@mui/icons-material";
import { api, formatDate } from "../../lib/api";

type AdminWorkspace = {
  id: string;
  name: string;
  slug: string;
  kind: string;
  ownerName: string;
  members: number;
  documents: number;
  lastEditedAt?: string;
  createdAt: string;
};

const kindLabels: Record<string, string> = {
  PERSONAL: "개인",
  TEAM: "팀",
  DEPARTMENT: "부서",
  ORGANIZATION: "조직",
};

/**
 * AdminWorkspacesPage lists every workspace in the system.
 *
 * An administrator could only see the workspaces they were a member of, which
 * is usually a handful of them — so the questions an operator gets asked, such
 * as which workspaces are dormant or who owns one nobody can get into, had no
 * screen that could answer them.
 */
export function AdminWorkspacesPage() {
  const [query, setQuery] = useState("");
  const list = useQuery({
    queryKey: ["admin-workspaces", query],
    queryFn: () =>
      api<AdminWorkspace[]>(
        `/api/v1/admin/workspaces?limit=100${query.trim() ? `&q=${encodeURIComponent(query.trim())}` : ""}`,
      ),
  });

  return (
    <Box sx={{ p: { xs: 2.5, sm: 4, lg: 5 }, maxWidth: 1200, mx: "auto" }}>
      <Typography variant="h1">워크스페이스</Typography>
      <Typography color="text.secondary" mt={0.7} mb={3}>
        문서 수와 마지막 편집 시각으로 쓰이고 있는 곳과 비어 있는 곳을
        구분합니다.
      </Typography>

      <TextField
        size="small"
        fullWidth
        placeholder="이름 또는 slug로 찾기"
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        sx={{ maxWidth: 380, mb: 3 }}
        InputProps={{
          startAdornment: (
            <InputAdornment position="start">
              <SearchOutlined fontSize="small" />
            </InputAdornment>
          ),
        }}
      />

      {list.isLoading && <CircularProgress />}

      <Stack gap={1}>
        {(list.data ?? []).map((workspace) => (
          <Card key={workspace.id} sx={{ p: 2 }}>
            <Stack
              direction={{ xs: "column", sm: "row" }}
              alignItems={{ sm: "center" }}
              gap={1.5}
            >
              <Chip size="small" label={kindLabels[workspace.kind] ?? workspace.kind} />
              <Box sx={{ flex: 1, minWidth: 0 }}>
                <Typography fontWeight={650} noWrap>
                  {workspace.name}
                  <Typography component="span" variant="body2" color="text.secondary">
                    {" "}
                    /{workspace.slug}
                  </Typography>
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  소유자 {workspace.ownerName} · 구성원 {workspace.members}명 · 문서{" "}
                  {workspace.documents}개
                </Typography>
              </Box>
              <Typography variant="body2" color="text.secondary">
                {workspace.lastEditedAt
                  ? `마지막 편집 ${formatDate(workspace.lastEditedAt)}`
                  : "편집된 문서 없음"}
              </Typography>
            </Stack>
          </Card>
        ))}
        {list.data && list.data.length === 0 && (
          <Typography color="text.secondary" textAlign="center" py={5}>
            찾는 워크스페이스가 없습니다.
          </Typography>
        )}
      </Stack>
    </Box>
  );
}
