import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Autocomplete, Box, Chip, Stack, TextField } from "@mui/material";
import { LocalOfferOutlined } from "@mui/icons-material";
import { api, jsonBody } from "../../lib/api";
import type { DocumentItem } from "../../types";

type WorkspaceTag = { id: string; name: string; color: string; documents: number };

/**
 * DocumentTags puts labels on a document — 상반기, 대외비, 부서 검토 — which
 * is how a workspace groups documents across folders.
 *
 * The tables and the search that reads them have existed from the start with
 * no way to put a tag on anything, so search could find a tag nobody could
 * ever apply.
 */
export function DocumentTags({
  document,
  canEdit,
}: {
  document: DocumentItem;
  canEdit: boolean;
}) {
  const client = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<string[]>(document.tags ?? []);

  useEffect(() => {
    if (!editing) setDraft(document.tags ?? []);
  }, [document.tags, editing]);

  // The workspace's existing tags are suggestions, so the same idea does not
  // become three tags that differ by a space.
  const suggestions = useQuery({
    queryKey: ["workspace-tags", document.workspaceId],
    queryFn: () =>
      api<WorkspaceTag[]>(`/api/v1/workspaces/${document.workspaceId}/tags`),
    enabled: editing,
  });

  const save = useMutation({
    mutationFn: (tags: string[]) =>
      api<{ tags: string[] }>(`/api/v1/documents/${document.id}/tags`, {
        method: "PUT",
        ...jsonBody({ tags }),
      }),
    onSuccess: () => {
      setEditing(false);
      void client.invalidateQueries({ queryKey: ["document", document.id] });
      void client.invalidateQueries({ queryKey: ["workspace-tags"] });
    },
  });

  if (!editing) {
    const tags = document.tags ?? [];
    if (tags.length === 0 && !canEdit) return null;
    return (
      <Stack direction="row" gap={0.5} alignItems="center" flexWrap="wrap">
        <LocalOfferOutlined fontSize="small" color="disabled" />
        {tags.map((tag) => (
          <Chip key={tag} size="small" label={tag} />
        ))}
        {canEdit && (
          <Chip
            size="small"
            variant="outlined"
            label={tags.length === 0 ? "태그 추가" : "편집"}
            onClick={() => setEditing(true)}
          />
        )}
      </Stack>
    );
  }

  return (
    <Box sx={{ maxWidth: 520 }}>
      <Autocomplete
        multiple
        freeSolo
        autoHighlight
        openOnFocus
        size="small"
        value={draft}
        options={(suggestions.data ?? []).map((tag) => tag.name)}
        onChange={(_, value) => setDraft(value as string[])}
        onBlur={() => save.mutate(draft)}
        renderInput={(params) => (
          <TextField
            {...params}
            autoFocus
            label="태그"
            placeholder="입력 후 Enter"
            helperText="워크스페이스의 기존 태그가 제안됩니다. 벗어나면 저장됩니다."
            onKeyDown={(event) => {
              if (event.key === "Escape") {
                setDraft(document.tags ?? []);
                setEditing(false);
              }
            }}
          />
        )}
      />
    </Box>
  );
}
