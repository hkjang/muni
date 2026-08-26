import { useMutation } from "@tanstack/react-query";
import { LockResetOutlined } from "@mui/icons-material";
import {
  Alert,
  Box,
  Button,
  Card,
  Stack,
  TextField,
  Typography,
} from "@mui/material";
import { useState } from "react";
import { useAuth } from "../contexts/AuthContext";
import { api, errorMessage, jsonBody } from "../lib/api";

/**
 * Where an account lands when its password is still the one it was handed.
 *
 * The server is what enforces this — every other endpoint answers
 * PASSWORD_CHANGE_REQUIRED — so this screen is not the lock. It is the
 * explanation, which is the part a wall of 403s cannot give.
 */
export function ChangeTemporaryPasswordPage() {
  const { user, refresh, logout } = useAuth();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");

  const mismatch = confirm.length > 0 && next !== confirm;
  const tooShort = next.length > 0 && [...next].length < 12;

  const change = useMutation({
    mutationFn: () =>
      api("/api/v1/me/password", {
        method: "POST",
        ...jsonBody({ currentPassword: current, newPassword: next }),
      }),
    onSuccess: () => refresh(),
  });

  return (
    <Box
      sx={{
        minHeight: "100dvh",
        display: "grid",
        placeItems: "center",
        p: 2.5,
      }}
    >
      <Card sx={{ p: { xs: 3, sm: 4 }, width: "100%", maxWidth: 460 }}>
        <Stack gap={2.5}>
          <Stack direction="row" gap={1.5} alignItems="center">
            <LockResetOutlined color="primary" />
            <Typography variant="h2">비밀번호를 바꿔 주세요</Typography>
          </Stack>
          <Typography variant="body2" color="text.secondary">
            {user?.displayName}님의 지금 비밀번호는 관리자가 정해 준 임시
            비밀번호입니다. 바꾸기 전까지는 다른 기능을 쓸 수 없습니다.
          </Typography>
          <TextField
            label="지금 비밀번호"
            type="password"
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
            autoComplete="current-password"
            autoFocus
            fullWidth
          />
          <TextField
            label="새 비밀번호"
            type="password"
            value={next}
            onChange={(e) => setNext(e.target.value)}
            autoComplete="new-password"
            error={tooShort}
            helperText={tooShort ? "12자 이상이어야 합니다" : "12자 이상"}
            fullWidth
          />
          <TextField
            label="새 비밀번호 확인"
            type="password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            autoComplete="new-password"
            error={mismatch}
            helperText={mismatch ? "위와 다릅니다" : " "}
            fullWidth
            onKeyDown={(e) => {
              if (e.key === "Enter" && !mismatch && !tooShort && current) {
                change.mutate();
              }
            }}
          />
          {change.error && (
            <Alert severity="error">{errorMessage(change.error)}</Alert>
          )}
          <Button
            variant="contained"
            size="large"
            disabled={
              !current || !next || mismatch || tooShort || change.isPending
            }
            onClick={() => change.mutate()}
          >
            {change.isPending ? "바꾸는 중…" : "바꾸고 시작하기"}
          </Button>
          <Typography variant="body2" color="text.secondary">
            바꾸면 다른 기기에 열려 있던 세션은 모두 종료됩니다.
          </Typography>
          <Button size="small" color="inherit" onClick={() => logout()}>
            로그아웃
          </Button>
        </Stack>
      </Card>
    </Box>
  );
}
