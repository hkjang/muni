import {
  AdminPanelSettingsOutlined,
  ArrowBack,
  GroupsOutlined,
  KeyOutlined,
  Menu as MenuIcon,
  PsychologyOutlined,
  PolicyOutlined,
  SettingsOutlined,
} from "@mui/icons-material";
import {
  AppBar,
  Box,
  Drawer,
  IconButton,
  List,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Toolbar,
  Typography,
  useMediaQuery,
  useTheme,
} from "@mui/material";
import { useState } from "react";
import { Navigate, Outlet, useLocation, useNavigate } from "react-router-dom";
import { Brand } from "../components/Brand";
import { useAuth } from "../contexts/AuthContext";

const width = 274;
export function AdminShell() {
  const { user } = useAuth();
  const theme = useTheme();
  const desktop = useMediaQuery(theme.breakpoints.up("md"));
  const [open, setOpen] = useState(false);
  const location = useLocation();
  const navigate = useNavigate();
  if (user?.role !== "ADMIN") return <Navigate to="/" replace />;
  const items = [
    ["/admin", "서비스 설정", SettingsOutlined],
    ["/admin/users", "사용자 관리", GroupsOutlined],
    ["/admin/key-policies", "키 권한 정책", KeyOutlined],
    ["/admin/ai-usage", "AI 호출 감사", PsychologyOutlined],
    ["/admin/audit", "감사 로그", PolicyOutlined],
  ] as const;
  const drawer = (
    <Box
      className="admin-menu-scroll"
      sx={{
        height: "100%",
        display: "flex",
        flexDirection: "column",
        bgcolor: "#292a3a",
        color: "#f5f5fb",
        overflowY: "auto",
      }}
    >
      <Box sx={{ height: 74, display: "flex", alignItems: "center", px: 2.5 }}>
        <Brand inverse />
      </Box>
      <Box sx={{ px: 2.5, pb: 1.5 }}>
        <Typography
          variant="caption"
          sx={{ color: "#bbbdd0", fontWeight: 700, letterSpacing: ".08em" }}
        >
          SERVICE ADMIN
        </Typography>
      </Box>
      <List sx={{ px: 0.75 }}>
        {items.map(([itemPath, label, Icon]) => (
          <ListItemButton
            key={itemPath}
            selected={location.pathname === itemPath}
            onClick={() => {
              navigate(itemPath);
              setOpen(false);
            }}
            sx={{
              color: "#e6e7f2",
              "& .MuiListItemIcon-root": { color: "inherit" },
              "&.Mui-selected": {
                bgcolor: "rgba(143,143,240,.22)",
                color: "#fff",
              },
              "&.Mui-selected:hover": { bgcolor: "rgba(143,143,240,.28)" },
              "&:hover": { bgcolor: "rgba(255,255,255,.07)" },
            }}
          >
            <ListItemIcon>
              <Icon />
            </ListItemIcon>
            <ListItemText primary={label} />
          </ListItemButton>
        ))}
      </List>
      <Box sx={{ flex: 1 }} />
      <List sx={{ px: 0.75, pb: 2 }}>
        <ListItemButton
          onClick={() => navigate("/")}
          sx={{
            color: "#e6e7f2",
            "& .MuiListItemIcon-root": { color: "inherit" },
            "&:hover": { bgcolor: "rgba(255,255,255,.07)" },
          }}
        >
          <ListItemIcon>
            <ArrowBack />
          </ListItemIcon>
          <ListItemText primary="문서로 돌아가기" />
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
        elevation={0}
        color="inherit"
        sx={{
          ml: { md: `${width}px` },
          width: { md: `calc(100% - ${width}px)` },
          borderBottom: "1px solid",
          borderColor: "divider",
        }}
      >
        <Toolbar sx={{ minHeight: "69px!important" }}>
          <IconButton
            onClick={() => setOpen(true)}
            sx={{ display: { md: "none" } }}
          >
            <MenuIcon />
          </IconButton>
          <AdminPanelSettingsOutlined color="primary" sx={{ mr: 1.2 }} />
          <Typography fontWeight={720}>muni 서비스 관리</Typography>
        </Toolbar>
      </AppBar>
      <Box component="nav" sx={{ width: { md: width }, flexShrink: { md: 0 } }}>
        <Drawer
          variant={desktop ? "permanent" : "temporary"}
          open={desktop || open}
          onClose={() => setOpen(false)}
          sx={{
            "& .MuiDrawer-paper": { width, boxSizing: "border-box", border: 0 },
          }}
        >
          {drawer}
        </Drawer>
      </Box>
      <Box component="main" sx={{ flex: 1, minWidth: 0, pt: "69px" }}>
        <Outlet />
      </Box>
    </Box>
  );
}
