import { useMemo, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
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
  Slider,
  Stack,
  TextField,
  Typography,
} from "@mui/material";
import { api, errorMessage, jsonBody } from "../../../lib/api";
import {
  audiences,
  details,
  purposes,
  suggestSlideCount,
  type PresentationLink,
} from "./types";

/**
 * PresentationDialog collects the few choices that decide whether a deck is
 * useful. Audience, purpose and the time available matter more than anything
 * else: a forty page document is eight slides for a ten minute executive
 * briefing and thirty for an hour of technical review.
 */
export function PresentationDialog({
  open,
  onClose,
  documentId,
  documentTitle,
}: {
  open: boolean;
  onClose: () => void;
  documentId: string;
  documentTitle: string;
}) {
  const client = useQueryClient();
  const [title, setTitle] = useState(documentTitle);
  const [audience, setAudience] = useState(audiences[0]!.value);
  const [purpose, setPurpose] = useState(purposes[0]!.value);
  const [detail, setDetail] = useState(details[0]!.value);
  const [minutes, setMinutes] = useState(10);
  const [slideCount, setSlideCount] = useState<number | null>(null);

  const suggested = useMemo(
    () => suggestSlideCount(minutes, detail),
    [minutes, detail],
  );
  const slides = slideCount ?? suggested;

  const create = useMutation({
    mutationFn: () =>
      api<PresentationLink>(`/api/v1/documents/${documentId}/presentations`, {
        method: "POST",
        ...jsonBody({
          title: title.trim() || documentTitle,
          audience,
          purpose,
          detail,
          minutes,
          slideCount: slides,
          tone: audience === "경영진" ? "executive" : "professional",
        }),
      }),
    onSuccess: () => {
      void client.invalidateQueries({
        queryKey: ["presentations", documentId],
      });
      onClose();
    },
  });

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>발표자료 만들기</DialogTitle>
      <DialogContent sx={{ display: "grid", gap: 2, pt: "10px!important" }}>
        {create.error && (
          <Alert severity="error">{errorMessage(create.error)}</Alert>
        )}
        <Typography variant="body2" color="text.secondary">
          문서의 제목·목록·표·수치 구조를 그대로 전달해 발표자료를 만듭니다.
          만들어진 자료는 Ptium에서 이어서 편집할 수 있습니다.
        </Typography>
        <TextField
          label="발표자료 제목"
          value={title}
          onChange={(event) => setTitle(event.target.value)}
          inputProps={{ maxLength: 200 }}
        />
        <Stack direction="row" gap={2}>
          <FormControl size="small" fullWidth>
            <InputLabel>대상</InputLabel>
            <Select
              value={audience}
              label="대상"
              onChange={(event) => setAudience(event.target.value)}
            >
              {audiences.map((item) => (
                <MenuItem key={item.value} value={item.value}>
                  {item.label}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <FormControl size="small" fullWidth>
            <InputLabel>목적</InputLabel>
            <Select
              value={purpose}
              label="목적"
              onChange={(event) => setPurpose(event.target.value)}
            >
              {purposes.map((item) => (
                <MenuItem key={item.value} value={item.value}>
                  {item.label}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <FormControl size="small" fullWidth>
            <InputLabel>상세 수준</InputLabel>
            <Select
              value={detail}
              label="상세 수준"
              onChange={(event) => {
                setDetail(event.target.value);
                setSlideCount(null);
              }}
            >
              {details.map((item) => (
                <MenuItem key={item.value} value={item.value}>
                  {item.label}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        </Stack>
        <div>
          <Typography variant="body2" gutterBottom>
            발표 시간 {minutes}분
          </Typography>
          <Slider
            value={minutes}
            min={5}
            max={60}
            step={5}
            marks
            valueLabelDisplay="auto"
            onChange={(_, value) => {
              setMinutes(value as number);
              setSlideCount(null);
            }}
          />
        </div>
        <div>
          <Typography variant="body2" gutterBottom>
            슬라이드 {slides}장
            {slideCount === null && ` (발표 시간에 맞춰 추천한 값입니다)`}
          </Typography>
          <Slider
            value={slides}
            min={3}
            max={50}
            valueLabelDisplay="auto"
            onChange={(_, value) => setSlideCount(value as number)}
          />
        </div>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>취소</Button>
        <Button
          variant="contained"
          disabled={create.isPending}
          onClick={() => create.mutate()}
        >
          {create.isPending ? "만드는 중…" : "만들기"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
