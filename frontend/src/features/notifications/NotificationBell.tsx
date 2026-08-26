import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import {
  Badge,
  Box,
  Button,
  Divider,
  IconButton,
  Menu,
  Stack,
  Tooltip,
  Typography,
} from "@mui/material";
import {
  ApprovalOutlined,
  CommentOutlined,
  NotificationsNoneOutlined,
  TaskAltOutlined,
} from "@mui/icons-material";
import { api, formatDate } from "../../lib/api";
import { notificationTarget, type NotificationItem } from "./notifications";

/** What each kind of notification is about, at a glance. */
const icons: Record<string, typeof ApprovalOutlined> = {
  APPROVAL_REQUEST: ApprovalOutlined,
  APPROVAL_DECISION: TaskAltOutlined,
  MENTION: CommentOutlined,
};

/**
 * NotificationBell shows what has happened that concerns the reader.
 *
 * These have been recorded since the beginning and, until now, could only be
 * read out of the database — so a review request reached the person who had to
 * act on it only if they happened to open the right document that day.
 */
export function NotificationBell() {
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const navigate = useNavigate();
  const client = useQueryClient();

  const query = useQuery({
    queryKey: ["notifications"],
    queryFn: () =>
      api<{ items: NotificationItem[]; unread: number }>(
        "/api/v1/notifications?limit=30",
      ),
    // Often enough to notice a review request during a meeting, rarely enough
    // that an idle tab is not a stream of requests.
    refetchInterval: 60000,
  });

  const refresh = () => client.invalidateQueries({ queryKey: ["notifications"] });
  const markOne = useMutation({
    mutationFn: (id: string) =>
      api(`/api/v1/notifications/${id}/read`, { method: "POST" }),
    onSuccess: () => void refresh(),
  });
  const markAll = useMutation({
    mutationFn: () => api("/api/v1/notifications/read-all", { method: "POST" }),
    onSuccess: () => void refresh(),
  });

  const items = query.data?.items ?? [];
  const unread = query.data?.unread ?? 0;

  const open = (item: NotificationItem) => {
    if (!item.readAt) markOne.mutate(item.id);
    const target = notificationTarget(item.resourceType, item.resourceId);
    setAnchor(null);
    if (target) navigate(target);
  };

  return (
    <>
      <Tooltip title="알림">
        <IconButton
          aria-label={unread > 0 ? `알림 ${unread}건` : "알림"}
          onClick={(event) => setAnchor(event.currentTarget)}
        >
          <Badge badgeContent={unread} color="error" max={99}>
            <NotificationsNoneOutlined />
          </Badge>
        </IconButton>
      </Tooltip>
      <Menu
        anchorEl={anchor}
        open={Boolean(anchor)}
        onClose={() => setAnchor(null)}
        slotProps={{
          paper: {
            className: "admin-menu-scroll",
            sx: { width: 380, maxHeight: 460, mt: 1 },
          },
        }}
      >
        <Stack direction="row" alignItems="center" px={2} py={1.25} gap={1}>
          <Typography fontWeight={700} sx={{ flex: 1 }}>
            알림
          </Typography>
          {unread > 0 && (
            <Button size="small" onClick={() => markAll.mutate()}>
              모두 읽음
            </Button>
          )}
        </Stack>
        <Divider />
        {items.length === 0 && (
          <Typography color="text.secondary" textAlign="center" py={4}>
            받은 알림이 없습니다.
          </Typography>
        )}
        {items.map((item) => {
          const Icon = icons[item.type] ?? NotificationsNoneOutlined;
          return (
            <Stack
              key={item.id}
              direction="row"
              gap={1.25}
              onClick={() => open(item)}
              sx={{
                px: 2,
                py: 1.25,
                cursor: "pointer",
                alignItems: "flex-start",
                bgcolor: item.readAt ? "transparent" : "action.hover",
                "&:hover": { bgcolor: "action.selected" },
              }}
            >
              <Icon
                fontSize="small"
                color={item.readAt ? "disabled" : "primary"}
                sx={{ mt: 0.25 }}
              />
              <Box sx={{ minWidth: 0, flex: 1 }}>
                <Typography
                  variant="body2"
                  fontWeight={item.readAt ? 400 : 700}
                  noWrap
                >
                  {item.title}
                </Typography>
                <Typography variant="caption" color="text.secondary" display="block">
                  {item.body}
                </Typography>
                <Typography variant="caption" color="text.disabled">
                  {formatDate(item.createdAt)}
                </Typography>
              </Box>
            </Stack>
          );
        })}
      </Menu>
    </>
  );
}
