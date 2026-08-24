import { CircularProgress, Stack, Typography } from "@mui/material";
import { CloudDoneOutlined, CloudOffOutlined } from "@mui/icons-material";

export function EditorStatus({
  state,
}: {
  state: "saved" | "saving" | "offline" | "error";
}) {
  const values = {
    saved: [<CloudDoneOutlined key="i" fontSize="small" />, "저장됨"],
    saving: [<CircularProgress key="i" size={16} />, "저장 중"],
    offline: [<CloudOffOutlined key="i" fontSize="small" />, "오프라인"],
    error: [
      <CloudOffOutlined key="i" color="error" fontSize="small" />,
      "저장 오류",
    ],
  } as const;
  return (
    <Stack
      direction="row"
      gap={0.5}
      alignItems="center"
      color={state === "error" ? "error.main" : "text.secondary"}
      sx={{ display: { xs: "none", sm: "flex" } }}
    >
      {values[state][0]}
      <Typography variant="caption">{values[state][1]}</Typography>
    </Stack>
  );
}
