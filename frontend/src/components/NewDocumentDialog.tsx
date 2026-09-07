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
  TextField,
} from "@mui/material";
import { useNavigate } from "react-router-dom";
import { api, errorMessage, jsonBody } from "../lib/api";
import type { DocumentItem, Folder, Template, Workspace } from "../types";
import { UploadFileOutlined } from "@mui/icons-material";
import { TemplateManagerDialog } from "../features/templates/TemplateManagerDialog";
import { confirmsInput } from "../lib/keyboard";
import { acceptedDocumentFiles } from "../features/editor/extensions/fileDrop";

export function NewDocumentDialog({
  open,
  onClose,
  initialWorkspaceId,
  initialFolderId,
  initialFiles,
}: {
  open: boolean;
  onClose: () => void;
  initialWorkspaceId?: string;
  initialFolderId?: string;
  /** Files dropped on the screen behind this dialog. */
  initialFiles?: File[];
}) {
  const [title, setTitle] = useState("");
  const [workspaceId, setWorkspaceId] = useState(initialWorkspaceId ?? "");
  const [folderId, setFolderId] = useState(initialFolderId ?? "");
  const [files, setFiles] = useState<File[]>([]);
  const file = files[0] ?? null;
  const [templateId, setTemplateId] = useState("");
  const [managingTemplates, setManagingTemplates] = useState(false);
  const navigate = useNavigate();
  const client = useQueryClient();
  const { data: workspaces = [] } = useQuery({
    queryKey: ["workspaces"],
    queryFn: () => api<Workspace[]>("/api/v1/workspaces"),
  });
  const { data: folders = [] } = useQuery({
    queryKey: ["folders", workspaceId],
    queryFn: () => api<Folder[]>(`/api/v1/workspaces/${workspaceId}/folders`),
    enabled: Boolean(workspaceId),
  });
  const { data: templates = [] } = useQuery({
    queryKey: ["templates", workspaceId],
    queryFn: () =>
      api<Template[]>(`/api/v1/workspaces/${workspaceId}/templates`),
    enabled: Boolean(workspaceId),
  });
  useEffect(() => {
    if (!open) return;
    if (initialWorkspaceId) setWorkspaceId(initialWorkspaceId);
    setFolderId(initialFolderId ?? "");
    setTemplateId("");
    // A file picked last time must not come back with the dialog.
    setFiles(initialFiles ?? []);
    setTitle("");
  }, [initialFiles, initialFolderId, initialWorkspaceId, open]);
  useEffect(() => {
    if (open && !workspaceId) {
      setWorkspaceId(initialWorkspaceId ?? workspaces[0]?.id ?? "");
    }
  }, [open, workspaceId, initialWorkspaceId, workspaces]);
  const mutation = useMutation({
    mutationFn: async (): Promise<DocumentItem[]> => {
      if (files.length > 0) {
        // Several files make several documents, in the order they were
        // dropped, and one that cannot be read stops the rest rather than
        // leaving a half-finished import unexplained.
        const created: DocumentItem[] = [];
        for (const [index, item] of files.entries()) {
          const form = new FormData();
          form.set("workspaceId", workspaceId);
          if (folderId) form.set("folderId", folderId);
          form.set("title", files.length === 1 ? title : "");
          form.set("file", item);
          created.push(
            await api<DocumentItem>("/api/v1/import", {
              method: "POST",
              body: form,
            }),
          );
          void index;
        }
        return created;
      }
      return [
        await api<DocumentItem>("/api/v1/documents", {
          method: "POST",
          ...jsonBody({
            workspaceId,
            folderId: folderId || null,
            title: title.trim() || "제목 없는 문서",
            templateId: templateId || null,
          }),
        }),
      ];
    },
    onSuccess: (created) => {
      void client.invalidateQueries({ queryKey: ["documents"] });
      void client.invalidateQueries({ queryKey: ["user-documents"] });
      setFiles([]);
      onClose();
      // One document is what you asked for and you go to it. Several are a
      // batch, and the list you are looking at is where they landed.
      if (created.length === 1 && created[0]) navigate(`/docs/${created[0].id}`);
    },
  });
  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>
        {files.length > 1
          ? `파일 ${files.length}개에서 문서 만들기`
          : file
            ? "파일에서 문서 가져오기"
            : "새 문서 만들기"}
      </DialogTitle>
      <DialogContent sx={{ display: "grid", gap: 2, pt: "10px!important" }}>
        {mutation.error && (
          <Alert severity="error">{errorMessage(mutation.error)}</Alert>
        )}
        <TextField
          autoFocus
          disabled={files.length > 1}
          helperText={
            files.length > 1 ? "제목은 각 파일 이름을 씁니다." : undefined
          }
          label={file ? "문서 제목 (선택)" : "문서 제목"}
          value={title}
          onChange={(event) => setTitle(event.target.value)}
          onKeyDown={(event) => {
            if (confirmsInput(event) && workspaceId) mutation.mutate();
          }}
          inputProps={{ maxLength: 240 }}
        />
        <FormControl size="small">
          <InputLabel>워크스페이스</InputLabel>
          <Select
            value={workspaceId}
            label="워크스페이스"
            onChange={(event) => {
              setWorkspaceId(event.target.value);
              setFolderId("");
              setTemplateId("");
            }}
          >
            {workspaces
              .filter((w) => w.role !== "VIEWER")
              .map((workspace) => (
                <MenuItem key={workspace.id} value={workspace.id}>
                  {workspace.name}
                </MenuItem>
              ))}
          </Select>
        </FormControl>
        {!file && templates.length > 0 && (
          <Stack direction="row" gap={1} alignItems="flex-end">
            <FormControl size="small" sx={{ flex: 1 }}>
              <InputLabel>서식 (선택)</InputLabel>
              <Select
                value={templateId}
                label="서식 (선택)"
                onChange={(event) => setTemplateId(event.target.value)}
              >
                <MenuItem value="">빈 문서</MenuItem>
                {templates.map((template) => (
                  <MenuItem key={template.id} value={template.id}>
                    {template.name}
                    {template.workspaceId ? "" : " · 공용"}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
            {/* Where the list is read is where its clutter is noticed, so this
                is where clearing it belongs. */}
            <Button size="small" onClick={() => setManagingTemplates(true)}>
              관리
            </Button>
          </Stack>
        )}
        <TemplateManagerDialog
          open={managingTemplates}
          onClose={() => setManagingTemplates(false)}
          workspaceId={workspaceId}
        />
        <FormControl size="small">
          <InputLabel>폴더 (선택)</InputLabel>
          <Select
            value={folderId}
            label="폴더 (선택)"
            onChange={(event) => setFolderId(event.target.value)}
          >
            <MenuItem value="">루트</MenuItem>
            {folders.map((folder) => (
              <MenuItem key={folder.id} value={folder.id}>
                {folder.name}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
        <Button
          component="label"
          variant="outlined"
          startIcon={<UploadFileOutlined />}
        >
          {files.length > 1
            ? files.map((item) => item.name).join(", ")
            : file
              ? file.name
              : "PDF · DOCX · HWP · HWPX · Markdown · TXT · HTML 가져오기"}
          <input
            hidden
            multiple
            type="file"
            accept={acceptedDocumentFiles}
            onChange={(event) => setFiles(Array.from(event.target.files ?? []))}
          />
        </Button>
        {file && (
          <Button size="small" color="inherit" onClick={() => setFiles([])}>
            파일 선택 취소
          </Button>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>취소</Button>
        <Button
          variant="contained"
          disabled={!workspaceId || mutation.isPending}
          onClick={() => mutation.mutate()}
        >
          {files.length > 1
            ? `${files.length}개 가져오기`
            : file
              ? "가져오기"
              : "문서 만들기"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
