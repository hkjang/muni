import { useCallback, useEffect, useRef, useState } from "react";
import type { DragEvent, ReactNode } from "react";
import { Box, Typography } from "@mui/material";
import UploadFileOutlinedIcon from "@mui/icons-material/UploadFileOutlined";
import {
  droppableFilesLabel,
  fileKind,
} from "../features/editor/extensions/fileDrop";

function carriesFiles(event: DragEvent): boolean {
  return Array.from(event.dataTransfer?.types ?? []).includes("Files");
}

/**
 * FileDropTarget makes a whole screen take a dropped file.
 *
 * The editor has taken files anywhere on the page for a while; the lists that
 * lead to it did not, and a file dropped on them was opened by the browser
 * instead — which throws away the screen you were on. Here a drop anywhere on
 * the list starts the same import the "새 문서" button offers.
 */
export function FileDropTarget({
  enabled,
  onFiles,
  children,
  sx,
  hint,
}: {
  enabled: boolean;
  onFiles: (files: File[]) => void;
  children: ReactNode;
  sx?: object;
  /** What the overlay says a drop will do. */
  hint: string;
}) {
  // Enter and leave fire for every child the pointer crosses, so being over
  // the zone is a count rather than a flag.
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
      event.preventDefault();
      if (!enabled) return;
      // A picture is not a document; here there is nothing to put it in.
      const files = Array.from(event.dataTransfer?.files ?? []).filter(
        (file) => fileKind(file) === "document",
      );
      if (files.length > 0) onFiles(files);
    },
    [enabled, onFiles],
  );

  return (
    <Box
      sx={{ position: "relative", ...sx }}
      onDragEnter={onDragEnter}
      onDragLeave={onDragLeave}
      onDragOver={onDragOver}
      onDrop={onDrop}
    >
      {children}
      {dragging && (
        <Box
          sx={{
            position: "absolute",
            inset: 8,
            zIndex: 8,
            pointerEvents: "none",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            borderRadius: 3,
            border: "2px dashed",
            borderColor: enabled ? "primary.main" : "text.disabled",
            bgcolor: enabled ? "rgba(81,81,198,.06)" : "rgba(120,120,120,.08)",
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
              {enabled ? hint : "이 화면으로는 가져올 수 없습니다"}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              {droppableFilesLabel.replace(" · 그림", "")}
            </Typography>
          </Box>
        </Box>
      )}
    </Box>
  );
}
