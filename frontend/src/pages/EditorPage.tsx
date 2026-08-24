import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  EditorContent,
  useEditor,
  useEditorState,
  type Editor,
} from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import { AISelectionMenu } from "../features/editor/ai/AISelectionMenu";
import { AgentPanel } from "../features/editor/ai/AgentPanel";
import { BlockId } from "../features/editor/extensions/blockId";
import { CommentsPanel } from "../features/editor/comments/CommentsPanel";
import { SuggestionsPanel } from "../features/editor/suggestions/SuggestionsPanel";
import { HistoryPanel } from "../features/editor/history/HistoryPanel";
import Collaboration from "@tiptap/extension-collaboration";
import CollaborationCaret from "@tiptap/extension-collaboration-caret";
import Placeholder from "@tiptap/extension-placeholder";
import Highlight from "@tiptap/extension-highlight";
import Image from "@tiptap/extension-image";
import { TableKit } from "@tiptap/extension-table";
import TaskList from "@tiptap/extension-task-list";
import TaskItem from "@tiptap/extension-task-item";
import TextAlign from "@tiptap/extension-text-align";
import { TextStyleKit } from "@tiptap/extension-text-style";
import {
  AddCommentOutlined,
  ArrowBack,
  AutoAwesome,
  CloudDoneOutlined,
  CloudOffOutlined,
  Code,
  CommentOutlined,
  DownloadOutlined,
  DeleteOutline,
  FormatAlignCenter,
  FormatAlignLeft,
  FormatAlignRight,
  FormatBold,
  FormatColorText,
  FormatItalic,
  FormatIndentDecrease,
  FormatIndentIncrease,
  FormatListBulleted,
  FormatListNumbered,
  FormatQuote,
  FormatUnderlined,
  BorderColor,
  History,
  HorizontalRule,
  ImageOutlined,
  InsertLink,
  Menu as MenuIcon,
  PeopleOutline,
  Redo,
  StrikethroughS,
  TableChartOutlined,
  TaskAlt,
  Undo,
} from "@mui/icons-material";
import {
  Alert,
  AppBar,
  Avatar,
  Box,
  Button,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  Drawer,
  FormControl,
  IconButton,
  InputLabel,
  ListItemIcon,
  Menu,
  MenuItem,
  Paper,
  Select,
  Stack,
  Tab,
  Tabs,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Toolbar,
  Tooltip,
  Typography,
  useMediaQuery,
  useTheme,
} from "@mui/material";
import { useNavigate, useParams } from "react-router-dom";
import { Brand } from "../components/Brand";
import { LoadingScreen } from "../components/LoadingScreen";
import { useAuth } from "../contexts/AuthContext";
import { useCollaboration } from "../hooks/useCollaboration";
import { ApiError, api, errorMessage, jsonBody } from "../lib/api";
import type { DocumentItem } from "../types";
import type {
  Capability,
  Permission,
  SideTab,
  UserSearch,
} from "../features/editor/types";

export function EditorPage() {
  const { documentId = "" } = useParams();
  const { user, build, logout } = useAuth();
  const navigate = useNavigate();
  const theme = useTheme();
  const compact = useMediaQuery(theme.breakpoints.down("lg"));
  const queryClient = useQueryClient();
  const [mode, setMode] = useState<"editing" | "suggesting" | "viewing">(
    "editing",
  );
  const [sideOpen, setSideOpen] = useState(!compact);
  const [sideTab, setSideTab] = useState<SideTab>("ai");
  const [saveState, setSaveState] = useState<
    "saved" | "saving" | "offline" | "error"
  >("saved");
  const [menuAnchor, setMenuAnchor] = useState<HTMLElement | null>(null);
  const [exportAnchor, setExportAnchor] = useState<HTMLElement | null>(null);
  const [shareOpen, setShareOpen] = useState(false);
  const [mobileTools, setMobileTools] = useState(false);
  const revisionRef = useRef(0);
  const saveTimer = useRef<number | undefined>(undefined);
  const seeded = useRef(false);
  const documentQuery = useQuery({
    queryKey: ["document", documentId],
    queryFn: () => api<DocumentItem>(`/api/v1/documents/${documentId}`),
    enabled: Boolean(documentId),
    retry: false,
  });
  const capabilities = useQuery({
    queryKey: ["capabilities"],
    queryFn: () => api<Capability>("/api/v1/system/capabilities"),
  });
  const document = documentQuery.data;
  useEffect(() => {
    if (document) revisionRef.current = document.revision;
  }, [document]);
  const collaboration = useCollaboration(documentId, user);
  const extensions = useMemo(
    () => [
      StarterKit.configure({ undoRedo: false }),
      Collaboration.configure({ document: collaboration.ydoc }),
      CollaborationCaret.configure({
        provider: collaboration.provider,
        user: {
          id: user?.id,
          name: user?.displayName ?? "사용자",
          color: "#5151c6",
        },
      }),
      Placeholder.configure({
        placeholder: "내용을 입력하거나 /ai로 AI 도우미를 시작하세요.",
      }),
      Highlight.configure({ multicolor: true }),
      Image.configure({ allowBase64: true, inline: false }),
      TableKit.configure({ table: { resizable: true } }),
      TaskList,
      TaskItem.configure({ nested: true }),
      TextAlign.configure({ types: ["heading", "paragraph"] }),
      TextStyleKit,
      BlockId,
    ],
    [collaboration.provider, collaboration.ydoc, user?.displayName, user?.id],
  );
  const editor = useEditor(
    {
      extensions,
      editable: false,
      onUpdate: ({ editor: current }) => scheduleSave(current),
    },
    [documentId],
  );
  const canEdit =
    (document?.permission === "OWNER" || document?.permission === "EDITOR") &&
    document?.workflowStatus !== "PENDING";
  const canComment = canEdit || document?.permission === "COMMENTER";
  useEffect(() => {
    seeded.current = false;
    revisionRef.current = 0;
  }, [documentId]);
  useEffect(() => {
    if (document && !canEdit && mode === "editing") {
      setMode(canComment ? "suggesting" : "viewing");
    }
  }, [canComment, canEdit, document, mode]);
  useEffect(() => {
    if (editor) editor.setEditable(Boolean(canEdit && mode === "editing"));
  }, [editor, canEdit, mode]);
  useEffect(() => {
    if (
      !editor ||
      !document?.content ||
      !collaboration.syncedAt ||
      seeded.current
    )
      return;
    const fragment = collaboration.ydoc.getXmlFragment("default");
    if (fragment.length === 0) editor.commands.setContent(document.content);
    seeded.current = true;
  }, [editor, document, collaboration.syncedAt, collaboration.ydoc]);
  useEffect(() => {
    setSaveState(
      collaboration.status === "offline"
        ? "offline"
        : (current) => (current === "offline" ? "saved" : current),
    );
  }, [collaboration.status]);

  const persist = useCallback(
    async (current: Editor, reason = "autosave") => {
      if (!canEdit) return;
      setSaveState("saving");
      const payload = {
        content: current.getJSON(),
        expectedRevision: revisionRef.current,
        reason,
      };
      try {
        const saved = await api<DocumentItem>(
          `/api/v1/documents/${documentId}`,
          { method: "PUT", ...jsonBody(payload) },
        );
        revisionRef.current = saved.revision;
        queryClient.setQueryData(["document", documentId], saved);
        setSaveState("saved");
      } catch (cause) {
        if (cause instanceof ApiError && cause.code === "REVISION_CONFLICT") {
          const next = Number(cause.details?.currentRevision ?? 0);
          if (next > 0) {
            revisionRef.current = next;
            try {
              const saved = await api<DocumentItem>(
                `/api/v1/documents/${documentId}`,
                {
                  method: "PUT",
                  ...jsonBody({ ...payload, expectedRevision: next }),
                },
              );
              revisionRef.current = saved.revision;
              queryClient.setQueryData(["document", documentId], saved);
              setSaveState("saved");
              return;
            } catch {
              /* handled below */
            }
          }
        }
        setSaveState(collaboration.status === "offline" ? "offline" : "error");
      }
    },
    [canEdit, collaboration.status, documentId, queryClient],
  );
  function scheduleSave(current: Editor) {
    if (saveTimer.current) window.clearTimeout(saveTimer.current);
    saveTimer.current = window.setTimeout(() => void persist(current), 1500);
  }
  useEffect(
    () => () => {
      if (saveTimer.current) window.clearTimeout(saveTimer.current);
    },
    [],
  );
  const updateMetadata = async (patch: Record<string, unknown>) => {
    if (!editor) return;
    setSaveState("saving");
    try {
      const saved = await api<DocumentItem>(`/api/v1/documents/${documentId}`, {
        method: "PUT",
        ...jsonBody({
          ...patch,
          content: editor.getJSON(),
          expectedRevision: revisionRef.current,
          reason: "metadata",
        }),
      });
      revisionRef.current = saved.revision;
      queryClient.setQueryData(["document", documentId], saved);
      setSaveState("saved");
    } catch {
      setSaveState("error");
    }
  };
  const submitApproval = useMutation({
    mutationFn: () =>
      api(`/api/v1/documents/${documentId}/workflow/submit`, {
        method: "POST",
      }),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["document", documentId] }),
  });
  const removeDocument = useMutation({
    mutationFn: () =>
      api<void>(`/api/v1/documents/${documentId}`, { method: "DELETE" }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["documents"] });
      void queryClient.invalidateQueries({ queryKey: ["user-documents"] });
      navigate("/trash");
    },
  });
  if (documentQuery.isLoading || !editor)
    return <LoadingScreen label="문서를 불러오고 있습니다" />;
  if (documentQuery.error || !document)
    return (
      <Box sx={{ height: "100%", display: "grid", placeItems: "center", p: 3 }}>
        <Box textAlign="center">
          <Typography variant="h2">문서를 열 수 없습니다</Typography>
          <Typography color="text.secondary" my={2}>
            {errorMessage(documentQuery.error)}
          </Typography>
          <Button onClick={() => navigate("/")}>홈으로</Button>
        </Box>
      </Box>
    );
  const side = (
    <SidePanel
      tab={sideTab}
      setTab={setSideTab}
      document={document}
      editor={editor}
      canComment={canComment}
      canEdit={canEdit}
      capabilities={capabilities.data}
    />
  );
  return (
    <Box
      sx={{
        height: "100%",
        minHeight: 0,
        display: "flex",
        flexDirection: "column",
        bgcolor: "#eef0f5",
      }}
    >
      <AppBar
        position="static"
        elevation={0}
        color="inherit"
        sx={{
          borderBottom: "1px solid",
          borderColor: "divider",
          zIndex: theme.zIndex.drawer + 1,
        }}
      >
        <Toolbar sx={{ minHeight: "64px!important", gap: 1 }}>
          <Tooltip title="문서 홈">
            <IconButton aria-label="문서 홈" onClick={() => navigate("/")}>
              <ArrowBack />
            </IconButton>
          </Tooltip>
          <Box sx={{ display: { xs: "none", sm: "block" }, mr: 1 }}>
            <Brand compact />
          </Box>
          <TextField
            variant="standard"
            value={document.title}
            onChange={(event) =>
              queryClient.setQueryData<DocumentItem>(
                ["document", documentId],
                (current) =>
                  current ? { ...current, title: event.target.value } : current,
              )
            }
            onBlur={(event) =>
              void updateMetadata({ title: event.target.value })
            }
            disabled={!canEdit}
            inputProps={{ "aria-label": "문서 제목", maxLength: 240 }}
            InputProps={{
              disableUnderline: true,
              sx: {
                fontWeight: 720,
                fontSize: { xs: 15, sm: 17 },
                minWidth: { xs: 120, sm: 280 },
              },
            }}
            sx={{ flex: { xs: 1, md: "0 1 480px" } }}
          />
          <SaveIndicator state={saveState} />
          <Box sx={{ flex: 1 }} />
          <Stack
            direction="row"
            alignItems="center"
            sx={{ display: { xs: "none", md: "flex" } }}
          >
            {collaboration.users.slice(0, 4).map((member) => (
              <Tooltip
                key={member.clientId}
                title={member.name ?? "공동 편집자"}
              >
                <Avatar
                  sx={{
                    width: 30,
                    height: 30,
                    fontSize: 13,
                    bgcolor: member.color,
                    ml: -0.5,
                    border: "2px solid #fff",
                  }}
                >
                  {member.name?.slice(0, 1)}
                </Avatar>
              </Tooltip>
            ))}
          </Stack>
          {capabilities.data?.workflowEnabled &&
            canEdit &&
            document.workflowStatus !== "PENDING" && (
              <Button
                variant="outlined"
                onClick={() => submitApproval.mutate()}
                disabled={submitApproval.isPending}
                sx={{ display: { xs: "none", lg: "inline-flex" } }}
              >
                검토 요청
              </Button>
            )}
          {document.workflowStatus === "PENDING" && (
            <Chip size="small" color="warning" label="승인 대기" />
          )}
          <Button
            variant="outlined"
            startIcon={<PeopleOutline />}
            onClick={() => setShareOpen(true)}
            sx={{ display: { xs: "none", sm: "inline-flex" } }}
          >
            공유
          </Button>
          <Button
            variant="outlined"
            startIcon={<CommentOutlined />}
            onClick={() => {
              setSideTab("comments");
              setSideOpen(true);
            }}
            sx={{ display: { xs: "none", md: "inline-flex" } }}
          >
            댓글
          </Button>
          <FormControl
            size="small"
            sx={{ minWidth: 120, display: { xs: "none", md: "flex" } }}
          >
            <Select
              value={mode}
              onChange={(event) => setMode(event.target.value as typeof mode)}
            >
              {canEdit && <MenuItem value="editing">편집</MenuItem>}
              {canComment && <MenuItem value="suggesting">제안</MenuItem>}
              <MenuItem value="viewing">보기</MenuItem>
            </Select>
          </FormControl>
          <IconButton
            onClick={(event) => setExportAnchor(event.currentTarget)}
            aria-label="내보내기"
          >
            <DownloadOutlined />
          </IconButton>
          {document.permission === "OWNER" &&
            document.workflowStatus !== "PENDING" && (
              <Tooltip title="휴지통으로 이동">
                <IconButton
                  aria-label="휴지통으로 이동"
                  disabled={removeDocument.isPending}
                  onClick={() => {
                    if (window.confirm("이 문서를 휴지통으로 이동할까요?")) {
                      removeDocument.mutate();
                    }
                  }}
                >
                  <DeleteOutline />
                </IconButton>
              </Tooltip>
            )}
          <IconButton
            onClick={() => setSideOpen((value) => !value)}
            sx={{ display: { lg: "none" } }}
          >
            <AutoAwesome />
          </IconButton>
          <IconButton onClick={(event) => setMenuAnchor(event.currentTarget)}>
            <Avatar
              src={user?.avatarUrl}
              sx={{
                width: 34,
                height: 34,
                bgcolor: "primary.main",
                fontSize: 14,
              }}
            >
              {user?.displayName.slice(0, 1)}
            </Avatar>
          </IconButton>
          <IconButton
            onClick={() => setMobileTools((value) => !value)}
            sx={{ display: { xs: "flex", sm: "none" } }}
          >
            <MenuIcon />
          </IconButton>
        </Toolbar>
      </AppBar>
      <Box
        sx={{
          display: { xs: mobileTools ? "block" : "none", sm: "block" },
          borderBottom: "1px solid",
          borderColor: "divider",
          bgcolor: "#fff",
          overflowX: "auto",
          pointerEvents: canEdit && mode === "editing" ? "auto" : "none",
          opacity: canEdit && mode === "editing" ? 1 : 0.62,
        }}
      >
        <EditorToolbar editor={editor} documentId={documentId} />
      </Box>
      <Box sx={{ display: "flex", minHeight: 0, flex: 1 }}>
        <Box
          sx={{
            flex: 1,
            minWidth: 0,
            overflow: "auto",
            py: { xs: 2, sm: 4 },
            px: { xs: 1, sm: 3 },
          }}
        >
          <Paper
            sx={{
              width: "min(860px,100%)",
              minHeight: 960,
              mx: "auto",
              px: { xs: 2.5, sm: 7, md: 9 },
              py: { xs: 4, sm: 7 },
              borderRadius: { xs: 1, sm: 2 },
              boxShadow: "0 5px 28px rgba(30,31,45,.09)",
            }}
          >
            {mode === "suggesting" && (
              <Alert severity="info" sx={{ mb: 2 }}>
                제안 모드입니다. 문장을 선택한 뒤 오른쪽 ‘제안’ 패널에서 대체
                문구를 등록하세요.
              </Alert>
            )}
            <EditorContent editor={editor} />
            <AISelectionMenu
              editor={editor}
              enabled={Boolean(capabilities.data?.aiEnabled)}
              canEdit={canEdit && mode === "editing"}
              maxTokens={capabilities.data?.maxAiTokens}
            />
          </Paper>
        </Box>
        {!compact && sideOpen && (
          <Box
            sx={{
              width: 390,
              borderLeft: "1px solid",
              borderColor: "divider",
              bgcolor: "#fff",
              minHeight: 0,
            }}
          >
            {side}
          </Box>
        )}
      </Box>
      <Drawer
        anchor="right"
        open={compact && sideOpen}
        onClose={() => setSideOpen(false)}
        PaperProps={{
          sx: {
            width: { xs: "100%", sm: 390 },
            mt: "64px",
            height: "calc(100% - 64px)",
          },
        }}
      >
        {side}
      </Drawer>
      <ShareDialog
        open={shareOpen}
        onClose={() => setShareOpen(false)}
        document={document}
        onVisibilityChange={(visibility) => updateMetadata({ visibility })}
      />
      <Menu
        anchorEl={exportAnchor}
        open={Boolean(exportAnchor)}
        onClose={() => setExportAnchor(null)}
      >
        {capabilities.data?.docxExport && (
          <MenuItem
            component="a"
            href={`/api/v1/documents/${documentId}/export/docx`}
          >
            <ListItemIcon>
              <DownloadOutlined />
            </ListItemIcon>
            DOCX
          </MenuItem>
        )}
        {capabilities.data?.pdfExport && (
          <MenuItem
            component="a"
            href={`/api/v1/documents/${documentId}/export/pdf`}
          >
            <ListItemIcon>
              <DownloadOutlined />
            </ListItemIcon>
            PDF
          </MenuItem>
        )}
        <MenuItem
          component="a"
          href={`/api/v1/documents/${documentId}/export/md`}
        >
          Markdown
        </MenuItem>
        <MenuItem
          component="a"
          href={`/api/v1/documents/${documentId}/export/html`}
        >
          HTML
        </MenuItem>
        <MenuItem
          component="a"
          href={`/api/v1/documents/${documentId}/export/txt`}
        >
          TXT
        </MenuItem>
      </Menu>
      <Menu
        anchorEl={menuAnchor}
        open={Boolean(menuAnchor)}
        onClose={() => setMenuAnchor(null)}
        slotProps={{
          paper: {
            className: "admin-menu-scroll",
            sx: { width: 280, maxHeight: 400 },
          },
        }}
      >
        <Box px={2} py={1.5}>
          <Typography fontWeight={700}>{user?.displayName}</Typography>
          <Typography variant="body2" color="text.secondary">
            {user?.email}
          </Typography>
        </Box>
        <Divider />
        <MenuItem onClick={() => navigate("/settings")}>개인 설정</MenuItem>
        {user?.role === "ADMIN" && (
          <MenuItem onClick={() => navigate("/admin")}>서비스 관리</MenuItem>
        )}
        <MenuItem
          onClick={async () => {
            await logout();
            navigate("/login");
          }}
        >
          로그아웃
        </MenuItem>
        <Divider />
        <Box px={2} py={1.25}>
          <Typography variant="caption" color="text.secondary">
            muni {build?.version ?? "dev"} ·{" "}
            {build?.commit?.slice(0, 8) ?? "none"}
          </Typography>
        </Box>
      </Menu>
    </Box>
  );
}

function SaveIndicator({
  state,
}: {
  state: "saved" | "saving" | "offline" | "error";
}) {
  const values = {
    saved: [<CloudDoneOutlined key="i" fontSize="small" />, "저장됨"],
    saving: [<CircularProgress key="i" size={16} />, "저장 중"],
    offline: [<CloudOffOutlined key="i" fontSize="small" />, "오프라인"],
    error: [
      <CloudOffOutlined key="i" color="error" fontSize="small" />,
      "저장 오류",
    ],
  } as const;
  return (
    <Stack
      direction="row"
      gap={0.5}
      alignItems="center"
      color={state === "error" ? "error.main" : "text.secondary"}
      sx={{ display: { xs: "none", sm: "flex" } }}
    >
      {values[state][0]}
      <Typography variant="caption">{values[state][1]}</Typography>
    </Stack>
  );
}

function EditorToolbar({
  editor,
  documentId,
}: {
  editor: Editor;
  documentId: string;
}) {
  const state = useEditorState({
    editor,
    selector: ({ editor: current }) => ({
      bold: current.isActive("bold"),
      italic: current.isActive("italic"),
      underline: current.isActive("underline"),
      strike: current.isActive("strike"),
      bullet: current.isActive("bulletList"),
      ordered: current.isActive("orderedList"),
      quote: current.isActive("blockquote"),
      code: current.isActive("codeBlock"),
      align: current.getAttributes("paragraph").textAlign ?? "left",
    }),
  });
  const link = () => {
    const previous = editor.getAttributes("link").href as string | undefined;
    const href = window.prompt(
      "연결할 URL을 입력하세요.",
      previous ?? "https://",
    );
    if (href === null) return;
    if (!href) {
      editor.chain().focus().extendMarkRange("link").unsetLink().run();
      return;
    }
    editor.chain().focus().extendMarkRange("link").setLink({ href }).run();
  };
  const uploadImage = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;
    const form = new FormData();
    form.set("file", file);
    try {
      const result = await api<{ url: string }>(
        `/api/v1/documents/${documentId}/attachments`,
        { method: "POST", body: form },
      );
      editor
        .chain()
        .focus()
        .setImage({ src: result.url, alt: file.name })
        .run();
    } finally {
      event.target.value = "";
    }
  };
  return (
    <Toolbar
      variant="dense"
      sx={{
        minHeight: "52px!important",
        gap: 0.5,
        width: "max-content",
        minWidth: "100%",
        px: { xs: 1, sm: 2 },
      }}
    >
      <Tooltip title="실행 취소">
        <span>
          <IconButton
            disabled={!editor.can().undo()}
            onClick={() => editor.chain().focus().undo().run()}
          >
            <Undo />
          </IconButton>
        </span>
      </Tooltip>
      <Tooltip title="다시 실행">
        <span>
          <IconButton
            disabled={!editor.can().redo()}
            onClick={() => editor.chain().focus().redo().run()}
          >
            <Redo />
          </IconButton>
        </span>
      </Tooltip>
      <Divider flexItem orientation="vertical" sx={{ mx: 0.5 }} />
      <Select
        size="small"
        value={
          editor.isActive("heading", { level: 1 })
            ? "h1"
            : editor.isActive("heading", { level: 2 })
              ? "h2"
              : editor.isActive("heading", { level: 3 })
                ? "h3"
                : editor.isActive("heading", { level: 4 })
                  ? "h4"
                  : editor.isActive("heading", { level: 5 })
                    ? "h5"
                    : editor.isActive("heading", { level: 6 })
                      ? "h6"
                      : "p"
        }
        onChange={(event) => {
          const value = event.target.value;
          if (value === "p") editor.chain().focus().setParagraph().run();
          else
            editor
              .chain()
              .focus()
              .toggleHeading({
                level: Number(value.slice(1)) as 1 | 2 | 3 | 4 | 5 | 6,
              })
              .run();
        }}
        sx={{ minWidth: 108 }}
      >
        <MenuItem value="p">본문</MenuItem>
        <MenuItem value="h1">제목 1</MenuItem>
        <MenuItem value="h2">제목 2</MenuItem>
        <MenuItem value="h3">제목 3</MenuItem>
        <MenuItem value="h4">제목 4</MenuItem>
        <MenuItem value="h5">제목 5</MenuItem>
        <MenuItem value="h6">제목 6</MenuItem>
      </Select>
      <Select
        size="small"
        value={editor.getAttributes("textStyle").fontSize ?? "17px"}
        onChange={(event) =>
          editor.chain().focus().setFontSize(event.target.value).run()
        }
        sx={{ minWidth: 84 }}
      >
        {["14px", "16px", "17px", "18px", "20px", "24px", "32px"].map(
          (value) => (
            <MenuItem key={value} value={value}>
              {value.replace("px", "")}
            </MenuItem>
          ),
        )}
      </Select>
      <Select
        size="small"
        value={editor.getAttributes("textStyle").fontFamily ?? "Noto Sans KR"}
        onChange={(event) =>
          editor.chain().focus().setFontFamily(event.target.value).run()
        }
        sx={{ minWidth: 130 }}
      >
        <MenuItem value="Noto Sans KR">Noto Sans KR</MenuItem>
        <MenuItem value="serif">명조 계열</MenuItem>
        <MenuItem value="monospace">고정폭</MenuItem>
      </Select>
      <Tooltip title="글자 색">
        <IconButton component="label" aria-label="글자 색">
          <FormatColorText />
          <input
            hidden
            type="color"
            onChange={(event) =>
              editor.chain().focus().setColor(event.target.value).run()
            }
          />
        </IconButton>
      </Tooltip>
      <Tooltip title="강조 색">
        <IconButton component="label" aria-label="강조 색">
          <BorderColor />
          <input
            hidden
            type="color"
            defaultValue="#fff59d"
            onChange={(event) =>
              editor
                .chain()
                .focus()
                .toggleHighlight({ color: event.target.value })
                .run()
            }
          />
        </IconButton>
      </Tooltip>
      <ToggleButtonGroup size="small">
        <ToggleButton
          value="bold"
          selected={state.bold}
          onClick={() => editor.chain().focus().toggleBold().run()}
        >
          <FormatBold />
        </ToggleButton>
        <ToggleButton
          value="italic"
          selected={state.italic}
          onClick={() => editor.chain().focus().toggleItalic().run()}
        >
          <FormatItalic />
        </ToggleButton>
        <ToggleButton
          value="underline"
          selected={state.underline}
          onClick={() => editor.chain().focus().toggleUnderline().run()}
        >
          <FormatUnderlined />
        </ToggleButton>
        <ToggleButton
          value="strike"
          selected={state.strike}
          onClick={() => editor.chain().focus().toggleStrike().run()}
        >
          <StrikethroughS />
        </ToggleButton>
      </ToggleButtonGroup>
      <ToggleButtonGroup
        size="small"
        value={state.align}
        exclusive
        onChange={(_, value) =>
          value && editor.chain().focus().setTextAlign(value).run()
        }
      >
        <ToggleButton value="left">
          <FormatAlignLeft />
        </ToggleButton>
        <ToggleButton value="center">
          <FormatAlignCenter />
        </ToggleButton>
        <ToggleButton value="right">
          <FormatAlignRight />
        </ToggleButton>
      </ToggleButtonGroup>
      <ToggleButtonGroup size="small">
        <ToggleButton
          value="bullet"
          selected={state.bullet}
          onClick={() => editor.chain().focus().toggleBulletList().run()}
        >
          <FormatListBulleted />
        </ToggleButton>
        <ToggleButton
          value="ordered"
          selected={state.ordered}
          onClick={() => editor.chain().focus().toggleOrderedList().run()}
        >
          <FormatListNumbered />
        </ToggleButton>
        <ToggleButton
          value="task"
          selected={editor.isActive("taskList")}
          onClick={() => editor.chain().focus().toggleTaskList().run()}
        >
          <TaskAlt />
        </ToggleButton>
        <ToggleButton
          value="quote"
          selected={state.quote}
          onClick={() => editor.chain().focus().toggleBlockquote().run()}
        >
          <FormatQuote />
        </ToggleButton>
        <ToggleButton
          value="code"
          selected={state.code}
          onClick={() => editor.chain().focus().toggleCodeBlock().run()}
        >
          <Code />
        </ToggleButton>
      </ToggleButtonGroup>
      <Tooltip title="들여쓰기">
        <span>
          <IconButton
            disabled={
              !editor
                .can()
                .sinkListItem(
                  editor.isActive("taskItem") ? "taskItem" : "listItem",
                )
            }
            onClick={() =>
              editor
                .chain()
                .focus()
                .sinkListItem(
                  editor.isActive("taskItem") ? "taskItem" : "listItem",
                )
                .run()
            }
          >
            <FormatIndentIncrease />
          </IconButton>
        </span>
      </Tooltip>
      <Tooltip title="내어쓰기">
        <span>
          <IconButton
            disabled={
              !editor
                .can()
                .liftListItem(
                  editor.isActive("taskItem") ? "taskItem" : "listItem",
                )
            }
            onClick={() =>
              editor
                .chain()
                .focus()
                .liftListItem(
                  editor.isActive("taskItem") ? "taskItem" : "listItem",
                )
                .run()
            }
          >
            <FormatIndentDecrease />
          </IconButton>
        </span>
      </Tooltip>
      <Tooltip title="링크">
        <IconButton onClick={link}>
          <InsertLink />
        </IconButton>
      </Tooltip>
      <Tooltip title="이미지 업로드">
        <IconButton component="label">
          <ImageOutlined />
          <input hidden type="file" accept="image/*" onChange={uploadImage} />
        </IconButton>
      </Tooltip>
      <Tooltip title="표 삽입">
        <IconButton
          onClick={() =>
            editor
              .chain()
              .focus()
              .insertTable({ rows: 3, cols: 3, withHeaderRow: true })
              .run()
          }
        >
          <TableChartOutlined />
        </IconButton>
      </Tooltip>
      <Tooltip title="가로 구분선">
        <IconButton
          onClick={() => editor.chain().focus().setHorizontalRule().run()}
        >
          <HorizontalRule />
        </IconButton>
      </Tooltip>
    </Toolbar>
  );
}

function SidePanel({
  tab,
  setTab,
  document,
  editor,
  canComment,
  canEdit,
  capabilities,
}: {
  tab: SideTab;
  setTab: (value: SideTab) => void;
  document: DocumentItem;
  editor: Editor;
  canComment: boolean;
  canEdit: boolean;
  capabilities?: Capability;
}) {
  return (
    <Box sx={{ height: "100%", display: "flex", flexDirection: "column" }}>
      <Tabs
        value={tab}
        onChange={(_, value) => setTab(value)}
        variant="scrollable"
        scrollButtons={false}
        sx={{
          borderBottom: "1px solid",
          borderColor: "divider",
          minHeight: 49,
        }}
      >
        <Tab
          value="ai"
          icon={<AutoAwesome />}
          iconPosition="start"
          label="AI"
        />
        <Tab
          value="comments"
          icon={<CommentOutlined />}
          iconPosition="start"
          label="댓글"
        />
        <Tab
          value="suggestions"
          icon={<AddCommentOutlined />}
          iconPosition="start"
          label="제안"
        />
        <Tab
          value="history"
          icon={<History />}
          iconPosition="start"
          label="버전"
        />
      </Tabs>
      <Box sx={{ flex: 1, minHeight: 0, overflowY: "auto", p: 2 }}>
        {tab === "ai" && (
          <AgentPanel
            document={document}
            editor={editor}
            enabled={capabilities?.aiEnabled ?? true}
            canEdit={canEdit}
            maxTokens={capabilities?.maxAiTokens ?? 32768}
          />
        )}{" "}
        {tab === "comments" && (
          <CommentsPanel
            document={document}
            editor={editor}
            canComment={canComment}
          />
        )}{" "}
        {tab === "suggestions" && (
          <SuggestionsPanel
            document={document}
            editor={editor}
            canComment={canComment}
            canEdit={canEdit}
          />
        )}{" "}
        {tab === "history" && <HistoryPanel document={document} />}
      </Box>
    </Box>
  );
}

function ShareDialog({
  open,
  onClose,
  document,
  onVisibilityChange,
}: {
  open: boolean;
  onClose: () => void;
  document: DocumentItem;
  onVisibilityChange: (visibility: DocumentItem["visibility"]) => Promise<void>;
}) {
  const client = useQueryClient();
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState<UserSearch | null>(null);
  const [role, setRole] = useState("VIEWER");
  const permissions = useQuery({
    queryKey: ["permissions", document.id],
    queryFn: () =>
      api<Permission[]>(`/api/v1/documents/${document.id}/permissions`),
    enabled: open && document.permission === "OWNER",
  });
  const users = useQuery({
    queryKey: ["user-search", query],
    queryFn: () =>
      api<UserSearch[]>(`/api/v1/users/search?q=${encodeURIComponent(query)}`),
    enabled: open && query.length >= 2,
  });
  const add = useMutation({
    mutationFn: () =>
      api(`/api/v1/documents/${document.id}/permissions`, {
        method: "PUT",
        ...jsonBody({ subjectType: "USER", subjectId: selected?.id, role }),
      }),
    onSuccess: () => {
      setSelected(null);
      setQuery("");
      void client.invalidateQueries({ queryKey: ["permissions", document.id] });
    },
  });
  const remove = useMutation({
    mutationFn: (permissionId: string) =>
      api(`/api/v1/documents/${document.id}/permissions/${permissionId}`, {
        method: "DELETE",
      }),
    onSuccess: () =>
      client.invalidateQueries({ queryKey: ["permissions", document.id] }),
  });
  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>문서 공유</DialogTitle>
      <DialogContent>
        {document.permission !== "OWNER" ? (
          <Alert severity="info">
            소유자만 공유 권한을 변경할 수 있습니다.
          </Alert>
        ) : (
          <Stack gap={2} pt={0.5}>
            {(add.error || remove.error) && (
              <Alert severity="error">
                {errorMessage(add.error || remove.error)}
              </Alert>
            )}
            <FormControl size="small">
              <InputLabel>기본 공유 범위</InputLabel>
              <Select
                label="기본 공유 범위"
                value={document.visibility}
                onChange={(event) =>
                  void onVisibilityChange(
                    event.target.value as DocumentItem["visibility"],
                  )
                }
              >
                <MenuItem value="RESTRICTED">지정 사용자만</MenuItem>
                <MenuItem value="WORKSPACE">워크스페이스 구성원</MenuItem>
                <MenuItem value="ORGANIZATION">조직 내 모든 사용자</MenuItem>
              </Select>
            </FormControl>
            <TextField
              label="사용자 검색"
              value={query}
              onChange={(e) => {
                setQuery(e.target.value);
                setSelected(null);
              }}
              placeholder="이름, 이메일 또는 아이디"
            />
            {!selected &&
              (users.data ?? []).map((item) => (
                <Paper
                  key={item.id}
                  variant="outlined"
                  onClick={() => {
                    setSelected(item);
                    setQuery(item.displayName);
                  }}
                  sx={{ p: 1.25, cursor: "pointer" }}
                >
                  <Typography fontWeight={650}>{item.displayName}</Typography>
                  <Typography variant="body2" color="text.secondary">
                    {item.email} · {item.username}
                  </Typography>
                </Paper>
              ))}
            <FormControl size="small">
              <InputLabel>권한</InputLabel>
              <Select
                value={role}
                label="권한"
                onChange={(e) => setRole(e.target.value)}
              >
                <MenuItem value="VIEWER">조회</MenuItem>
                <MenuItem value="COMMENTER">댓글</MenuItem>
                <MenuItem value="EDITOR">편집</MenuItem>
              </Select>
            </FormControl>
            <Button
              variant="contained"
              disabled={!selected}
              onClick={() => add.mutate()}
            >
              공유 추가
            </Button>
            <Divider />
            {(permissions.data ?? []).map((item) => (
              <Stack
                key={item.id}
                direction="row"
                justifyContent="space-between"
              >
                <Box>
                  <Typography fontWeight={650}>{item.label}</Typography>
                  <Typography variant="body2" color="text.secondary">
                    {item.role}
                  </Typography>
                </Box>
                <Button
                  size="small"
                  color="error"
                  onClick={() => remove.mutate(item.id)}
                >
                  제거
                </Button>
              </Stack>
            ))}
          </Stack>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>닫기</Button>
      </DialogActions>
    </Dialog>
  );
}
