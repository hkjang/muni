import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import {
  Box,
  Chip,
  CircularProgress,
  Dialog,
  InputBase,
  List,
  ListItemButton,
  ListSubheader,
  Stack,
  Typography,
} from "@mui/material";
import {
  ArticleOutlined,
  FolderOutlined,
  SearchOutlined,
} from "@mui/icons-material";
import { api } from "../../lib/api";
import { useAuth } from "../../contexts/AuthContext";
import type { DocumentItem, Workspace } from "../../types";
import { groupCommands, rank, type QuickCommand } from "./commands";

type SearchResult = {
  id: string;
  workspaceId: string;
  title: string;
  ownerName: string;
  snippet: string;
};

const staticDestinations: QuickCommand[] = [
  {
    id: "go:home",
    label: "홈",
    group: "이동",
    to: "/",
    keywords: "dashboard home",
  },
  {
    id: "go:favorites",
    label: "즐겨찾기",
    group: "이동",
    to: "/favorites",
    keywords: "favorites star",
  },
  {
    id: "go:shared",
    label: "공유 문서",
    group: "이동",
    to: "/shared",
    keywords: "shared",
  },
  {
    id: "go:approvals",
    label: "승인 대기",
    group: "이동",
    to: "/approvals",
    keywords: "approvals",
  },
  {
    id: "go:trash",
    label: "휴지통",
    group: "이동",
    to: "/trash",
    keywords: "trash",
  },
  {
    id: "go:settings",
    label: "개인 설정",
    group: "이동",
    to: "/settings",
    keywords: "settings profile",
  },
];

/** Only reachable by an administrator, so they are added per user. */
const adminDestinations: QuickCommand[] = [
  {
    id: "go:admin",
    label: "운영 현황",
    group: "이동",
    to: "/admin",
    keywords: "admin 관리자 현황 dashboard overview",
  },
  {
    id: "go:admin-settings",
    label: "서비스 설정",
    group: "이동",
    to: "/admin/settings",
    keywords: "admin service settings 설정",
  },
  {
    id: "go:admin-documents",
    label: "문서 관리",
    group: "이동",
    to: "/admin/documents",
    keywords: "admin documents 문서 소유권 완전삭제",
  },
  {
    id: "go:admin-workspaces",
    label: "워크스페이스 관리",
    group: "이동",
    to: "/admin/workspaces",
    keywords: "admin workspaces 워크스페이스",
  },
  {
    id: "go:admin-users",
    label: "사용자 관리",
    group: "이동",
    to: "/admin/users",
    keywords: "admin users 계정",
  },
  {
    id: "go:admin-keys",
    label: "키 권한 정책",
    group: "이동",
    to: "/admin/key-policies",
    keywords: "admin key policy",
  },
  {
    id: "go:admin-ai",
    label: "AI 호출 감사",
    group: "이동",
    to: "/admin/ai-usage",
    keywords: "admin ai usage token",
  },
  {
    id: "go:admin-audit",
    label: "감사 로그",
    group: "이동",
    to: "/admin/audit",
    keywords: "admin audit log",
  },
];

/**
 * QuickSwitcher jumps to a document, a workspace or a screen without leaving
 * the keyboard.
 *
 * Recent documents and workspaces are already loaded for the shell, so an empty
 * box is useful straight away; typing three characters starts asking the server,
 * which searches document bodies as well as titles.
 */
export function QuickSwitcher({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const navigate = useNavigate();
  const { user } = useAuth();
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const listRef = useRef<HTMLUListElement>(null);

  const recent = useQuery({
    queryKey: ["user-documents", "recent"],
    queryFn: () =>
      api<DocumentItem[]>("/api/v1/documents?scope=recent&limit=24"),
    enabled: open,
  });
  const workspaces = useQuery({
    queryKey: ["workspaces"],
    queryFn: () => api<Workspace[]>("/api/v1/workspaces"),
    enabled: open,
  });

  const trimmed = query.trim();
  // Below three characters the recent list is a better answer than a search,
  // and every keystroke would otherwise be a request.
  const searching = trimmed.length >= 2;
  const search = useQuery({
    queryKey: ["quick-search", trimmed],
    queryFn: () =>
      api<SearchResult[]>(
        `/api/v1/search?q=${encodeURIComponent(trimmed)}&limit=12`,
      ),
    enabled: open && searching,
  });

  const commands = useMemo(() => {
    const seen = new Set<string>();
    const out: QuickCommand[] = [];

    for (const result of search.data ?? []) {
      seen.add(result.id);
      out.push({
        id: `doc:${result.id}`,
        label: result.title || "제목 없는 문서",
        detail: stripTags(result.snippet) || result.ownerName,
        group: "문서",
        to: `/docs/${result.id}`,
      });
    }
    const workspaceNames = new Map(
      (workspaces.data ?? []).map((workspace) => [
        workspace.id,
        workspace.name,
      ]),
    );
    for (const document of recent.data ?? []) {
      if (seen.has(document.id)) continue;
      out.push({
        id: `doc:${document.id}`,
        label: document.title || "제목 없는 문서",
        detail: workspaceNames.get(document.workspaceId) ?? document.ownerName,
        group: "문서",
        to: `/docs/${document.id}`,
      });
    }
    for (const workspace of workspaces.data ?? []) {
      out.push({
        id: `ws:${workspace.id}`,
        label: workspace.name,
        detail: workspace.description,
        keywords: workspace.slug,
        group: "워크스페이스",
        to: `/workspace/${workspace.id}`,
      });
    }
    out.push(...staticDestinations);
    if (user?.role === "ADMIN") out.push(...adminDestinations);
    if (trimmed) {
      out.push({
        id: "action:search",
        label: `'${trimmed}' 전체 검색`,
        group: "동작",
        to: `/search?q=${encodeURIComponent(trimmed)}`,
      });
    }
    return out;
  }, [recent.data, search.data, workspaces.data, trimmed, user?.role]);

  // A search result already matched on the server, so ranking again would only
  // drop rows whose match was in the body rather than the title.
  const matches = useMemo(() => {
    const ranked = rank(commands, searching ? "" : trimmed, 14);
    return searching
      ? ranked.filter(
          (command) =>
            command.group !== "워크스페이스" ||
            rank([command], trimmed, 1).length > 0,
        )
      : ranked;
  }, [commands, trimmed, searching]);

  const grouped = useMemo(() => groupCommands(matches), [matches]);
  const flat = useMemo(
    () => grouped.flatMap((entry) => entry.commands),
    [grouped],
  );

  useEffect(() => setActive(0), [trimmed, matches.length]);
  useEffect(() => {
    if (!open) {
      setQuery("");
      setActive(0);
    }
  }, [open]);

  const choose = (command?: QuickCommand) => {
    if (!command) return;
    onClose();
    if (command.run) return command.run();
    if (command.to) navigate(command.to);
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      fullWidth
      maxWidth="sm"
      slotProps={{
        paper: { sx: { position: "fixed", top: 80, m: 0, borderRadius: 2 } },
      }}
    >
      <Stack direction="row" alignItems="center" gap={1} px={2} py={1.25}>
        <SearchOutlined color="disabled" />
        <InputBase
          autoFocus
          fullWidth
          placeholder="문서, 워크스페이스, 화면으로 이동"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "ArrowDown") {
              event.preventDefault();
              setActive((current) => Math.min(current + 1, flat.length - 1));
            }
            if (event.key === "ArrowUp") {
              event.preventDefault();
              setActive((current) => Math.max(current - 1, 0));
            }
            if (event.key === "Enter") {
              event.preventDefault();
              choose(flat[active]);
            }
            if (event.key === "Escape") onClose();
          }}
          sx={{ fontSize: 17 }}
        />
        {search.isFetching && <CircularProgress size={16} />}
      </Stack>

      <Box
        sx={{
          borderTop: "1px solid",
          borderColor: "divider",
          maxHeight: 420,
          overflowY: "auto",
        }}
      >
        {flat.length === 0 ? (
          <Typography color="text.secondary" textAlign="center" py={4}>
            {searching ? "찾는 항목이 없습니다." : "최근 문서가 없습니다."}
          </Typography>
        ) : (
          <List ref={listRef} dense disablePadding>
            {grouped.map((entry) => (
              <li key={entry.group}>
                <ul style={{ padding: 0 }}>
                  <ListSubheader sx={{ lineHeight: "28px", fontSize: 12 }}>
                    {entry.group}
                  </ListSubheader>
                  {entry.commands.map((command) => {
                    const index = flat.indexOf(command);
                    return (
                      <ListItemButton
                        key={command.id}
                        selected={index === active}
                        onMouseEnter={() => setActive(index)}
                        onClick={() => choose(command)}
                        sx={{ gap: 1.25, py: 0.75 }}
                      >
                        {command.group === "워크스페이스" ? (
                          <FolderOutlined fontSize="small" color="disabled" />
                        ) : command.group === "문서" ? (
                          <ArticleOutlined fontSize="small" color="disabled" />
                        ) : (
                          <SearchOutlined fontSize="small" color="disabled" />
                        )}
                        <Box sx={{ minWidth: 0, flex: 1 }}>
                          <Typography noWrap>{command.label}</Typography>
                          {command.detail && (
                            <Typography
                              variant="caption"
                              color="text.secondary"
                              noWrap
                              display="block"
                            >
                              {command.detail}
                            </Typography>
                          )}
                        </Box>
                        {index === active && (
                          <Chip size="small" label="Enter" />
                        )}
                      </ListItemButton>
                    );
                  })}
                </ul>
              </li>
            ))}
          </List>
        )}
      </Box>
    </Dialog>
  );
}

function stripTags(value: string): string {
  return value.replace(/<[^>]*>/g, "").trim();
}
