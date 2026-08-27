import { useEffect, useMemo, useState } from "react";
import { EditorContent, useEditor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import Highlight from "@tiptap/extension-highlight";
import { TableKit } from "@tiptap/extension-table";
import TaskList from "@tiptap/extension-task-list";
import TaskItem from "@tiptap/extension-task-item";
import TextAlign from "@tiptap/extension-text-align";
import { TextStyleKit } from "@tiptap/extension-text-style";
import Superscript from "@tiptap/extension-superscript";
import Subscript from "@tiptap/extension-subscript";
import {
  Alert,
  Box,
  Button,
  Container,
  Paper,
  Stack,
  TextField,
  Typography,
} from "@mui/material";
import { LockOutlined } from "@mui/icons-material";
import { SizedImage } from "../features/editor/extensions/imageAttributes";
import { BlockId } from "../features/editor/extensions/blockId";
import { CellBackground } from "../features/editor/extensions/cellBackground";
import { LineHeight } from "../features/editor/extensions/lineHeight";
import { PageBreak } from "../features/editor/extensions/pageBreak";
import { ParagraphIndent } from "../features/editor/extensions/paragraphIndent";
import { HeadingNumbers } from "../features/editor/extensions/headingNumbers";
import { LoadingScreen } from "../components/LoadingScreen";
import { ApiError, formatDate } from "../lib/api";

type Shared = {
  title: string;
  content: string;
  updatedAt: string;
  serviceName: string;
};

/**
 * What someone outside the organisation sees when they open a share link.
 *
 * Deliberately not the editor. There is no sidebar, no comments, no AI, no
 * navigation to anything else — the page can render this one document and
 * nothing else exists from here. The same extensions are loaded so tables,
 * images and numbering look the way the author left them, but the editor is
 * not editable and carries no collaboration connection.
 */
export function SharedDocumentPage({ token }: { token: string }) {
  const [shared, setShared] = useState<Shared | null>(null);
  const [needsPassword, setNeedsPassword] = useState(false);
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);

  const open = async (withPassword: string) => {
    setError("");
    try {
      const response = await fetch(
        `/api/v1/public/documents/${encodeURIComponent(token)}`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(
            withPassword ? { password: withPassword } : {},
          ),
        },
      );
      const envelope = await response.json().catch(() => null);
      if (!response.ok) {
        const code = envelope?.error?.code ?? "";
        if (code === "LINK_PASSWORD_REQUIRED") {
          setNeedsPassword(true);
          return;
        }
        if (code === "LINK_PASSWORD_INVALID") {
          setNeedsPassword(true);
          setError("비밀번호가 올바르지 않습니다.");
          return;
        }
        throw new ApiError(
          response.status,
          code,
          envelope?.error?.message ?? "문서를 열 수 없습니다.",
        );
      }
      setShared(envelope.data as Shared);
      setNeedsPassword(false);
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "문서를 열 수 없습니다.",
      );
    }
  };

  useEffect(() => {
    void open("").finally(() => setLoading(false));
    // The token is the whole identity of this page; nothing else can change it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token]);

  const extensions = useMemo(
    () => [
      StarterKit.configure({ undoRedo: false }),
      Highlight.configure({ multicolor: true }),
      SizedImage.configure({ allowBase64: true, inline: false }),
      TableKit.configure({ table: { resizable: false } }),
      TaskList,
      TaskItem.configure({ nested: true }),
      TextAlign.configure({ types: ["heading", "paragraph"] }),
      TextStyleKit,
      // Without these the schema does not know the marks, and ProseMirror
      // discards the whole paragraph a single one appears in — not just the
      // mark. A Word document with m², H₂O or a footnote number lost the
      // paragraph carrying it the moment it was opened, and the next autosave
      // made that permanent.
      Superscript,
      Subscript,
      BlockId,
      CellBackground,
      LineHeight,
      PageBreak,
      ParagraphIndent,
      HeadingNumbers,
    ],
    [],
  );

  const editor = useEditor(
    { extensions, editable: false, content: parseContent(shared?.content) },
    [shared?.content],
  );

  if (loading) return <LoadingScreen />;

  if (needsPassword)
    return (
      <Centred>
        <Stack gap={2.5} alignItems="stretch">
          <Stack direction="row" gap={1.2} alignItems="center">
            <LockOutlined color="primary" />
            <Typography variant="h2">비밀번호가 필요합니다</Typography>
          </Stack>
          <Typography variant="body2" color="text.secondary">
            이 링크를 보낸 분에게 받은 비밀번호를 입력하세요.
          </Typography>
          <TextField
            type="password"
            label="비밀번호"
            value={password}
            autoFocus
            onChange={(event) => setPassword(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter" && password) {
                setSubmitting(true);
                void open(password).finally(() => setSubmitting(false));
              }
            }}
          />
          {error && <Alert severity="error">{error}</Alert>}
          <Button
            variant="contained"
            disabled={!password || submitting}
            onClick={() => {
              setSubmitting(true);
              void open(password).finally(() => setSubmitting(false));
            }}
          >
            {submitting ? "확인하는 중…" : "열기"}
          </Button>
        </Stack>
      </Centred>
    );

  if (error || !shared)
    return (
      <Centred>
        <Stack gap={2}>
          <Typography variant="h2">문서를 열 수 없습니다</Typography>
          <Typography variant="body2" color="text.secondary">
            {error ||
              "링크가 만료되었거나 해지되었을 수 있습니다. 보낸 분에게 문의하세요."}
          </Typography>
        </Stack>
      </Centred>
    );

  return (
    <Box sx={{ minHeight: "100dvh", bgcolor: "background.default", py: { xs: 3, md: 6 } }}>
      <Container maxWidth="md">
        <Stack gap={1} mb={3}>
          <Typography variant="h1">{shared.title}</Typography>
          <Typography variant="body2" color="text.secondary">
            {shared.serviceName} · {formatDate(shared.updatedAt)} 수정 · 읽기 전용
            공유
          </Typography>
        </Stack>
        <Paper variant="outlined" sx={{ p: { xs: 2.5, md: 5 } }}>
          {/* Tiptap puts its own .tiptap class on the rendered element, so
              the document styles apply here exactly as they do in the editor. */}
          <EditorContent editor={editor} />
        </Paper>
        <Typography
          variant="caption"
          color="text.secondary"
          display="block"
          textAlign="center"
          mt={3}
        >
          공유 링크로 열람 중입니다. 이 링크를 받은 사람은 누구나 이 문서를 읽을
          수 있습니다.
        </Typography>
      </Container>
    </Box>
  );
}

function Centred({ children }: { children: React.ReactNode }) {
  return (
    <Box sx={{ minHeight: "100dvh", display: "grid", placeItems: "center", p: 2.5 }}>
      <Paper variant="outlined" sx={{ p: { xs: 3, sm: 4 }, width: "100%", maxWidth: 420 }}>
        {children}
      </Paper>
    </Box>
  );
}

/**
 * The server sends the stored document as a JSON string. A document that will
 * not parse renders as empty rather than taking the page down with it.
 */
function parseContent(raw?: string) {
  if (!raw) return undefined;
  try {
    return JSON.parse(raw);
  } catch {
    return undefined;
  }
}
