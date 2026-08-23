import { Box, Button, Typography } from "@mui/material";
import { useNavigate } from "react-router-dom";
import { Brand } from "../components/Brand";
export function NotFoundPage() {
  const navigate = useNavigate();
  return (
    <Box sx={{ height: "100%", display: "grid", placeItems: "center", p: 3 }}>
      <Box textAlign="center">
        <Box display="flex" justifyContent="center" mb={3}>
          <Brand />
        </Box>
        <Typography variant="h1">페이지를 찾을 수 없습니다</Typography>
        <Typography color="text.secondary" my={2}>
          주소가 바뀌었거나 접근할 수 없는 페이지입니다.
        </Typography>
        <Button variant="contained" onClick={() => navigate("/")}>
          홈으로 이동
        </Button>
      </Box>
    </Box>
  );
}
