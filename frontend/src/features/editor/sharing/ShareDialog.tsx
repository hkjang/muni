import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControl,
  InputLabel,
  MenuItem,
  Paper,
  Select,
  Stack,
  TextField,
  Typography,
} from "@mui/material";
import { api, errorMessage, jsonBody } from "../../../lib/api";
import type { DocumentItem } from "../../../types";
import type { Permission, UserSearch } from "../types";

export function ShareDialog({
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
