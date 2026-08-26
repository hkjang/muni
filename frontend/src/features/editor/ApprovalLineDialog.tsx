import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  IconButton,
  Stack,
  Switch,
  TextField,
  Typography,
} from "@mui/material";
import {
  ArrowDownwardOutlined,
  ArrowUpwardOutlined,
  DeleteOutline,
} from "@mui/icons-material";
import { api, errorMessage, jsonBody } from "../../lib/api";
import type { User } from "../../types";

/**
 * ApprovalLineDialog is where a 결재선 is decided before a document is sent.
 *
 * A Korean report is approved in order — 팀장, then 부서장, then 본부장 — and
 * only the person whose turn it is can act. muni could only express "any N of
 * the managers", which is a different thing and not the one an organisation
 * with an approval line is asking for.
 */
export function ApprovalLineDialog({
  open,
  onClose,
  documentId,
  onSubmitted,
}: {
  open: boolean;
  onClose: () => void;
  documentId: string;
  onSubmitted: () => void;
}) {
  const [line, setLine] = useState<User[]>([]);
  const [search, setSearch] = useState("");
  const [finalAt, setFinalAt] = useState(0);
  const [sequential, setSequential] = useState(true);

  const candidates = useQuery({
    queryKey: ["users-search", search],
    queryFn: () =>
      api<User[]>(`/api/v1/users/search?q=${encodeURIComponent(search)}&limit=15`),
    enabled: open,
  });

  const submit = useMutation({
    mutationFn: () =>
      api(`/api/v1/documents/${documentId}/workflow/submit`, {
        method: "POST",
        ...jsonBody(
          sequential && line.length > 0
            ? { approvers: line.map((person) => person.id), finalAt }
            : {},
        ),
      }),
    onSuccess: () => {
      setLine([]);
      setFinalAt(0);
      onSubmitted();
      onClose();
    },
  });

  const move = (index: number, direction: -1 | 1) => {
    const next = [...line];
    const target = index + direction;
    if (target < 0 || target >= next.length) return;
    [next[index], next[target]] = [next[target]!, next[index]!];
    setLine(next);
    // The 전결 marker follows the person it was set on.
    if (finalAt === index + 1) setFinalAt(target + 1);
    else if (finalAt === target + 1) setFinalAt(index + 1);
  };

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>검토 요청</DialogTitle>
      <DialogContent>
        {submit.error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {errorMessage(submit.error)}
          </Alert>
        )}
        <FormControlLabel
          control={
            <Switch
              checked={sequential}
              onChange={(event) => setSequential(event.target.checked)}
            />
          }
          label="결재선을 지정해 순서대로 결재"
        />
        {!sequential ? (
          <Typography variant="body2" color="text.secondary" mt={1}>
            워크스페이스의 관리자 누구나 검토할 수 있고, 관리자 설정에 정한
            인원만큼 승인되면 통과합니다.
          </Typography>
        ) : (
          <Stack gap={2} mt={1}>
            <Typography variant="body2" color="text.secondary">
              적은 순서대로 결재가 진행됩니다. 자기 차례가 된 사람만 결재할 수
              있고, 반려되면 그 뒤 차례는 진행되지 않습니다.
            </Typography>
            <Autocomplete
              options={(candidates.data ?? []).filter(
                (person) => !line.some((chosen) => chosen.id === person.id),
              )}
              value={null}
              blurOnSelect
              onInputChange={(_, value) => setSearch(value)}
              onChange={(_, value) => {
                if (value) setLine([...line, value]);
                setSearch("");
              }}
              getOptionLabel={(option) =>
                `${option.displayName} (${option.username})`
              }
              renderInput={(params) => (
                <TextField {...params} label="결재자 추가" placeholder="이름으로 찾기" />
              )}
            />
            {line.length === 0 ? (
              <Typography variant="body2" color="text.disabled">
                아직 결재자가 없습니다. 한 명 이상 추가해 주세요.
              </Typography>
            ) : (
              <Stack gap={1}>
                {line.map((person, index) => (
                  <Stack
                    key={person.id}
                    direction="row"
                    alignItems="center"
                    gap={1}
                    sx={{
                      border: "1px solid",
                      borderColor: "divider",
                      borderRadius: 1,
                      px: 1.25,
                      py: 0.75,
                    }}
                  >
                    <Chip size="small" label={index + 1} />
                    <Box sx={{ flex: 1, minWidth: 0 }}>
                      <Typography variant="body2" noWrap>
                        {person.displayName}
                      </Typography>
                    </Box>
                    <Chip
                      size="small"
                      variant={finalAt === index + 1 ? "filled" : "outlined"}
                      color={finalAt === index + 1 ? "primary" : "default"}
                      label="전결"
                      onClick={() =>
                        setFinalAt(finalAt === index + 1 ? 0 : index + 1)
                      }
                    />
                    <IconButton
                      size="small"
                      aria-label="결재선에서 위로 옮기기"
                      onClick={() => move(index, -1)}
                    >
                      <ArrowUpwardOutlined fontSize="small" />
                    </IconButton>
                    <IconButton
                      size="small"
                      aria-label="결재선에서 아래로 옮기기"
                      onClick={() => move(index, 1)}
                    >
                      <ArrowDownwardOutlined fontSize="small" />
                    </IconButton>
                    <IconButton
                      size="small"
                      aria-label="결재선에서 빼기"
                      onClick={() => {
                        setLine(line.filter((_, at) => at !== index));
                        setFinalAt(0);
                      }}
                    >
                      <DeleteOutline fontSize="small" />
                    </IconButton>
                  </Stack>
                ))}
                <Typography variant="caption" color="text.secondary">
                  전결로 표시한 단계에서 승인되면 그 뒤 차례는 건너뜁니다.
                  표시하지 않으면 마지막 차례가 최종 결재입니다.
                </Typography>
              </Stack>
            )}
          </Stack>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>취소</Button>
        <Button
          variant="contained"
          disabled={
            submit.isPending || (sequential && line.length === 0)
          }
          onClick={() => submit.mutate()}
        >
          검토 요청
        </Button>
      </DialogActions>
    </Dialog>
  );
}
