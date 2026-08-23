import { useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Card,
  CircularProgress,
  Divider,
  Stack,
  TextField,
  Typography,
} from "@mui/material";
import { LoginOutlined, SecurityOutlined } from "@mui/icons-material";
import {
  Navigate,
  useLocation,
  useNavigate,
  useSearchParams,
} from "react-router-dom";
import { Brand } from "../components/Brand";
import { useAuth } from "../contexts/AuthContext";
import { errorMessage } from "../lib/api";

export function LoginPage() {
  const { user, system, loading, login } = useAuth();
  const [identity, setIdentity] = useState("");
  const [password, setPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const location = useLocation();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const returnTo =
    (location.state as { returnTo?: string } | null)?.returnTo ?? "/";
  useEffect(() => {
    const code = params.get("error");
    if (code)
      setError(
        `SSO 로그인에 실패했습니다 (${code}). 관리자에게 설정을 확인해 달라고 요청하세요.`,
      );
  }, [params]);
  if (loading)
    return (
      <Box sx={{ height: "100%", display: "grid", placeItems: "center" }}>
        <CircularProgress />
      </Box>
    );
  if (user) return <Navigate to={returnTo} replace />;
  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      await login(identity, password);
      navigate(returnTo, { replace: true });
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setSubmitting(false);
    }
  };
  return (
    <Box
      sx={{
        minHeight: "100%",
        display: "grid",
        gridTemplateColumns: {
          xs: "1fr",
          md: "minmax(380px, .9fr) minmax(520px, 1.1fr)",
        },
        bgcolor: "#f7f7fb",
      }}
    >
      <Box
        sx={{
          display: "flex",
          flexDirection: "column",
          p: { xs: 3, sm: 6, lg: 9 },
          bgcolor: "#fff",
        }}
      >
        <Brand />
        <Box
          sx={{ my: "auto", width: "100%", maxWidth: 430, mx: "auto", py: 6 }}
        >
          <Typography variant="h1" sx={{ mb: 1 }}>
            다시 만나 반갑습니다
          </Typography>
          <Typography color="text.secondary" sx={{ mb: 4, fontSize: 16 }}>
            안전한 문서 협업 공간에 로그인하세요.
          </Typography>
          {error && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {error}
            </Alert>
          )}
          {system?.localLoginEnabled && (
            <Box component="form" onSubmit={submit}>
              <Stack gap={2}>
                <TextField
                  label="아이디 또는 이메일"
                  autoComplete="username"
                  value={identity}
                  onChange={(event) => setIdentity(event.target.value)}
                  required
                  fullWidth
                />
                <TextField
                  label="비밀번호"
                  type="password"
                  autoComplete="current-password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  required
                  fullWidth
                />
                <Button
                  type="submit"
                  variant="contained"
                  size="large"
                  disabled={submitting}
                  startIcon={
                    submitting ? (
                      <CircularProgress size={18} />
                    ) : (
                      <LoginOutlined />
                    )
                  }
                >
                  {submitting ? "로그인 중…" : "로그인"}
                </Button>
              </Stack>
            </Box>
          )}
          {system?.localLoginEnabled && system.oidcEnabled && (
            <Divider sx={{ my: 3 }}>또는</Divider>
          )}
          {system?.oidcEnabled && (
            <Button
              fullWidth
              size="large"
              variant="outlined"
              startIcon={<SecurityOutlined />}
              href={`${system.oidcLoginUrl}?return_to=${encodeURIComponent(returnTo)}`}
            >
              Keycloak SSO로 로그인
            </Button>
          )}
          {!system?.localLoginEnabled && !system?.oidcEnabled && (
            <Alert severity="warning">
              사용 가능한 로그인 방식이 없습니다. 서비스 관리자 설정을
              확인하세요.
            </Alert>
          )}
        </Box>
        <Typography variant="caption" color="text.secondary">
          muni {system?.version ?? "dev"} · 온프레미스 문서 플랫폼
        </Typography>
      </Box>
      <Box
        sx={{
          display: { xs: "none", md: "flex" },
          position: "relative",
          overflow: "hidden",
          alignItems: "center",
          justifyContent: "center",
          p: 8,
          color: "#fff",
          background:
            "linear-gradient(145deg,#34359a 0%,#5c5dcd 58%,#477d96 100%)",
          "&::before": {
            content: '""',
            position: "absolute",
            width: 520,
            height: 520,
            borderRadius: "50%",
            border: "1px solid rgba(255,255,255,.15)",
            right: -140,
            top: -170,
          },
          "&::after": {
            content: '""',
            position: "absolute",
            width: 320,
            height: 320,
            borderRadius: "50%",
            bgcolor: "rgba(255,255,255,.06)",
            left: -90,
            bottom: -100,
          },
        }}
      >
        <Box sx={{ position: "relative", zIndex: 1, maxWidth: 620 }}>
          <Typography
            sx={{
              fontSize: { md: 38, lg: 48 },
              lineHeight: 1.25,
              fontWeight: 730,
              letterSpacing: "-.045em",
              mb: 3,
            }}
          >
            생각이 문서가 되고,
            <br />
            문서가 함께 움직입니다.
          </Typography>
          <Typography
            sx={{
              fontSize: 18,
              lineHeight: 1.8,
              color: "rgba(255,255,255,.84)",
              maxWidth: 520,
            }}
          >
            실시간 공동편집, 버전, 검토, 검색과 AI 도구를 사내 네트워크 안에서
            안전하게 사용하세요.
          </Typography>
          <Card
            sx={{
              mt: 6,
              p: 2.5,
              bgcolor: "rgba(255,255,255,.1)",
              color: "#fff",
              borderColor: "rgba(255,255,255,.18)",
              backdropFilter: "blur(10px)",
            }}
          >
            <Stack direction="row" gap={2} alignItems="center">
              <SecurityOutlined />
              <Box>
                <Typography fontWeight={700}>
                  데이터 경계를 지키는 설계
                </Typography>
                <Typography
                  variant="body2"
                  sx={{ color: "rgba(255,255,255,.75)" }}
                >
                  문서 ACL을 검색과 AI 컨텍스트보다 먼저 적용합니다.
                </Typography>
              </Box>
            </Stack>
          </Card>
        </Box>
      </Box>
    </Box>
  );
}
