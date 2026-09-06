import { useCallback, useEffect, useRef, useState } from "react";
import type { DragEvent, ReactNode } from "react";
import {
  Alert,
  Box,
  CircularProgress,
  Snackbar,
  Typography,
} from "@mui/material";
import UploadFileOutlinedIcon from "@mui/icons-material/UploadFileOutlined";
import type { Editor } from "@tiptap/react";
import { droppableFilesLabel } from "../extensions/fileDrop";
import type { ImportProgress } from "./useFileImport";

function carriesFiles(event: DragEvent): boolean {
  return Array.from(event.dataTransfer?.types ?? []).includes("Files");
}

/**
 * FileDropZone wraps the page so a file can be dropped anywhere on it — the
 * margin around the text as well as the text — and says so while one is
 * being dragged over. The editor takes a drop on the text and works out the
 * position; a drop on the margin goes to the end of the document.
 *
 * It also stops the browser from opening a file dropped anywhere else on
 * the screen while the document is open, which would navigate away from it
 * mid-edit.
 */
export function FileDropZone({
  editor,
  enabled,
  progress,
  onFiles,
  onDismiss,
  children,
  sx,
  className,
}: {
  editor: Editor | null;
  enabled: boolean;
  progress: ImportProgress;
  onFiles: (files: File[], position: number) => void;
  onDismiss: () => void;
  children: ReactNode;
  className?: string;
  sx?: object;
}) {
  // Enter and leave fire for every child the pointer crosses, so the state
  // is a count: over the zone while it is above zero.
  const depth = useRef(0);
  const [dragging, setDragging] = useState(false);

  useEffect(() => {
    const swallow = (event: Event) => {
      const transfer = (event as unknown as DragEvent).dataTransfer;
      if (transfer && Array.from(transfer.types).includes("Files"))
        event.preventDefault();
    };
    window.addEventListener("dragover", swallow);
    window.addEventListener("drop", swallow);
    return () => {
      window.removeEventListener("dragover", swallow);
      window.removeEventListener("drop", swallow);
    };
  }, []);

  const onDragEnter = useCallback((event: DragEvent) => {
    if (!carriesFiles(event)) return;
    depth.current += 1;
    setDragging(true);
  }, []);
  const onDragLeave = useCallback((event: DragEvent) => {
    if (!carriesFiles(event)) return;
    depth.current = Math.max(0, depth.current - 1);
    if (depth.current === 0) setDragging(false);
  }, []);
  const onDragOver = useCallback((event: DragEvent) => {
    if (!carriesFiles(event)) return;
    event.preventDefault();
    if (event.dataTransfer) event.dataTransfer.dropEffect = "copy";
  }, []);
  const onDrop = useCallback(
    (event: DragEvent) => {
      depth.current = 0;
      setDragging(false);
      if (!carriesFiles(event)) return;
      const files = Array.from(event.dataTransfer?.files ?? []);
      // A drop on the text itself reaches the editor's own handler first
      // and is already taken; only a drop on the margin arrives here.
      if (
        !editor ||
        event.defaultPrevented ||
        editor.view.dom.contains(event.target as Node)
      )
        return;
      event.preventDefault();
      if (files.length > 0) onFiles(files, editor.state.doc.content.size);
    },
    [editor, onFiles],
  );

  return (
    <Box
      className={className}
      sx={sx}
      onDragEnter={onDragEnter}
      onDragLeave={onDragLeave}
      onDragOver={onDragOver}
      onDrop={onDrop}
    >
      {children}
      {dragging && (
        <Box
          className="muni-no-print"
          sx={{
            position: "absolute",
            inset: 12,
            zIndex: 5,
            pointerEvents: "none",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            borderRadius: 3,
            border: "2px dashed",
            borderColor: enabled ? "primary.main" : "text.disabled",
            bgcolor: enabled
              ? "rgba(81,81,198,.06)"
              : "rgba(120,120,120,.08)",
          }}
        >
          <Box
            sx={{
              px: 3,
              py: 2,
              borderRadius: 2,
              bgcolor: "background.paper",
              boxShadow: 3,
              textAlign: "center",
            }}
          >
            <UploadFileOutlinedIcon
              color={enabled ? "primary" : "disabled"}
              sx={{ fontSize: 34 }}
            />
            <Typography variant="subtitle1" fontWeight={600}>
              {enabled
                ? "여기에 놓으면 그 자리에 문서 내용을 넣습니다"
                : "편집 모드에서만 파일 내용을 넣을 수 있습니다"}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              {droppableFilesLabel}
            </Typography>
          </Box>
        </Box>
      )}
      <Snackbar
        open={progress.state !== "idle"}
        autoHideDuration={progress.state === "done" ? 4000 : null}
        onClose={(_event, reason) => {
          if (reason !== "clickaway" && progress.state !== "working")
            onDismiss();
        }}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert
          severity={
            progress.state === "error"
              ? "error"
              : progress.state === "done"
                ? "success"
                : "info"
          }
          onClose={progress.state === "working" ? undefined : onDismiss}
          icon={
            progress.state === "working" ? (
              <CircularProgress size={18} sx={{ mt: 0.25 }} />
            ) : undefined
          }
          sx={{ maxWidth: 560 }}
        >
          {progress.state === "working"
            ? `${progress.name} 읽는 중… (${progress.index}/${progress.total})`
            : progress.state === "idle"
              ? ""
              : progress.message}
        </Alert>
      </Snackbar>
    </Box>
  );
}
