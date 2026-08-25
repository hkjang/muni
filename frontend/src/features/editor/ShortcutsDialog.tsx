import {
  Box,
  Dialog,
  DialogContent,
  DialogTitle,
  Grid,
  Stack,
  Typography,
} from "@mui/material";

const groups: { title: string; items: [string, string][] }[] = [
  {
    title: "문서",
    items: [
      ["Ctrl S", "지금 저장"],
      ["Ctrl P", "인쇄"],
      ["Ctrl F", "찾기"],
      ["Ctrl H", "찾아 바꾸기"],
      ["Ctrl /", "단축키 보기"],
      ["Ctrl \\", "문서 개요 열고 닫기"],
    ],
  },
  {
    title: "서식",
    items: [
      ["Ctrl B", "굵게"],
      ["Ctrl I", "기울임"],
      ["Ctrl U", "밑줄"],
      ["Ctrl Shift X", "취소선"],
      ["Ctrl K", "링크"],
      ["Ctrl Shift 8", "글머리 기호"],
      ["Ctrl Shift 7", "번호 매기기"],
    ],
  },
  {
    title: "문단",
    items: [
      ["Ctrl Alt 1~6", "제목 1~6"],
      ["Ctrl Alt 0", "본문"],
      ["Tab", "목록 한 단계 안으로"],
      ["Shift Tab", "목록 한 단계 밖으로"],
      ["Ctrl Z", "실행 취소"],
      ["Ctrl Shift Z", "다시 실행"],
    ],
  },
  {
    title: "입력하면 바뀝니다",
    items: [
      ["# 공백", "제목 1"],
      ["## 공백", "제목 2"],
      ["- 공백", "글머리 기호"],
      ["1. 공백", "번호 매기기"],
      ["> 공백", "인용"],
      ["``` ", "코드 블록"],
    ],
  },
];

/** ShortcutsDialog is the list Google Docs opens on Ctrl+/. */
export function ShortcutsDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="md">
      <DialogTitle>단축키</DialogTitle>
      <DialogContent>
        <Grid container spacing={3}>
          {groups.map((group) => (
            <Grid key={group.title} size={{ xs: 12, sm: 6 }}>
              <Typography variant="h3" mb={1.2}>
                {group.title}
              </Typography>
              <Stack gap={0.9}>
                {group.items.map(([keys, label]) => (
                  <Stack key={label} direction="row" alignItems="center" gap={1.5}>
                    <Box sx={{ flex: 1 }}>
                      <Typography variant="body2">{label}</Typography>
                    </Box>
                    <Stack direction="row" gap={0.4}>
                      {keys.split(" ").map((key, index) => (
                        <Box
                          key={`${key}-${index}`}
                          sx={{
                            border: "1px solid",
                            borderColor: "divider",
                            borderRadius: 0.8,
                            px: 0.7,
                            py: 0.1,
                            fontSize: 12,
                            fontFamily: "monospace",
                            bgcolor: "action.hover",
                            whiteSpace: "nowrap",
                          }}
                        >
                          {key}
                        </Box>
                      ))}
                    </Stack>
                  </Stack>
                ))}
              </Stack>
            </Grid>
          ))}
        </Grid>
        <Typography variant="body2" color="text.secondary" mt={3}>
          macOS에서는 Ctrl 대신 ⌘를 사용합니다.
        </Typography>
      </DialogContent>
    </Dialog>
  );
}
