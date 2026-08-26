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

export function NewDocumentDialog({
  open,
  onClose,
  initialWorkspaceId,
  initialFolderId,
}: {
  open: boolean;
  onClose: () => void;
  initialWorkspaceId?: string;
  initialFolderId?: string;
}) {
  const [title, setTitle] = useState("");
  const [workspaceId, setWorkspaceId] = useState(initialWorkspaceId ?? "");
  const [folderId, setFolderId] = useState(initialFolderId ?? "");
  const [file, setFile] = useState<File | null>(null);
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
    queryFn: () => api<Template[]>(`/api/v1/workspaces/${workspaceId}/templates`),
    enabled: Boolean(workspaceId),
  });
  useEffect(() => {
    if (!open) return;
    if (initialWorkspaceId) setWorkspaceId(initialWorkspaceId);
    setFolderId(initialFolderId ?? "");
    setTemplateId("");
  }, [initialFolderId, initialWorkspaceId, open]);
  useEffect(() => {
    if (open && !workspaceId) {
      setWorkspaceId(initialWorkspaceId ?? workspaces[0]?.id ?? "");
    }
  }, [open, workspaceId, initialWorkspaceId, workspaces]);
  const mutation = useMutation({
    mutationFn: () => {
      if (file) {
        const form = new FormData();
        form.set("workspaceId", workspaceId);
        if (folderId) form.set("folderId", folderId);
        form.set("title", title);
        form.set("file", file);
        return api<DocumentItem>("/api/v1/import", {
          method: "POST",
          body: form,
        });
      }
      return api<DocumentItem>("/api/v1/documents", {
        method: "POST",
        ...jsonBody({
          workspaceId,
          folderId: folderId || null,
          title: title.trim() || "제목 없는 문서",
          templateId: templateId || null,
        }),
      });
    },
    onSuccess: (document) => {
      void client.invalidateQueries({ queryKey: ["documents"] });
      void client.invalidateQueries({ queryKey: ["user-documents"] });
      setFile(null);
      onClose();
      navigate(`/docs/${document.id}`);
    },
  });
  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>
        {file ? "파일에서 문서 가져오기" : "새 문서 만들기"}
      </DialogTitle>
      <DialogContent sx={{ display: "grid", gap: 2, pt: "10px!important" }}>
        {mutation.error && (
          <Alert severity="error">{errorMessage(mutation.error)}</Alert>
        )}
        <TextField
          autoFocus
          label={file ? "문서 제목 (선택)" : "문서 제목"}
          value={title}
          onChange={(event) => setTitle(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter" && workspaceId) mutation.mutate();
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
          {file ? file.name : "PDF · DOCX · Markdown · TXT · HTML 가져오기"}
          <input
            hidden
            type="file"
            accept=".pdf,.docx,.md,.markdown,.txt,.html,.htm"
            onChange={(event) => setFile(event.target.files?.[0] ?? null)}
          />
        </Button>
        {file && (
          <Button size="small" color="inherit" onClick={() => setFile(null)}>
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
          {file ? "가져오기" : "문서 만들기"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
