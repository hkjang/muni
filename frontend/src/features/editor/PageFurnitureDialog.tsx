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
  ToggleButton,
  ToggleButtonGroup,
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
  orientation,
  title,
  canEdit,
  onSave,
}: {
  open: boolean;
  onClose: () => void;
  header: string;
  footer: string;
  orientation: string;
  title: string;
  canEdit: boolean;
  onSave: (values: {
    pageHeader: string;
    pageFooter: string;
    pageOrientation: string;
  }) => Promise<void>;
}) {
  const [nextHeader, setNextHeader] = useState(header);
  const [nextFooter, setNextFooter] = useState(footer);
  const [nextOrientation, setNextOrientation] = useState(orientation);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) return;
    setNextHeader(header);
    setNextFooter(footer);
    setNextOrientation(orientation || "PORTRAIT");
  }, [open, header, footer, orientation]);

  const landscape = nextOrientation === "LANDSCAPE";

  const save = async () => {
    setSaving(true);
    try {
      await onSave({
        pageHeader: nextHeader,
        pageFooter: nextFooter,
        pageOrientation: nextOrientation,
      });
      onClose();
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>페이지 설정</DialogTitle>
      <DialogContent>
        <Stack gap={2.5} mt={1}>
          <Typography variant="body2" color="text.secondary">
            인쇄하거나 PDF·Word로 내보낼 때 적용됩니다. 화면에서 편집할 때는
            보이지 않습니다.
          </Typography>

          <Box>
            <Typography variant="body2" fontWeight={640} mb={0.8}>
              용지 방향
            </Typography>
            <ToggleButtonGroup
              size="small"
              exclusive
              value={nextOrientation}
              onChange={(_, value) => value && setNextOrientation(value)}
              disabled={!canEdit}
            >
              <ToggleButton value="PORTRAIT">세로</ToggleButton>
              <ToggleButton value="LANDSCAPE">가로</ToggleButton>
            </ToggleButtonGroup>
            <Typography variant="caption" color="text.secondary" display="block" mt={0.6}>
              {landscape
                ? "가로 A4 (297 × 210mm). 넓은 표가 잘리지 않습니다."
                : "세로 A4 (210 × 297mm)."}
            </Typography>
          </Box>

          <Typography variant="body2" color="text.secondary">
            아래 두 줄은 <b>모든 페이지</b>에 찍힙니다. 대외비 표시, 부서명,
            문서번호를 넣는 자리입니다.
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
                // The preview is the shape of the page it describes.
                maxWidth: landscape ? "100%" : 260,
                mx: landscape ? 0 : "auto",
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
