import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { EditorContent, useEditor, type Editor } from "@tiptap/react";
import Collaboration from "@tiptap/extension-collaboration";
import CollaborationCaret from "@tiptap/extension-collaboration-caret";
import Placeholder from "@tiptap/extension-placeholder";
import { EditorSidebar } from "../features/editor/EditorSidebar";
import { EditorStatus } from "../features/editor/EditorStatus";
import { EditorToolbar } from "../features/editor/EditorToolbar";
import { AISelectionMenu } from "../features/editor/ai/AISelectionMenu";
import { EditorStatusBar } from "../features/editor/EditorStatusBar";
import { ImageMenu } from "../features/editor/ImageMenu";
import { NotificationBell } from "../features/notifications/NotificationBell";
import { DocumentTags } from "../features/editor/DocumentTags";
import { ApprovalLineDialog } from "../features/editor/ApprovalLineDialog";
import { MoveDocumentDialog } from "../features/editor/MoveDocumentDialog";
import { recallPosition, rememberPosition } from "../features/editor/lastPosition";
import { LinkMenu } from "../features/editor/LinkMenu";
import { SlashMenu } from "../features/editor/insert/SlashMenu";
import { documentExtensions } from "../features/editor/documentExtensions";
import { PageFurnitureDialog } from "../features/editor/PageFurnitureDialog";
import { ShortcutsDialog } from "../features/editor/ShortcutsDialog";
import { TableTools } from "../features/editor/TableTools";
import { FindReplaceBar } from "../features/editor/find/FindReplaceBar";
import { OutlinePanel } from "../features/editor/outline/OutlinePanel";
import { PasteBehaviour } from "../features/editor/extensions/pasteBehaviour";
import { validScheme, type NumberingScheme } from "../features/editor/outline/numbering";
import { SearchHighlight } from "../features/editor/extensions/searchHighlight";
import { ShareDialog } from "../features/editor/sharing/ShareDialog";
import { PresentationDialog } from "../features/editor/presentations/PresentationDialog";
import {
  ArrowBack,
  AutoAwesome,
  SlideshowOutlined,
  CommentOutlined,
  DashboardCustomizeOutlined,
  DownloadOutlined,
  DeleteOutline,
  ContentCopyOutlined,
  VerticalSplitOutlined,
  DriveFileMoveOutlined,
  PeopleOutline,
} from "@mui/icons-material";
import {
  Alert,
  AppBar,
  Avatar,
  Box,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  Drawer,
  FormControl,
  IconButton,
  ListItemIcon,
  Menu,
  MenuItem,
  Paper,
  Select,
  Stack,
  TextField,
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
import type { Capability, SideTab } from "../features/editor/types";

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
  const [furnitureOpen, setFurnitureOpen] = useState(false);
  const [deckOpen, setDeckOpen] = useState(false);
  const [shareOpen, setShareOpen] = useState(false);
  // The reading aids Google Docs keeps around the page: an outline beside it,
  // find and replace over it, a zoom, and the shortcut list.
  const [outlineOpen, setOutlineOpen] = useState(!compact);
  const [find, setFind] = useState<{ open: boolean; replace: boolean }>({
    open: false,
    replace: false,
  });
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
  const [templateName, setTemplateName] = useState<string | null>(null);
  const [approvalOpen, setApprovalOpen] = useState(false);
  const [moveOpen, setMoveOpen] = useState(false);
  const [zoom, setZoom] = useState(() => readZoom());
  const revisionRef = useRef(0);
  const saveTimer = useRef<number | undefined>(undefined);
  const seeded = useRef(false);
  const restored = useRef(false);
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
  // The scheme belongs to the document, so everyone reading it sees the same
  // section numbers.
  const numbering: NumberingScheme = validScheme(document?.headingNumbering);
  const collaboration = useCollaboration(
    documentId,
    user,
    document?.crdtGeneration ?? 0,
  );
  const extensions = useMemo(
    () => [
      ...documentExtensions(),
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
      PasteBehaviour,
      SearchHighlight,
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
    restored.current = false;
    revisionRef.current = 0;
  }, [documentId]);
  // Reopening a twenty-page report at the top means scrolling back every time.
  useEffect(() => {
    if (!editor || !collaboration.syncedAt || restored.current) return;
    restored.current = true;
    const position = recallPosition(window.localStorage, documentId);
    const size = editor.state.doc.content.size;
    // Only worth doing for a document long enough to have been scrolled, and
    // only when the position still exists after whatever changed since.
    if (!position || position >= size || size < 600) return;
    editor.chain().setTextSelection(position).scrollIntoView().run();
  }, [collaboration.syncedAt, documentId, editor]);
  useEffect(() => {
    if (!editor) return;
    let timer: number | undefined;
    const onSelection = () => {
      window.clearTimeout(timer);
      timer = window.setTimeout(
        () => rememberPosition(window.localStorage, documentId, editor.state.selection.from),
        800,
      );
    };
    editor.on("selectionUpdate", onSelection);
    return () => {
      window.clearTimeout(timer);
      editor.off("selectionUpdate", onSelection);
    };
  }, [documentId, editor]);
  useEffect(() => {
    if (document && !canEdit && mode === "editing") {
      setMode(canComment ? "suggesting" : "viewing");
    }
  }, [canComment, canEdit, document, mode]);
  useEffect(() => {
    if (editor) editor.setEditable(Boolean(canEdit && mode === "editing"));
  }, [editor, canEdit, mode]);
  // The scheme arrives with the document, and changes when anyone edits it.
  useEffect(() => {
    editor?.commands.setHeadingNumbering(numbering);
  }, [editor, numbering]);
  useEffect(() => {
    if (
      !editor ||
      !document?.content ||
      !collaboration.syncedAt ||
      seeded.current
    )
      return;
    const fragment = collaboration.ydoc.getXmlFragment("default");
    if (fragment.length === 0) {
      // Only the client the server picked writes the stored content into an
      // empty document; the others receive it as an ordinary update. Two
      // clients seeding at once would insert the whole document twice.
      if (!collaboration.maySeed) return;
      editor.commands.setContent(document.content);
    }
    seeded.current = true;
  }, [
    editor,
    document,
    collaboration.syncedAt,
    collaboration.ydoc,
    collaboration.maySeed,
  ]);
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
  // The shortcuts a person coming from Google Docs will try first. Tiptap
  // already owns the formatting ones; these are the document-level keys the
  // browser would otherwise take.
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const meta = event.metaKey || event.ctrlKey;
      if (!meta) return;
      const key = event.key.toLowerCase();
      if (key === "f") {
        event.preventDefault();
        setFind({ open: true, replace: false });
      } else if (key === "h") {
        event.preventDefault();
        setFind({ open: true, replace: true });
      } else if (key === "s") {
        // Everything is saved continuously; the key is here because people
        // press it anyway, and it should not open the browser's save dialog.
        event.preventDefault();
        if (editor) void persist(editor, "manual");
      } else if (key === "/") {
        event.preventDefault();
        setShortcutsOpen(true);
      } else if (key === "\\") {
        event.preventDefault();
        setOutlineOpen((value) => !value);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [editor, persist]);
  useEffect(() => {
    try {
      window.localStorage.setItem(zoomKey, String(zoom));
    } catch {
      /* A browser that refuses storage simply forgets the zoom. */
    }
  }, [zoom]);
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
  // A template usually starts life as a document somebody already wrote.
  const duplicate = useMutation({
    mutationFn: () =>
      api<DocumentItem>(`/api/v1/documents/${documentId}/duplicate`, {
        method: "POST",
      }),
    onSuccess: (copy) => {
      void queryClient.invalidateQueries({ queryKey: ["documents"] });
      void queryClient.invalidateQueries({ queryKey: ["user-documents"] });
      navigate(`/docs/${copy.id}`);
    },
  });
  const saveTemplate = useMutation({
    mutationFn: () =>
      api(`/api/v1/workspaces/${document?.workspaceId}/templates`, {
        method: "POST",
        ...jsonBody({ name: templateName, fromDocumentId: documentId }),
      }),
    onSuccess: () => setTemplateName(null),
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
    <EditorSidebar
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
        className="muni-no-print"
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
          <EditorStatus state={saveState} />
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
                onClick={() => setApprovalOpen(true)}
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
            aria-label="사이드 패널 열기"
          >
            <AutoAwesome />
          </IconButton>
          <NotificationBell />
          <IconButton
            aria-label="내 계정 메뉴"
            onClick={(event) => setMenuAnchor(event.currentTarget)}
          >
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
        </Toolbar>
      </AppBar>
      <Box
        className="muni-no-print"
        sx={{
          // The toolbar used to be hidden behind a menu button on a phone,
          // which put every formatting control two taps away. It is always
          // there now; the controls that do not fit have their own menu.
          display: "block",
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
        {outlineOpen && !compact && (
          <OutlinePanel
            editor={editor}
            onClose={() => setOutlineOpen(false)}
            numbering={numbering}
            onNumberingChange={
              canEdit
                ? (scheme) => void updateMetadata({ headingNumbering: scheme })
                : undefined
            }
          />
        )}
        <Box
          className="muni-page-scroll"
          sx={{
            flex: 1,
            minWidth: 0,
            overflow: "auto",
            position: "relative",
            py: { xs: 2, sm: 4 },
            px: { xs: 1, sm: 3 },
          }}
        >
          <FindReplaceBar
            editor={editor}
            open={find.open}
            withReplace={find.replace}
            canEdit={canEdit && mode === "editing"}
            onClose={() => setFind({ open: false, replace: false })}
          />
          <Paper
            className="muni-page"
            sx={{
              // The page keeps its width and is scaled, the way a zoom works
              // on paper — reflowing the text instead would change where every
              // line breaks and make the zoom useless for checking a layout.
              width: 860,
              maxWidth: zoom === 100 ? "100%" : "none",
              minHeight: 960,
              mx: "auto",
              px: { xs: 2.5, sm: 7, md: 9 },
              py: { xs: 4, sm: 7 },
              borderRadius: { xs: 1, sm: 2 },
              boxShadow: "0 5px 28px rgba(30,31,45,.09)",
              transform: zoom === 100 ? undefined : `scale(${zoom / 100})`,
              transformOrigin: "top center",
              transition: "transform .12s ease-out",
            }}
          >
            <Box sx={{ mb: 2 }}>
              <DocumentTags document={document} canEdit={canEdit} />
            </Box>
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
            <TableTools editor={editor} canEdit={canEdit && mode === "editing"} />
            <LinkMenu editor={editor} canEdit={canEdit && mode === "editing"} />
            <ImageMenu editor={editor} canEdit={canEdit && mode === "editing"} />
            <SlashMenu
              editor={editor}
              documentId={documentId}
              canEdit={canEdit && mode === "editing"}
            />
          </Paper>
        </Box>
        {!compact && sideOpen && (
          <Box
            className="muni-no-print"
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
        anchor="left"
        open={compact && outlineOpen}
        onClose={() => setOutlineOpen(false)}
        PaperProps={{ sx: { mt: "64px", height: "calc(100% - 64px)" } }}
      >
        <OutlinePanel
          editor={editor}
          onClose={() => setOutlineOpen(false)}
          numbering={numbering}
          onNumberingChange={
            canEdit
              ? (scheme) => void updateMetadata({ headingNumbering: scheme })
              : undefined
          }
        />
      </Drawer>
      <EditorStatusBar
        editor={editor}
        zoom={zoom}
        onZoom={setZoom}
        outlineOpen={outlineOpen}
        onToggleOutline={() => setOutlineOpen((value) => !value)}
        onShortcuts={() => setShortcutsOpen(true)}
      />
      <ShortcutsDialog
        open={shortcutsOpen}
        onClose={() => setShortcutsOpen(false)}
      />
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
      <Dialog
        open={templateName !== null}
        onClose={() => setTemplateName(null)}
        fullWidth
        maxWidth="sm"
      >
        <DialogTitle>서식으로 저장</DialogTitle>
        <DialogContent>
          {saveTemplate.error && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {errorMessage(saveTemplate.error)}
            </Alert>
          )}
          <Typography variant="body2" color="text.secondary" mb={2}>
            이 문서의 현재 내용을 워크스페이스 서식으로 저장합니다. 새 문서를
            만들 때 시작점으로 고를 수 있습니다. 서식은 복사되는 것이라, 나중에
            서식을 바꾸거나 지워도 이미 만든 문서는 그대로입니다.
          </Typography>
          <TextField
            autoFocus
            fullWidth
            label="서식 이름"
            value={templateName ?? ""}
            inputProps={{ maxLength: 120 }}
            onChange={(event) => setTemplateName(event.target.value)}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setTemplateName(null)}>취소</Button>
          <Button
            variant="contained"
            disabled={!templateName?.trim() || saveTemplate.isPending}
            onClick={() => saveTemplate.mutate()}
          >
            저장
          </Button>
        </DialogActions>
      </Dialog>
      <MoveDocumentDialog
        open={moveOpen}
        onClose={() => setMoveOpen(false)}
        document={document}
      />
      <ApprovalLineDialog
        open={approvalOpen}
        onClose={() => setApprovalOpen(false)}
        documentId={documentId}
        onSubmitted={() =>
          queryClient.invalidateQueries({ queryKey: ["document", documentId] })
        }
      />
      <ShareDialog
        open={shareOpen}
        onClose={() => setShareOpen(false)}
        document={document}
        onVisibilityChange={(visibility) => updateMetadata({ visibility })}
      />
      {document && (
        <PresentationDialog
          open={deckOpen}
          onClose={() => setDeckOpen(false)}
          documentId={documentId}
          documentTitle={document.title}
        />
      )}
      <PageFurnitureDialog
        open={furnitureOpen}
        onClose={() => setFurnitureOpen(false)}
        header={document.pageHeader ?? ""}
        footer={document.pageFooter ?? ""}
        orientation={document.pageOrientation ?? "PORTRAIT"}
        title={document.title}
        canEdit={canEdit}
        onSave={(values) => updateMetadata(values)}
      />
      <Menu
        anchorEl={exportAnchor}
        open={Boolean(exportAnchor)}
        onClose={() => setExportAnchor(null)}
      >
        {capabilities.data?.presentations && [
          <MenuItem
            key="make-deck"
            onClick={() => {
              setExportAnchor(null);
              setSideTab("presentations");
              setSideOpen(true);
              setDeckOpen(true);
            }}
          >
            <ListItemIcon>
              <SlideshowOutlined />
            </ListItemIcon>
            발표자료 만들기
          </MenuItem>,
          <Divider key="deck-divider" />,
        ]}
        <MenuItem
          onClick={() => {
            setExportAnchor(null);
            setFurnitureOpen(true);
          }}
        >
          <ListItemIcon>
            <VerticalSplitOutlined />
          </ListItemIcon>
          페이지 설정
        </MenuItem>
        <Divider />
        <MenuItem
          onClick={() => {
            setExportAnchor(null);
            duplicate.mutate();
          }}
        >
          <ListItemIcon>
            <ContentCopyOutlined />
          </ListItemIcon>
          문서 복제
        </MenuItem>,
        <MenuItem
          onClick={() => {
            setExportAnchor(null);
            setMoveOpen(true);
          }}
        >
          <ListItemIcon>
            <DriveFileMoveOutlined />
          </ListItemIcon>
          다른 폴더로 옮기기
        </MenuItem>,
        <MenuItem
          onClick={() => {
            setExportAnchor(null);
            setTemplateName(document?.title ?? "");
          }}
        >
          <ListItemIcon>
            <DashboardCustomizeOutlined />
          </ListItemIcon>
          서식으로 저장
        </MenuItem>,
        <Divider key="template-divider" />,
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

const zoomKey = "muni:editor:zoom";

/** The zoom is a per-reader preference, so it lives in the browser. */
function readZoom(): number {
  try {
    const stored = Number(window.localStorage.getItem(zoomKey));
    if (stored >= 50 && stored <= 200) return Math.round(stored);
  } catch {
    /* A browser that refuses storage just starts at 100%. */
  }
  return 100;
}
