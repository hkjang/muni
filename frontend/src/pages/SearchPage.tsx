import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import DOMPurify from "dompurify";
import { ArticleOutlined, Search } from "@mui/icons-material";
import {
  Box,
  Card,
  CardActionArea,
  InputAdornment,
  Skeleton,
  Stack,
  TextField,
  Typography,
} from "@mui/material";
import { useNavigate, useSearchParams } from "react-router-dom";
import { api, formatDate } from "../lib/api";
import { EmptyState } from "../components/EmptyState";

type SearchResult = {
  id: string;
  workspaceId: string;
  title: string;
  status: string;
  updatedAt: string;
  ownerName: string;
  snippet: string;
};
export function SearchPage() {
  const [params, setParams] = useSearchParams();
  const navigate = useNavigate();
  const query = params.get("q") ?? "";
  const [value, setValue] = useState(query);
  const { data: results = [], isLoading } = useQuery({
    queryKey: ["search", query],
    queryFn: () =>
      api<SearchResult[]>(`/api/v1/search?q=${encodeURIComponent(query)}`),
    enabled: Boolean(query),
  });
  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    setParams(value.trim() ? { q: value.trim() } : {});
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
      <Typography variant="h3" sx={{ mt: 4, mb: 2 }}>
        {query ? `“${query}” 검색 결과` : "검색어를 입력하세요"}
      </Typography>
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
