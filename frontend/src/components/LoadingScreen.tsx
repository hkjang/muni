import { Box, CircularProgress, Typography } from "@mui/material";
import { Brand } from "./Brand";

export function LoadingScreen({
  label = "muni를 준비하고 있습니다",
}: {
  label?: string;
}) {
  return (
    <Box
      sx={{
        minHeight: "100%",
        display: "grid",
        placeItems: "center",
        bgcolor: "background.default",
      }}
    >
      <Box sx={{ textAlign: "center" }}>
        <Box sx={{ mb: 3 }}>
          <Brand />
        </Box>
        <CircularProgress size={30} />
        <Typography color="text.secondary" sx={{ mt: 1.5 }}>
          {label}
        </Typography>
      </Box>
    </Box>
  );
}
