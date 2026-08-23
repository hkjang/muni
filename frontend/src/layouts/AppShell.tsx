import { useMemo, useState } from "react";
import {
  Add,
  AdminPanelSettingsOutlined,
  ApprovalOutlined,
  DescriptionOutlined,
  DeleteOutline,
  FolderSharedOutlined,
  HomeOutlined,
  Logout,
  Menu as MenuIcon,
  PersonOutline,
  Search,
  SettingsOutlined,
  StarOutline,
} from "@mui/icons-material";
import {
  AppBar,
  Alert,
  Avatar,
  Box,
  Button,
  Divider,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Drawer,
  IconButton,
  InputBase,
  List,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Menu,
  MenuItem,
  Stack,
  TextField,
  Toolbar,
  Tooltip,
  Typography,
  useMediaQuery,
  useTheme,
} from "@mui/material";
import { Outlet, useLocation, useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, errorMessage, jsonBody } from "../lib/api";
import { useAuth } from "../contexts/AuthContext";
import type { Workspace } from "../types";
import { Brand } from "../components/Brand";

const drawerWidth = 264;

export function AppShell() {
  const theme = useTheme();
  const desktop = useMediaQuery(theme.breakpoints.up("md"));
  const [mobileOpen, setMobileOpen] = useState(false);
  const [profileAnchor, setProfileAnchor] = useState<HTMLElement | null>(null);
  const [search, setSearch] = useState("");
  const [workspaceOpen, setWorkspaceOpen] = useState(false);
  const [workspaceName, setWorkspaceName] = useState("");
  const [workspaceSlug, setWorkspaceSlug] = useState("");
  const [workspaceDescription, setWorkspaceDescription] = useState("");
  const { user, build, logout } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const queryClient = useQueryClient();
  const { data: workspaces = [] } = useQuery({
    queryKey: ["workspaces"],
    queryFn: () => api<Workspace[]>("/api/v1/workspaces"),
  });
  const { data: capabilities } = useQuery({
    queryKey: ["capabilities"],
    queryFn: () =>
      api<{ workflowEnabled: boolean }>("/api/v1/system/capabilities"),
  });
  const createWorkspace = useMutation({
    mutationFn: () =>
      api<Workspace>("/api/v1/workspaces", {
        method: "POST",
        ...jsonBody({
          name: workspaceName,
          slug: workspaceSlug,
          description: workspaceDescription,
        }),
      }),
    onSuccess: (workspace) => {
      setWorkspaceOpen(false);
      setWorkspaceName("");
      setWorkspaceSlug("");
      setWorkspaceDescription("");
      void queryClient.invalidateQueries({ queryKey: ["workspaces"] });
      navigate(`/workspace/${workspace.id}`);
    },
  });

  const active = (path: string) =>
    location.pathname === path ||
    (path !== "/" && location.pathname.startsWith(path + "/"));
  const nav = (path: string) => {
    navigate(path);
    setMobileOpen(false);
  };
  const submitSearch = (event: React.FormEvent) => {
    event.preventDefault();
    if (search.trim())
      navigate(`/search?q=${encodeURIComponent(search.trim())}`);
  };
  const initials = useMemo(
    () => user?.displayName.slice(0, 1).toUpperCase() ?? "M",
    [user],
  );

  const drawer = (
    <Box
      sx={{
        height: "100%",
        display: "flex",
        flexDirection: "column",
        bgcolor: "#fbfbfd",
      }}
    >
      <Box sx={{ height: 70, display: "flex", alignItems: "center", px: 2.5 }}>
        <Brand />
      </Box>
      <Box px={1.25} pb={1}>
        <Button
          fullWidth
          variant="contained"
          startIcon={<Add />}
          onClick={() => nav("/?new=1")}
        >
          새 문서
        </Button>
      </Box>
      <Box sx={{ overflowY: "auto", flex: 1, pb: 2 }}>
        <List aria-label="주 메뉴" disablePadding>
          <ListItemButton selected={active("/")} onClick={() => nav("/")}>
            <ListItemIcon>
              <HomeOutlined />
            </ListItemIcon>
            <ListItemText primary="홈" />
          </ListItemButton>
          <ListItemButton
            selected={active("/search")}
            onClick={() => nav("/search")}
          >
            <ListItemIcon>
              <Search />
            </ListItemIcon>
            <ListItemText primary="검색" />
          </ListItemButton>
          <ListItemButton
            selected={active("/favorites")}
            onClick={() => nav("/favorites")}
          >
            <ListItemIcon>
              <StarOutline />
            </ListItemIcon>
            <ListItemText primary="즐겨찾기" />
          </ListItemButton>
          <ListItemButton
            selected={active("/shared")}
            onClick={() => nav("/shared")}
          >
            <ListItemIcon>
              <FolderSharedOutlined />
            </ListItemIcon>
            <ListItemText primary="나에게 공유됨" />
          </ListItemButton>
          {capabilities?.workflowEnabled && (
            <ListItemButton
              selected={active("/approvals")}
              onClick={() => nav("/approvals")}
            >
              <ListItemIcon>
                <ApprovalOutlined />
              </ListItemIcon>
              <ListItemText primary="검토 및 승인" />
            </ListItemButton>
          )}
          <ListItemButton
            selected={active("/trash")}
            onClick={() => nav("/trash")}
          >
            <ListItemIcon>
              <DeleteOutline />
            </ListItemIcon>
            <ListItemText primary="휴지통" />
          </ListItemButton>
        </List>
        <Divider sx={{ my: 1.5, mx: 2 }} />
        <Stack
          direction="row"
          alignItems="center"
          justifyContent="space-between"
          sx={{ pl: 2.5, pr: 1.25 }}
        >
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ fontWeight: 700, letterSpacing: ".04em" }}
          >
            WORKSPACE
          </Typography>
          <Tooltip title="워크스페이스 만들기">
            <IconButton
              size="small"
              aria-label="워크스페이스 만들기"
              onClick={() => setWorkspaceOpen(true)}
            >
              <Add fontSize="small" />
            </IconButton>
          </Tooltip>
        </Stack>
        <List aria-label="워크스페이스" disablePadding sx={{ mt: 0.5 }}>
          {workspaces.map((workspace) => (
            <ListItemButton
              key={workspace.id}
              selected={active(`/workspace/${workspace.id}`)}
              onClick={() => nav(`/workspace/${workspace.id}`)}
            >
              <ListItemIcon>
                {workspace.kind === "PERSONAL" ? (
                  <DescriptionOutlined />
                ) : (
                  <FolderSharedOutlined />
                )}
              </ListItemIcon>
              <ListItemText
                primary={workspace.name}
                primaryTypographyProps={{ noWrap: true }}
              />
            </ListItemButton>
          ))}
        </List>
      </Box>
      <Divider />
      <List disablePadding sx={{ py: 1 }}>
        <ListItemButton
          selected={active("/settings")}
          onClick={() => nav("/settings")}
        >
          <ListItemIcon>
            <SettingsOutlined />
          </ListItemIcon>
          <ListItemText primary="개인 설정" />
        </ListItemButton>
      </List>
    </Box>
  );

  return (
    <Box
      sx={{ display: "flex", minHeight: "100%", bgcolor: "background.default" }}
    >
      <AppBar
        position="fixed"
        color="inherit"
        elevation={0}
        sx={{
          borderBottom: "1px solid",
          borderColor: "divider",
          ml: { md: `${drawerWidth}px` },
          width: { md: `calc(100% - ${drawerWidth}px)` },
          bgcolor: "rgba(255,255,255,.94)",
          backdropFilter: "blur(12px)",
        }}
      >
        <Toolbar sx={{ minHeight: "69px!important", gap: 2 }}>
          <IconButton
            onClick={() => setMobileOpen(true)}
            sx={{ display: { md: "none" } }}
            aria-label="메뉴 열기"
          >
            <MenuIcon />
          </IconButton>
          <Box
            component="form"
            onSubmit={submitSearch}
            sx={{
              display: "flex",
              alignItems: "center",
              bgcolor: "#f2f3f8",
              borderRadius: 2.5,
              px: 1.5,
              maxWidth: 620,
              flex: 1,
            }}
          >
            <Search color="action" />
            <InputBase
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="문서, 작성자, 태그 검색"
              inputProps={{ "aria-label": "통합 검색" }}
              sx={{ ml: 1, flex: 1, minHeight: 42, fontSize: 15 }}
            />
          </Box>
          <Tooltip title="프로필 메뉴">
            <IconButton
              onClick={(event) => setProfileAnchor(event.currentTarget)}
              aria-label="프로필 메뉴"
            >
              <Avatar
                src={user?.avatarUrl}
                sx={{
                  width: 36,
                  height: 36,
                  bgcolor: "primary.main",
                  fontSize: 16,
                }}
              >
                {initials}
              </Avatar>
            </IconButton>
          </Tooltip>
        </Toolbar>
      </AppBar>
      <Box
        component="nav"
        sx={{ width: { md: drawerWidth }, flexShrink: { md: 0 } }}
      >
        <Drawer
          variant={desktop ? "permanent" : "temporary"}
          open={desktop || mobileOpen}
          onClose={() => setMobileOpen(false)}
          ModalProps={{ keepMounted: true }}
          sx={{
            "& .MuiDrawer-paper": {
              width: drawerWidth,
              boxSizing: "border-box",
              borderRightColor: "divider",
            },
          }}
        >
          {drawer}
        </Drawer>
      </Box>
      <Box component="main" sx={{ flex: 1, minWidth: 0, pt: "69px" }}>
        <Outlet />
      </Box>
      <Menu
        anchorEl={profileAnchor}
        open={Boolean(profileAnchor)}
        onClose={() => setProfileAnchor(null)}
        slotProps={{
          paper: {
            className: "admin-menu-scroll",
            sx: { width: 290, maxHeight: 430, mt: 1 },
          },
        }}
      >
        <Box px={2} py={1.25}>
          <Typography fontWeight={700}>{user?.displayName}</Typography>
          <Typography variant="body2" color="text.secondary" noWrap>
            {user?.email}
          </Typography>
        </Box>
        <Divider />
        <MenuItem
          onClick={() => {
            navigate("/settings");
            setProfileAnchor(null);
          }}
        >
          <ListItemIcon>
            <PersonOutline />
          </ListItemIcon>
          개인 설정
        </MenuItem>
        {user?.role === "ADMIN" && (
          <MenuItem
            onClick={() => {
              navigate("/admin");
              setProfileAnchor(null);
            }}
          >
            <ListItemIcon>
              <AdminPanelSettingsOutlined />
            </ListItemIcon>
            서비스 관리
          </MenuItem>
        )}
        <MenuItem
          onClick={async () => {
            await logout();
            navigate("/login");
          }}
        >
          <ListItemIcon>
            <Logout />
          </ListItemIcon>
          로그아웃
        </MenuItem>
        <Divider />
        <Box px={2} py={1.25}>
          <Typography variant="caption" color="text.secondary">
            muni {build?.version ?? "dev"}
          </Typography>
          <Typography variant="caption" display="block" color="text.secondary">
            commit {build?.commit?.slice(0, 8) ?? "none"}
          </Typography>
        </Box>
      </Menu>
      <Dialog
        open={workspaceOpen}
        onClose={() => setWorkspaceOpen(false)}
        fullWidth
        maxWidth="sm"
      >
        <DialogTitle>팀 워크스페이스 만들기</DialogTitle>
        <DialogContent sx={{ display: "grid", gap: 2, pt: "8px!important" }}>
          {createWorkspace.error && (
            <Alert severity="error">
              {errorMessage(createWorkspace.error)}
            </Alert>
          )}
          <TextField
            autoFocus
            label="워크스페이스 이름"
            value={workspaceName}
            onChange={(event) => setWorkspaceName(event.target.value)}
            inputProps={{ maxLength: 80 }}
          />
          <TextField
            label="Slug"
            placeholder="project-alpha"
            value={workspaceSlug}
            onChange={(event) =>
              setWorkspaceSlug(
                event.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ""),
              )
            }
            helperText="영문 소문자, 숫자, 하이픈 3~48자"
            inputProps={{ maxLength: 48 }}
          />
          <TextField
            label="설명"
            multiline
            minRows={2}
            value={workspaceDescription}
            onChange={(event) => setWorkspaceDescription(event.target.value)}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setWorkspaceOpen(false)}>취소</Button>
          <Button
            variant="contained"
            disabled={
              !workspaceName.trim() ||
              workspaceSlug.length < 3 ||
              createWorkspace.isPending
            }
            onClick={() => createWorkspace.mutate()}
          >
            만들기
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
