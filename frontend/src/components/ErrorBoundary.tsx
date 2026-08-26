import { Component, type ReactNode } from "react";
import { Box, Button, Paper, Stack, Typography } from "@mui/material";
import { RefreshOutlined, WarningAmberOutlined } from "@mui/icons-material";

type Props = { children: ReactNode };
type State = { error: Error | null };

/**
 * The last thing between a mistake and a white page.
 *
 * There was no boundary anywhere: a render error in any component unmounted
 * the whole tree, and the person saw nothing at all — no message, no way back,
 * nothing to tell anyone about it.
 *
 * It also catches the failure that route splitting introduces. Screens now
 * arrive as separate files, so clicking into the editor while the network is
 * down fails at the fetch rather than working from what was already loaded.
 * That failure is worth naming precisely, because "reload" is genuinely the
 * fix for it and is not the fix for a bug in the code.
 */
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error) {
    // Kept in the console rather than sent anywhere: muni runs inside an
    // office network and does not report to the outside.
    console.error("화면을 그리는 중 오류가 발생했습니다", error);
  }

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;

    const isChunkFailure =
      /dynamically imported module|Loading chunk|Importing a module script failed/i.test(
        error.message,
      );

    return (
      <Box
        sx={{ minHeight: "100dvh", display: "grid", placeItems: "center", p: 2.5 }}
      >
        <Paper
          variant="outlined"
          sx={{ p: { xs: 3, sm: 4 }, width: "100%", maxWidth: 460 }}
        >
          <Stack gap={2}>
            <Stack direction="row" gap={1.2} alignItems="center">
              <WarningAmberOutlined color="warning" />
              <Typography variant="h2">
                {isChunkFailure
                  ? "화면을 불러오지 못했습니다"
                  : "문제가 생겼습니다"}
              </Typography>
            </Stack>
            <Typography variant="body2" color="text.secondary">
              {isChunkFailure
                ? "네트워크가 끊겼거나 muni가 업데이트되었을 수 있습니다. 새로고침하면 대개 해결됩니다."
                : "작성 중이던 내용은 서버에 저장된 것까지 그대로 남아 있습니다. 새로고침해 주세요. 계속 같은 화면이 나오면 관리자에게 알려 주세요."}
            </Typography>
            <Box
              component="pre"
              sx={{
                m: 0,
                p: 1.2,
                borderRadius: 1,
                bgcolor: "action.hover",
                fontSize: 12,
                whiteSpace: "pre-wrap",
                wordBreak: "break-word",
                color: "text.secondary",
                maxHeight: 140,
                overflowY: "auto",
              }}
            >
              {error.message}
            </Box>
            <Button
              variant="contained"
              startIcon={<RefreshOutlined />}
              onClick={() => window.location.reload()}
            >
              새로고침
            </Button>
          </Stack>
        </Paper>
      </Box>
    );
  }
}
