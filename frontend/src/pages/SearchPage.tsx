import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import DOMPurify from "dompurify";
import { ArticleOutlined, Search } from "@mui/icons-material";
import {
  Box,
  Button,
  Card,
  CardActionArea,
  Chip,
  InputAdornment,
  MenuItem,
  Skeleton,
  Stack,
  TextField,
  Typography,
} from "@mui/material";
import { useNavigate, useSearchParams } from "react-router-dom";
import { api, formatDate } from "../lib/api";
import { EmptyState } from "../components/EmptyState";
import type { Workspace } from "../types";

type SearchResult = {
  id: string;
  workspaceId: string;
  title: string;
  status: string;
  updatedAt: string;
  ownerName: string;
  snippet: string;
};
type WorkspaceTag = { id: string; name: string; documents: number };

export function SearchPage() {
  const [params, setParams] = useSearchParams();
  const navigate = useNavigate();
  const query = params.get("q") ?? "";
  const workspaceId = params.get("workspaceId") ?? "";
  const owner = params.get("owner") ?? "";
  const tag = params.get("tag") ?? "";
  const from = params.get("from") ?? "";
  const to = params.get("to") ?? "";
  const [value, setValue] = useState(query);

  const filtered = Boolean(workspaceId || owner || tag || from || to);
  const { data: results = [], isLoading } = useQuery({
    queryKey: ["search", query, workspaceId, owner, tag, from, to],
    queryFn: () => api<SearchResult[]>(`/api/v1/search?${params.toString()}`),
    // A filter with no keyword is a search too: "everything I wrote here last
    // month" has no word in it.
    enabled: Boolean(query) || filtered,
  });

  const workspaces = useQuery({
    queryKey: ["workspaces"],
    queryFn: () => api<Workspace[]>("/api/v1/workspaces"),
  });
  const tags = useQuery({
    queryKey: ["workspace-tags", workspaceId],
    queryFn: () => api<WorkspaceTag[]>(`/api/v1/workspaces/${workspaceId}/tags`),
    enabled: Boolean(workspaceId),
  });

  // Every filter lives in the address, so a narrowed search can be sent to
  // somebody or kept in a bookmark.
  const setFilter = (key: string, next: string) => {
    const updated = new URLSearchParams(params);
    if (next) updated.set(key, next);
    else updated.delete(key);
    if (key === "workspaceId") updated.delete("tag");
    setParams(updated);
  };

  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    const updated = new URLSearchParams(params);
    if (value.trim()) updated.set("q", value.trim());
    else updated.delete("q");
    setParams(updated);
  };
  return (
    <Box sx={{ p: { xs: 2.5, sm: 4, lg: 5 }, maxWidth: 1000, mx: "auto" }}>
      <Typography variant="h1" mb={3}>
        통합 검색
      </Typography>
      <Box component="form" onSubmit={submit}>
        <TextField
          fullWidth
          value={value}
          onChange={(event) => setValue(event.target.value)}
          placeholder="제목과 본문에서 검색"
          inputProps={{ "aria-label": "문서 검색" }}
          InputProps={{
            startAdornment: (
              <InputAdornment position="start">
                <Search />
              </InputAdornment>
            ),
          }}
        />
      </Box>
      <Stack direction="row" gap={1.5} mt={2.5} flexWrap="wrap" alignItems="center">
        <TextField
          select
          size="small"
          label="워크스페이스"
          value={workspaceId}
          onChange={(event) => setFilter("workspaceId", event.target.value)}
          sx={{ minWidth: 180 }}
        >
          <MenuItem value="">전체</MenuItem>
          {(workspaces.data ?? []).map((workspace) => (
            <MenuItem key={workspace.id} value={workspace.id}>
              {workspace.name}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          select
          size="small"
          label="작성자"
          value={owner}
          onChange={(event) => setFilter("owner", event.target.value)}
          sx={{ minWidth: 130 }}
        >
          <MenuItem value="">전체</MenuItem>
          <MenuItem value="me">내 문서</MenuItem>
        </TextField>
        {workspaceId && (tags.data ?? []).length > 0 && (
          <TextField
            select
            size="small"
            label="태그"
            value={tag}
            onChange={(event) => setFilter("tag", event.target.value)}
            sx={{ minWidth: 150 }}
          >
            <MenuItem value="">전체</MenuItem>
            {(tags.data ?? []).map((item) => (
              <MenuItem key={item.id} value={item.name}>
                {item.name} ({item.documents})
              </MenuItem>
            ))}
          </TextField>
        )}
        <TextField
          size="small"
          type="date"
          label="수정 시작"
          value={from}
          onChange={(event) => setFilter("from", event.target.value)}
          InputLabelProps={{ shrink: true }}
        />
        <TextField
          size="small"
          type="date"
          label="수정 끝"
          value={to}
          onChange={(event) => setFilter("to", event.target.value)}
          InputLabelProps={{ shrink: true }}
        />
        {filtered && (
          <Button
            color="inherit"
            onClick={() => {
              const cleared = new URLSearchParams();
              if (query) cleared.set("q", query);
              setParams(cleared);
            }}
          >
            조건 지우기
          </Button>
        )}
      </Stack>
      <Stack direction="row" gap={1} alignItems="baseline" sx={{ mt: 3, mb: 2 }}>
        <Typography variant="h3">
          {query
            ? `“${query}” 검색 결과`
            : filtered
              ? "조건에 맞는 문서"
              : "검색어를 입력하세요"}
        </Typography>
        {(query || filtered) && !isLoading && (
          <Chip size="small" label={`${results.length}건`} />
        )}
      </Stack>
      {isLoading ? (
        <Stack gap={1.5}>
          {[1, 2, 3].map((n) => (
            <Skeleton key={n} height={112} variant="rounded" />
          ))}
        </Stack>
      ) : results.length ? (
        <Stack gap={1.5}>
          {results.map((result) => (
            <Card key={result.id}>
              <CardActionArea
                onClick={() => navigate(`/docs/${result.id}`)}
                sx={{ p: 2.25 }}
              >
                <Stack direction="row" gap={2}>
                  <Box
                    sx={{
                      width: 42,
                      height: 42,
                      borderRadius: 2,
                      bgcolor: "#eeeeFA",
                      color: "primary.main",
                      display: "grid",
                      placeItems: "center",
                      flexShrink: 0,
                    }}
                  >
                    <ArticleOutlined />
                  </Box>
                  <Box minWidth={0}>
                    <Typography fontWeight={700}>{result.title}</Typography>
                    <Typography
                      variant="body2"
                      color="text.secondary"
                      sx={{ my: 0.5 }}
                    >
                      {result.ownerName} · {formatDate(result.updatedAt)}
                    </Typography>
                    <Typography
                      variant="body2"
                      noWrap
                      dangerouslySetInnerHTML={{
                        __html: DOMPurify.sanitize(result.snippet, {
                          ALLOWED_TAGS: ["b"],
                          ALLOWED_ATTR: [],
                        }),
                      }}
                    />
                  </Box>
                </Stack>
              </CardActionArea>
            </Card>
          ))}
        </Stack>
      ) : query ? (
        <EmptyState
          icon={Search}
          title="검색 결과가 없습니다"
          description="다른 단어를 사용하거나 띄어쓰기를 바꿔 보세요."
        />
      ) : null}
    </Box>
  );
}
