import { useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Stack,
  TextField,
  Typography,
} from "@mui/material";

/**
 * 머리글과 바닥글 — the one line each that prints on every page.
 *
 * A Korean office document carries its classification there: 대외비, the
 * department, the document number. muni had nowhere to put it, and importing
 * a Word document that had one dropped it without a word.
 *
 * One line, because that is what a page header is. Anything that needs to wrap
 * is part of the document.
 */
export function PageFurnitureDialog({
  open,
  onClose,
  header,
  footer,
  title,
  canEdit,
  onSave,
}: {
  open: boolean;
  onClose: () => void;
  header: string;
  footer: string;
  title: string;
  canEdit: boolean;
  onSave: (values: { pageHeader: string; pageFooter: string }) => Promise<void>;
}) {
  const [nextHeader, setNextHeader] = useState(header);
  const [nextFooter, setNextFooter] = useState(footer);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) return;
    setNextHeader(header);
    setNextFooter(footer);
  }, [open, header, footer]);

  const save = async () => {
    setSaving(true);
    try {
      await onSave({ pageHeader: nextHeader, pageFooter: nextFooter });
      onClose();
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>머리글과 바닥글</DialogTitle>
      <DialogContent>
        <Stack gap={2.5} mt={1}>
          <Typography variant="body2" color="text.secondary">
            인쇄하거나 PDF·Word로 내보낼 때 <b>모든 페이지</b>에 찍히는 한 줄입니다.
            대외비 표시, 부서명, 문서번호를 넣는 자리입니다. 화면에서 편집할 때는
            보이지 않습니다.
          </Typography>

          <TextField
            label="머리글 (페이지 위)"
            value={nextHeader}
            onChange={(event) => setNextHeader(event.target.value)}
            placeholder="예: 기획조정실 · 대외비"
            disabled={!canEdit}
            inputProps={{ maxLength: 200 }}
            fullWidth
          />
          <TextField
            label="바닥글 (페이지 아래 왼쪽)"
            value={nextFooter}
            onChange={(event) => setNextFooter(event.target.value)}
            placeholder={`비우면 문서 제목이 들어갑니다 — ${title}`}
            disabled={!canEdit}
            inputProps={{ maxLength: 200 }}
            helperText="페이지 번호는 오른쪽에 자동으로 붙습니다."
            fullWidth
          />

          <Box>
            <Typography variant="caption" color="text.secondary">
              인쇄 미리보기
            </Typography>
            <Box
              sx={{
                mt: 0.8,
                border: 1,
                borderColor: "divider",
                borderRadius: 1,
                p: 1.5,
                bgcolor: "action.hover",
                fontSize: 12,
                color: "text.secondary",
              }}
            >
              <Box sx={{ textAlign: "right", minHeight: 18 }}>
                {nextHeader.trim() || <span style={{ opacity: 0.4 }}>(머리글 없음)</span>}
              </Box>
              <Box
                sx={{
                  my: 1,
                  py: 2,
                  textAlign: "center",
                  border: "1px dashed",
                  borderColor: "divider",
                  borderRadius: 0.5,
                  opacity: 0.5,
                }}
              >
                본문
              </Box>
              <Box sx={{ display: "flex", justifyContent: "space-between" }}>
                <span>{nextFooter.trim() || title}</span>
                <span>1 / 3</span>
              </Box>
            </Box>
          </Box>

          {(nextHeader.includes("\n") || nextFooter.includes("\n")) && (
            <Alert severity="info">
              줄바꿈은 한 칸 띄어쓰기로 바뀝니다. 머리글은 한 줄입니다.
            </Alert>
          )}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>닫기</Button>
        {canEdit && (
          <Button variant="contained" disabled={saving} onClick={() => void save()}>
            {saving ? "저장 중…" : "저장"}
          </Button>
        )}
      </DialogActions>
    </Dialog>
  );
}
