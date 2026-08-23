import { Box, Typography } from "@mui/material";
import { useAuth } from "../contexts/AuthContext";

export function Brand({
  inverse = false,
  compact = false,
}: {
  inverse?: boolean;
  compact?: boolean;
}) {
  const { system } = useAuth();
  const name = system?.serviceName || "muni";
  return (
    <Box
      sx={{ display: "flex", alignItems: "center", gap: 1.1, minWidth: 0 }}
      aria-label={`${name} 홈`}
    >
      <Box
        sx={{
          width: compact ? 31 : 36,
          height: compact ? 31 : 36,
          borderRadius: "11px 5px 11px 5px",
          bgcolor: inverse ? "#8f8ff0" : "primary.main",
          display: "grid",
          placeItems: "center",
          color: "#fff",
          fontWeight: 800,
          fontSize: compact ? 17 : 20,
          boxShadow: inverse ? "none" : "0 5px 14px rgba(81,81,198,.22)",
        }}
      >
        {name.slice(0, 1).toLowerCase()}
      </Box>
      {!compact && (
        <Typography
          sx={{
            fontWeight: 760,
            fontSize: 21,
            letterSpacing: "-.04em",
            color: inverse ? "#fff" : "text.primary",
          }}
        >
          {name}
        </Typography>
      )}
    </Box>
  );
}
