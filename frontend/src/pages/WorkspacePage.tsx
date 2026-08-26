import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Add,
  CreateNewFolderOutlined,
  MoreVert,
  FolderOpenOutlined,
  FolderOutlined,
  PeopleOutline,
} from "@mui/icons-material";
import {
  Alert,
  Avatar,
  Box,
  Button,
  Card,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControl,
  Grid,
  IconButton,
  Menu,
  InputLabel,
  List,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  MenuItem,
  Paper,
  Select,
  Skeleton,
  Stack,
  TextField,
  Typography,
} from "@mui/material";
import { useParams, useSearchParams } from "react-router-dom";
import { api, errorMessage, jsonBody } from "../lib/api";
import type { DocumentItem, Folder, Workspace } from "../types";
import { folderPaths } from "../features/editor/folderTree";
import { DocumentCard } from "../components/DocumentCard";
import { EmptyState } from "../components/EmptyState";
import { NewDocumentDialog } from "../components/NewDocumentDialog";

type WorkspaceMember = {
  id: string;
  username: string;
  email: string;
  displayName: string;
  avatarUrl?: string;
  role: "OWNER" | "MANAGER" | "MEMBER" | "VIEWER";
};
type UserSearch = {
  id: string;
  username: string;
  email: string;
  displayName: string;
};

export function WorkspacePage() {
  const { workspaceId = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const folderId = params.get("folder") ?? "";
  const [dialog, setDialog] = useState(false);
  const [folderDialog, setFolderDialog] = useState(false);
  const [folderName, setFolderName] = useState("");
  // Folders could be created and listed and nothing else, so one named by
  // mistake stayed that way.
  const [folderMenu, setFolderMenu] = useState<{
    anchor: HTMLElement;
    folder: Folder;
  } | null>(null);
  const [renaming, setRenaming] = useState<Folder | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const [membersOpen, setMembersOpen] = useState(false);
  const [userQuery, setUserQuery] = useState("");
  const [selectedUser, setSelectedUser] = useState<UserSearch | null>(null);
  const [memberRole, setMemberRole] = useState("MEMBER");
  const client = useQueryClient();
  const { data: workspace } = useQuery({
    queryKey: ["workspace", workspaceId],
    queryFn: () => api<Workspace>(`/api/v1/workspaces/${workspaceId}`),
    enabled: Boolean(workspaceId),
  });
  const { data: documents = [], isLoading } = useQuery({
    queryKey: ["documents", workspaceId, folderId],
    queryFn: () =>
      api<DocumentItem[]>(
        `/api/v1/workspaces/${workspaceId}/documents${folderId ? `?folderId=${folderId}` : ""}`,
      ),
    enabled: Boolean(workspaceId),
  });
  const { data: folders = [] } = useQuery({
    queryKey: ["folders", workspaceId],
    queryFn: () => api<Folder[]>(`/api/v1/workspaces/${workspaceId}/folders`),
    enabled: Boolean(workspaceId),
  });
  const renameFolder = useMutation({
    mutationFn: () =>
      api(`/api/v1/folders/${renaming?.id}`, {
        method: "PATCH",
        ...jsonBody({ name: renameValue }),
      }),
    onSuccess: () => {
      setRenaming(null);
      void client.invalidateQueries({ queryKey: ["folders", workspaceId] });
    },
  });
  const removeFolder = useMutation({
    mutationFn: (id: string) =>
      api<{ documentsMoved: number }>(`/api/v1/folders/${id}`, {
        method: "DELETE",
      }),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: ["folders", workspaceId] });
      void client.invalidateQueries({ queryKey: ["documents", workspaceId] });
    },
  });
  const createFolder = useMutation({
    mutationFn: () =>
      api(`/api/v1/workspaces/${workspaceId}/folders`, {
        method: "POST",
        ...jsonBody({ name: folderName, parentId: folderId || null }),
      }),
    onSuccess: () => {
      setFolderDialog(false);
      setFolderName("");
      void client.invalidateQueries({ queryKey: ["folders", workspaceId] });
    },
  });
  const members = useQuery({
    queryKey: ["workspace-members", workspaceId],
    queryFn: () =>
      api<WorkspaceMember[]>(`/api/v1/workspaces/${workspaceId}/members`),
    enabled: membersOpen,
  });
  const users = useQuery({
    queryKey: ["user-search", userQuery],
    queryFn: () =>
      api<UserSearch[]>(
        `/api/v1/users/search?q=${encodeURIComponent(userQuery)}`,
      ),
    enabled: membersOpen && userQuery.length >= 2 && !selectedUser,
  });
  const addMember = useMutation({
    mutationFn: () =>
      api(`/api/v1/workspaces/${workspaceId}/members`, {
        method: "PUT",
        ...jsonBody({ userId: selectedUser?.id, role: memberRole }),
      }),
    onSuccess: () => {
      setSelectedUser(null);
      setUserQuery("");
      void client.invalidateQueries({
        queryKey: ["workspace-members", workspaceId],
      });
      void client.invalidateQueries({ queryKey: ["workspaces"] });
    },
  });
  const removeMember = useMutation({
    mutationFn: (userId: string) =>
      api(`/api/v1/workspaces/${workspaceId}/members/${userId}`, {
        method: "DELETE",
      }),
    onSuccess: () =>
      client.invalidateQueries({
        queryKey: ["workspace-members", workspaceId],
      }),
  });
  const selectFolder = (id: string) =>
    setParams(id ? { folder: id } : {}, { replace: true });
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
          <Typography variant="h1">
            {workspace?.name ?? "워크스페이스"}
          </Typography>
          <Typography color="text.secondary" mt={0.7}>
            {workspace?.description ||
              `${workspace?.kind === "PERSONAL" ? "나만의" : "팀"} 문서 공간`}
          </Typography>
        </Box>
        {workspace?.role !== "VIEWER" && (
          <Stack direction="row" gap={1}>
            {(workspace?.role === "OWNER" || workspace?.role === "MANAGER") && (
              <Button
                variant="outlined"
                startIcon={<PeopleOutline />}
                onClick={() => setMembersOpen(true)}
              >
                구성원
              </Button>
            )}
            <Button
              variant="outlined"
              startIcon={<CreateNewFolderOutlined />}
              onClick={() => setFolderDialog(true)}
            >
              새 폴더
            </Button>
            <Button
              variant="contained"
              startIcon={<Add />}
              onClick={() => setDialog(true)}
            >
              새 문서
            </Button>
          </Stack>
        )}
      </Stack>
      <Menu
        anchorEl={folderMenu?.anchor ?? null}
        open={Boolean(folderMenu)}
        onClose={() => setFolderMenu(null)}
      >
        <MenuItem
          onClick={() => {
            setRenaming(folderMenu!.folder);
            setRenameValue(folderMenu!.folder.name);
            setFolderMenu(null);
          }}
        >
          이름 바꾸기
        </MenuItem>
        <MenuItem
          onClick={() => {
            const folder = folderMenu!.folder;
            setFolderMenu(null);
            if (
              window.confirm(
                `${folder.name} 폴더를 지웁니다. 안에 있던 문서와 하위 폴더는 지워지지 않고 상위로 올라갑니다.`,
              )
            )
              removeFolder.mutate(folder.id);
          }}
        >
          폴더 삭제
        </MenuItem>
      </Menu>
      <Dialog
        open={Boolean(renaming)}
        onClose={() => setRenaming(null)}
        fullWidth
        maxWidth="xs"
      >
        <DialogTitle>폴더 이름 바꾸기</DialogTitle>
        <DialogContent>
          <TextField
            autoFocus
            fullWidth
            value={renameValue}
            inputProps={{ maxLength: 120 }}
            onChange={(event) => setRenameValue(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter" && renameValue.trim())
                renameFolder.mutate();
            }}
            sx={{ mt: 1 }}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setRenaming(null)}>취소</Button>
          <Button
            variant="contained"
            disabled={!renameValue.trim() || renameFolder.isPending}
            onClick={() => renameFolder.mutate()}
          >
            저장
          </Button>
        </DialogActions>
      </Dialog>
      <Grid container spacing={3}>
        <Grid size={{ xs: 12, md: 3 }}>
          <Card sx={{ p: 1.25 }}>
            <Typography variant="h3" px={1.25} pt={1} pb={0.5}>
              폴더
            </Typography>
            <List dense aria-label="폴더 목록">
              <ListItemButton
                selected={!folderId}
                onClick={() => selectFolder("")}
              >
                <ListItemIcon>
                  <FolderOpenOutlined />
                </ListItemIcon>
                <ListItemText primary="모든 문서" />
              </ListItemButton>
              {folderPaths(folders).map((entry) => {
                const folder = folders.find((item) => item.id === entry.id)!;
                return (
                  <ListItemButton
                    key={folder.id}
                    selected={folderId === folder.id}
                    onClick={() => selectFolder(folder.id)}
                    sx={{ pl: 2 + entry.depth * 2 }}
                  >
                    <ListItemIcon>
                      <FolderOutlined />
                    </ListItemIcon>
                    <ListItemText
                      primary={folder.name}
                      primaryTypographyProps={{ noWrap: true }}
                    />
                    {workspace?.role !== "VIEWER" && (
                      <IconButton
                        size="small"
                        aria-label={`${folder.name} 폴더 메뉴`}
                        onClick={(event) => {
                          event.stopPropagation();
                          setFolderMenu({ anchor: event.currentTarget, folder });
                        }}
                      >
                        <MoreVert fontSize="small" />
                      </IconButton>
                    )}
                  </ListItemButton>
                );
              })}
            </List>
          </Card>
        </Grid>
        <Grid size={{ xs: 12, md: 9 }}>
          {isLoading ? (
            <Grid container spacing={2}>
              {[1, 2, 3, 4].map((item) => (
                <Grid key={item} size={{ xs: 12, sm: 6, xl: 4 }}>
                  <Skeleton height={190} variant="rounded" />
                </Grid>
              ))}
            </Grid>
          ) : documents.length ? (
            <Grid container spacing={2}>
              {documents.map((document) => (
                <Grid key={document.id} size={{ xs: 12, sm: 6, xl: 4 }}>
                  <DocumentCard
                    document={document}
                    onFavorite={() =>
                      client.invalidateQueries({
                        queryKey: ["documents", workspaceId],
                      })
                    }
                  />
                </Grid>
              ))}
            </Grid>
          ) : (
            <EmptyState
              icon={FolderOutlined}
              title="아직 문서가 없습니다"
              description="선택한 위치에 첫 문서를 만들고 팀과 공유해 보세요."
              action={workspace?.role === "VIEWER" ? undefined : "새 문서"}
              onAction={() => setDialog(true)}
            />
          )}
        </Grid>
      </Grid>
      <Dialog open={folderDialog} onClose={() => setFolderDialog(false)}>
        <DialogTitle>새 폴더</DialogTitle>
        <DialogContent sx={{ pt: "8px!important", minWidth: { sm: 420 } }}>
          {createFolder.error && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {errorMessage(createFolder.error)}
            </Alert>
          )}
          <TextField
            autoFocus
            fullWidth
            label="폴더 이름"
            value={folderName}
            onChange={(event) => setFolderName(event.target.value)}
            inputProps={{ maxLength: 120 }}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setFolderDialog(false)}>취소</Button>
          <Button
            variant="contained"
            disabled={!folderName.trim() || createFolder.isPending}
            onClick={() => createFolder.mutate()}
          >
            만들기
          </Button>
        </DialogActions>
      </Dialog>
      <Dialog
        open={membersOpen}
        onClose={() => setMembersOpen(false)}
        fullWidth
        maxWidth="sm"
      >
        <DialogTitle>워크스페이스 구성원</DialogTitle>
        <DialogContent sx={{ pt: "8px!important" }}>
          {(addMember.error || removeMember.error) && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {errorMessage(addMember.error || removeMember.error)}
            </Alert>
          )}
          <Stack direction={{ xs: "column", sm: "row" }} gap={1}>
            <TextField
              fullWidth
              label="사용자 검색"
              value={userQuery}
              onChange={(event) => {
                setUserQuery(event.target.value);
                setSelectedUser(null);
              }}
              placeholder="이름, 이메일 또는 아이디"
            />
            <FormControl size="small" sx={{ minWidth: 130 }}>
              <InputLabel>역할</InputLabel>
              <Select
                label="역할"
                value={memberRole}
                onChange={(event) => setMemberRole(event.target.value)}
              >
                {workspace?.role === "OWNER" && (
                  <MenuItem value="MANAGER">MANAGER</MenuItem>
                )}
                <MenuItem value="MEMBER">MEMBER</MenuItem>
                <MenuItem value="VIEWER">VIEWER</MenuItem>
              </Select>
            </FormControl>
          </Stack>
          {!selectedUser &&
            (users.data ?? []).map((user) => (
              <Paper
                key={user.id}
                variant="outlined"
                onClick={() => {
                  setSelectedUser(user);
                  setUserQuery(user.displayName);
                }}
                sx={{ p: 1.25, mt: 1, cursor: "pointer" }}
              >
                <Typography fontWeight={650}>{user.displayName}</Typography>
                <Typography variant="body2" color="text.secondary">
                  {user.email} · {user.username}
                </Typography>
              </Paper>
            ))}
          <Button
            fullWidth
            variant="contained"
            sx={{ mt: 1.5 }}
            disabled={!selectedUser || addMember.isPending}
            onClick={() => addMember.mutate()}
          >
            구성원 추가 또는 역할 변경
          </Button>
          <Divider sx={{ my: 2 }} />
          <Stack gap={1}>
            {(members.data ?? []).map((member) => (
              <Stack
                key={member.id}
                direction="row"
                alignItems="center"
                justifyContent="space-between"
                gap={1}
              >
                <Stack
                  direction="row"
                  alignItems="center"
                  gap={1.25}
                  minWidth={0}
                >
                  <Avatar src={member.avatarUrl} sx={{ width: 34, height: 34 }}>
                    {member.displayName.slice(0, 1)}
                  </Avatar>
                  <Box minWidth={0}>
                    <Typography fontWeight={650} noWrap>
                      {member.displayName}
                    </Typography>
                    <Typography variant="body2" color="text.secondary" noWrap>
                      {member.email} · {member.role}
                    </Typography>
                  </Box>
                </Stack>
                {member.role !== "OWNER" &&
                  (workspace?.role === "OWNER" ||
                    (workspace?.role === "MANAGER" &&
                      ["MEMBER", "VIEWER"].includes(member.role))) && (
                    <Button
                      size="small"
                      color="error"
                      onClick={() => removeMember.mutate(member.id)}
                    >
                      제거
                    </Button>
                  )}
              </Stack>
            ))}
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setMembersOpen(false)}>닫기</Button>
        </DialogActions>
      </Dialog>
      <NewDocumentDialog
        open={dialog}
        onClose={() => setDialog(false)}
        initialWorkspaceId={workspaceId}
        initialFolderId={folderId}
      />
    </Box>
  );
}
