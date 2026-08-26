import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  Typography,
} from "@mui/material";
import { api, errorMessage, jsonBody } from "../../lib/api";
import type { DocumentItem, Folder, Workspace } from "../../types";
import { folderPaths } from "./folderTree";

/**
 * MoveDocumentDialog puts a document somewhere else.
 *
 * A folder could only be chosen at the moment a document was created, so one
 * filed in the wrong place stayed there and folders could not be reorganised
 * at all.
 */
export function MoveDocumentDialog({
  open,
  onClose,
  document,
}: {
  open: boolean;
  onClose: () => void;
  document: DocumentItem;
}) {
  const client = useQueryClient();
  const [workspaceId, setWorkspaceId] = useState(document.workspaceId);
  const [folderId, setFolderId] = useState(document.folderId ?? "");

  useEffect(() => {
    if (!open) return;
    setWorkspaceId(document.workspaceId);
    setFolderId(document.folderId ?? "");
  }, [document.folderId, document.workspaceId, open]);

  const workspaces = useQuery({
    queryKey: ["workspaces"],
    queryFn: () => api<Workspace[]>("/api/v1/workspaces"),
    enabled: open,
  });
  const folders = useQuery({
    queryKey: ["folders", workspaceId],
    queryFn: () => api<Folder[]>(`/api/v1/workspaces/${workspaceId}/folders`),
    enabled: open && Boolean(workspaceId),
  });

  const move = useMutation({
    mutationFn: () =>
      api<DocumentItem>(`/api/v1/documents/${document.id}/move`, {
        method: "POST",
        ...jsonBody({ workspaceId, folderId: folderId || null }),
      }),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: ["document", document.id] });
      void client.invalidateQueries({ queryKey: ["documents"] });
      void client.invalidateQueries({ queryKey: ["user-documents"] });
      onClose();
    },
  });

  const paths = folderPaths(folders.data ?? []);

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>문서 옮기기</DialogTitle>
      <DialogContent>
        {move.error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {errorMessage(move.error)}
          </Alert>
        )}
        <Stack gap={2} mt={1}>
          <FormControl size="small">
            <InputLabel>워크스페이스</InputLabel>
            <Select
              value={workspaceId}
              label="워크스페이스"
              onChange={(event) => {
                setWorkspaceId(event.target.value);
                // A folder belongs to one workspace, so the choice cannot
                // survive the move.
                setFolderId("");
              }}
            >
              {(workspaces.data ?? [])
                .filter((workspace) => workspace.role !== "VIEWER")
                .map((workspace) => (
                  <MenuItem key={workspace.id} value={workspace.id}>
                    {workspace.name}
                  </MenuItem>
                ))}
            </Select>
          </FormControl>
          <FormControl size="small">
            <InputLabel>폴더</InputLabel>
            <Select
              value={folderId}
              label="폴더"
              onChange={(event) => setFolderId(event.target.value)}
            >
              <MenuItem value="">워크스페이스 최상위</MenuItem>
              {paths.map((folder) => (
                <MenuItem key={folder.id} value={folder.id}>
                  {folder.path}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          {workspaceId !== document.workspaceId && (
            <Typography variant="body2" color="text.secondary">
              워크스페이스를 옮기면 문서를 볼 수 있는 사람이 달라집니다. 문서를
              열어 두고 있던 사람은 다시 연결됩니다.
            </Typography>
          )}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>취소</Button>
        <Button
          variant="contained"
          disabled={move.isPending}
          onClick={() => move.mutate()}
        >
          옮기기
        </Button>
      </DialogActions>
    </Dialog>
  );
}
