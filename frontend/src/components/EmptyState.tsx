import { Box, Button, Typography, type SvgIconProps } from "@mui/material";
import type { ComponentType } from "react";

export function EmptyState({
  icon: Icon,
  title,
  description,
  action,
  onAction,
}: {
  icon: ComponentType<SvgIconProps>;
  title: string;
  description: string;
  action?: string;
  onAction?: () => void;
}) {
  return (
    <Box sx={{ textAlign: "center", py: 8, px: 3, color: "text.secondary" }}>
      <Box
        sx={{
          width: 58,
          height: 58,
          borderRadius: 3,
          bgcolor: "#eeeeFA",
          color: "primary.main",
          display: "grid",
          placeItems: "center",
          mx: "auto",
          mb: 2,
        }}
      >
        <Icon sx={{ fontSize: 29 }} />
      </Box>
      <Typography variant="h3" color="text.primary" gutterBottom>
        {title}
      </Typography>
      <Typography sx={{ maxWidth: 480, mx: "auto", mb: action ? 2.5 : 0 }}>
        {description}
      </Typography>
      {action && (
        <Button variant="contained" onClick={onAction}>
          {action}
        </Button>
      )}
    </Box>
  );
}
