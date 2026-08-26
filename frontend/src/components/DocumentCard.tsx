import {
  ArticleOutlined,
  RestoreFromTrashOutlined,
  Star,
  StarBorder,
} from "@mui/icons-material";
import {
  Box,
  Card,
  CardActionArea,
  Checkbox,
  Chip,
  IconButton,
  Stack,
  Typography,
} from "@mui/material";
import { useNavigate } from "react-router-dom";
import { api, formatDate } from "../lib/api";
import type { DocumentItem } from "../types";

const statusLabel: Record<DocumentItem["status"], string> = {
  DRAFT: "초안",
  REVIEW: "검토",
  PUBLISHED: "게시",
  ARCHIVED: "보관",
};

export function DocumentCard({
  document,
  onFavorite,
  onRestore,
  selected,
  onSelect,
}: {
  document: DocumentItem;
  onFavorite?: () => void;
  onRestore?: () => void;
  /** Present only where documents can be worked on several at a time. */
  selected?: boolean;
  onSelect?: (selected: boolean) => void;
}) {
  const navigate = useNavigate();
  const toggleFavorite = async (event: React.MouseEvent) => {
    event.stopPropagation();
    await api<void>(`/api/v1/documents/${document.id}/favorite`, {
      method: document.favorite ? "DELETE" : "POST",
    });
    onFavorite?.();
  };
  const restore = async (event: React.MouseEvent) => {
    event.stopPropagation();
    await api(`/api/v1/documents/${document.id}/restore`, { method: "POST" });
    onRestore?.();
  };
  return (
    <Card sx={{ height: "100%", position: "relative" }}>
      <CardActionArea
        onClick={() => !document.deletedAt && navigate(`/docs/${document.id}`)}
        sx={{ height: "100%", p: 2.25, textAlign: "left" }}
      >
        <Stack
          direction="row"
          alignItems="flex-start"
          justifyContent="space-between"
        >
          <Box
            sx={{
              width: 42,
              height: 42,
              bgcolor: "#eeeeFA",
              color: "primary.main",
              borderRadius: 2,
              display: "grid",
              placeItems: "center",
              mb: 2,
            }}
          >
            <ArticleOutlined />
          </Box>
          {onSelect && (
            <Checkbox
              size="small"
              checked={Boolean(selected)}
              inputProps={{ "aria-label": `${document.title} 선택` }}
              onClick={(event) => event.stopPropagation()}
              onChange={(event) => onSelect(event.target.checked)}
              sx={{ mt: -1, ml: -1 }}
            />
          )}
          {document.deletedAt ? (
            <IconButton
              aria-label="문서 복원"
              onClick={restore}
              size="small"
              sx={{ mt: -0.75, mr: -0.75 }}
            >
              <RestoreFromTrashOutlined color="primary" />
            </IconButton>
          ) : (
            <IconButton
              aria-label={document.favorite ? "즐겨찾기 해제" : "즐겨찾기"}
              onClick={toggleFavorite}
              size="small"
              sx={{ mt: -0.75, mr: -0.75 }}
            >
              {document.favorite ? <Star color="warning" /> : <StarBorder />}
            </IconButton>
          )}
        </Stack>
        <Typography sx={{ fontWeight: 680, fontSize: 16, mb: 0.8 }} noWrap>
          {document.title}
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {document.ownerName} · {formatDate(document.updatedAt)}
        </Typography>
        {(document.tags ?? []).length > 0 && (
          <Stack direction="row" gap={0.5} mt={1} flexWrap="wrap">
            {(document.tags ?? []).slice(0, 3).map((tag) => (
              <Chip key={tag} size="small" label={tag} />
            ))}
            {(document.tags ?? []).length > 3 && (
              <Chip
                size="small"
                variant="outlined"
                label={`+${(document.tags ?? []).length - 3}`}
              />
            )}
          </Stack>
        )}
        <Stack direction="row" gap={0.75} mt={2}>
          <Chip
            label={document.deletedAt ? "휴지통" : statusLabel[document.status]}
            size="small"
            variant="outlined"
          />
          <Chip
            label={document.permission}
            size="small"
            sx={{ bgcolor: "#f6f7fb" }}
          />
        </Stack>
      </CardActionArea>
    </Card>
  );
}
