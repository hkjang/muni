import { useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Add,
  DeleteOutline,
  DescriptionOutlined,
  FolderSharedOutlined,
  StarOutline,
} from "@mui/icons-material";
import {
  Alert,
  Box,
  Button,
  Grid,
  Skeleton,
  Stack,
  Typography,
} from "@mui/material";
import { useSearchParams } from "react-router-dom";
import { api } from "../lib/api";
import type { DocumentItem } from "../types";
import { DocumentCard } from "../components/DocumentCard";
import { EmptyState } from "../components/EmptyState";
import { NewDocumentDialog } from "../components/NewDocumentDialog";
import { useAuth } from "../contexts/AuthContext";

export function DashboardPage({
  scope = "recent",
}: {
  scope?: "recent" | "favorites" | "shared" | "trash";
}) {
  const { user } = useAuth();
  const [params, setParams] = useSearchParams();
  const [dialog, setDialog] = useState(params.get("new") === "1");
  const client = useQueryClient();
  useEffect(() => {
    if (params.get("new") === "1") setDialog(true);
  }, [params]);
  const {
    data: documents = [],
    error,
    isLoading: loading,
  } = useQuery({
    queryKey: ["user-documents", scope],
    queryFn: () =>
      api<DocumentItem[]>(`/api/v1/documents?scope=${scope}&limit=24`),
  });
  const presentation = {
    recent: {
      title: `${user?.displayName}님, 안녕하세요`,
      description: "최근 작업하던 문서에서 바로 이어가세요.",
      section: "최근 문서",
      emptyTitle: "첫 문서를 만들어 보세요",
      emptyDescription:
        "muni에서 작성한 문서는 자동 저장되고 버전 기록에 남습니다.",
      icon: DescriptionOutlined,
    },
    favorites: {
      title: "즐겨찾기",
      description: "중요하게 표시한 문서를 모았습니다.",
      section: "즐겨찾는 문서",
      emptyTitle: "즐겨찾기가 없습니다",
      emptyDescription:
        "문서 카드의 별 아이콘을 누르면 이곳에서 빠르게 찾을 수 있습니다.",
      icon: StarOutline,
    },
    shared: {
      title: "나에게 공유됨",
      description: "다른 사용자가 직접 공유한 문서를 모았습니다.",
      section: "공유 문서",
      emptyTitle: "공유받은 문서가 없습니다",
      emptyDescription: "문서가 공유되면 이곳에서 바로 확인할 수 있습니다.",
      icon: FolderSharedOutlined,
    },
    trash: {
      title: "휴지통",
      description: "삭제한 문서를 확인하고 복원할 수 있습니다.",
      section: "삭제된 문서",
      emptyTitle: "휴지통이 비어 있습니다",
      emptyDescription: "휴지통으로 이동한 문서가 이곳에 표시됩니다.",
      icon: DeleteOutline,
    },
  }[scope];
  const close = () => {
    setDialog(false);
    if (params.has("new")) {
      params.delete("new");
      setParams(params, { replace: true });
    }
  };
  return (
    <Box sx={{ p: { xs: 2.5, sm: 4, lg: 5 }, maxWidth: 1480, mx: "auto" }}>
      <Stack
        direction={{ xs: "column", sm: "row" }}
        justifyContent="space-between"
        alignItems={{ sm: "center" }}
        gap={2}
        mb={4}
      >
        <Box>
          <Typography variant="h1">{presentation.title}</Typography>
          <Typography color="text.secondary" sx={{ mt: 0.8 }}>
            {presentation.description}
          </Typography>
        </Box>
        {scope !== "trash" && (
          <Button
            variant="contained"
            startIcon={<Add />}
            onClick={() => setDialog(true)}
          >
            새 문서
          </Button>
        )}
      </Stack>
      {error && (
        <Alert severity="error" sx={{ mb: 3 }}>
          문서 목록을 불러오지 못했습니다.
        </Alert>
      )}
      <Typography variant="h3" sx={{ mb: 2.25 }}>
        {presentation.section}
      </Typography>
      {loading ? (
        <Grid container spacing={2}>
          {[1, 2, 3, 4].map((item) => (
            <Grid key={item} size={{ xs: 12, sm: 6, lg: 3 }}>
              <Skeleton variant="rounded" height={190} />
            </Grid>
          ))}
        </Grid>
      ) : documents.length ? (
        <Grid container spacing={2}>
          {documents.map((document) => (
            <Grid key={document.id} size={{ xs: 12, sm: 6, lg: 3 }}>
              <DocumentCard
                document={document}
                onFavorite={() =>
                  client.invalidateQueries({ queryKey: ["user-documents"] })
                }
                onRestore={() =>
                  client.invalidateQueries({ queryKey: ["user-documents"] })
                }
              />
            </Grid>
          ))}
        </Grid>
      ) : (
        <EmptyState
          icon={presentation.icon}
          title={presentation.emptyTitle}
          description={presentation.emptyDescription}
          action={scope === "recent" ? "새 문서" : undefined}
          onAction={() => setDialog(true)}
        />
      )}
      <NewDocumentDialog open={dialog} onClose={close} />
    </Box>
  );
}
