import { useMutation } from "@tanstack/react-query";
import {
  ContentCopyOutlined,
  PersonAddAlt1Outlined,
  UploadFileOutlined,
} from "@mui/icons-material";
import {
  Alert,
  Box,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControl,
  FormControlLabel,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  Switch,
  Tab,
  Tabs,
  TextField,
  Typography,
} from "@mui/material";
import { useRef, useState } from "react";
import { api, errorMessage, jsonBody } from "../../lib/api";

type CreatedUser = {
  id: string;
  username: string;
  email: string;
  displayName: string;
  role: string;
  temporaryPassword?: string;
  emailSent?: boolean;
  emailError?: string;
};

type ImportRow = {
  line: number;
  email: string;
  ok: boolean;
  error?: string;
  username?: string;
  temporaryPassword?: string;
};

type ImportResult = {
  created: number;
  failed: number;
  results: ImportRow[];
};

/**
 * A password muni generated is shown once and never again — it is stored only
 * as a hash. Copying it has to be one click, because the alternative is an
 * administrator retyping sixteen characters into a chat window and getting one
 * of them wrong.
 */
function CopyableSecret({ value }: { value: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <Stack direction="row" gap={1} alignItems="center">
      <Box
        component="code"
        sx={{
          fontFamily: "monospace",
          fontSize: 15,
          px: 1.2,
          py: 0.6,
          borderRadius: 1,
          bgcolor: "action.hover",
          userSelect: "all",
        }}
      >
        {value}
      </Box>
      <Button
        size="small"
        startIcon={<ContentCopyOutlined />}
        onClick={async () => {
          try {
            await navigator.clipboard.writeText(value);
            setCopied(true);
            setTimeout(() => setCopied(false), 1600);
          } catch {
            // Clipboard access can be refused. The value is selectable above,
            // so saying nothing is better than an error the person cannot act
            // on.
          }
        }}
      >
        {copied ? "복사함" : "복사"}
      </Button>
    </Stack>
  );
}

export function CreateUserDialog({
  open,
  onClose,
  onCreated,
}: {
  open: boolean;
  onClose: () => void;
  onCreated: () => void;
}) {
  const [tab, setTab] = useState<"one" | "csv">("one");
  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [username, setUsername] = useState("");
  const [role, setRole] = useState("USER");
  const [sendEmail, setSendEmail] = useState(false);
  const [created, setCreated] = useState<CreatedUser | null>(null);
  const [imported, setImported] = useState<ImportResult | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);

  const reset = () => {
    setEmail("");
    setDisplayName("");
    setUsername("");
    setRole("USER");
    setSendEmail(false);
    setCreated(null);
    setImported(null);
    createOne.reset();
    importCSV.reset();
  };

  const createOne = useMutation({
    mutationFn: () =>
      api<CreatedUser>("/api/v1/admin/users", {
        method: "POST",
        ...jsonBody({
          email: email.trim(),
          displayName: displayName.trim(),
          username: username.trim(),
          role,
          sendEmail,
        }),
      }),
    onSuccess: (user) => {
      setCreated(user);
      onCreated();
    },
  });

  const importCSV = useMutation({
    mutationFn: (file: File) => {
      const form = new FormData();
      form.append("file", file);
      return api<ImportResult>("/api/v1/admin/users/import", {
        method: "POST",
        body: form,
      });
    },
    onSuccess: (result) => {
      setImported(result);
      onCreated();
    },
  });

  const close = () => {
    reset();
    onClose();
  };

  // Once an account exists the dialog stops being a form: the only thing left
  // that matters is the password, which cannot be recovered after this.
  if (created) {
    return (
      <Dialog open={open} onClose={close} fullWidth maxWidth="sm">
        <DialogTitle>{created.displayName} 계정을 만들었습니다</DialogTitle>
        <DialogContent>
          <Stack gap={2} mt={1}>
            <Stack direction="row" gap={1} alignItems="center">
              <Chip size="small" label={created.username} />
              <Typography variant="body2" color="text.secondary">
                {created.email}
              </Typography>
            </Stack>
            {created.temporaryPassword && (
              <Box>
                <Typography variant="body2" fontWeight={700} mb={0.8}>
                  임시 비밀번호
                </Typography>
                <CopyableSecret value={created.temporaryPassword} />
                <Alert severity="warning" sx={{ mt: 1.5 }}>
                  이 화면을 닫으면 다시 볼 수 없습니다. muni는 비밀번호를 해시로만
                  보관합니다. 잃어버리면 새로 설정하면 됩니다.
                </Alert>
              </Box>
            )}
            {created.emailSent === true && (
              <Alert severity="success">
                {created.email}로 접속 정보를 보냈습니다.
              </Alert>
            )}
            {created.emailSent === false && (
              <Alert severity="warning">
                계정은 만들어졌지만 메일은 보내지 못했습니다
                {created.emailError ? ` — ${created.emailError}` : "."} 위
                비밀번호를 직접 전달해 주세요.
              </Alert>
            )}
            <Typography variant="body2" color="text.secondary">
              이 사람이 처음 로그인하면 비밀번호를 바꾸기 전까지 다른 기능을 쓸 수
              없습니다.
            </Typography>
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={reset}>계속 만들기</Button>
          <Button variant="contained" onClick={close}>
            닫기
          </Button>
        </DialogActions>
      </Dialog>
    );
  }

  if (imported) {
    return (
      <Dialog open={open} onClose={close} fullWidth maxWidth="md">
        <DialogTitle>
          {imported.created}건 만들었습니다
          {imported.failed > 0 && ` · ${imported.failed}건 실패`}
        </DialogTitle>
        <DialogContent>
          <Alert severity="warning" sx={{ mb: 2 }}>
            임시 비밀번호는 이 화면에서만 볼 수 있습니다. 닫기 전에
            내려받으세요.
          </Alert>
          <Stack gap={0.5}>
            {imported.results.map((row) => (
              <Stack
                key={row.line}
                direction="row"
                gap={1.5}
                alignItems="center"
                sx={{ py: 0.6, borderBottom: 1, borderColor: "divider" }}
              >
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ width: 44 }}
                >
                  {row.line}행
                </Typography>
                <Typography variant="body2" sx={{ flex: 1, minWidth: 0 }}>
                  {row.email}
                </Typography>
                {row.ok ? (
                  <CopyableSecret value={row.temporaryPassword ?? ""} />
                ) : (
                  <Typography variant="body2" color="error.main">
                    {row.error}
                  </Typography>
                )}
              </Stack>
            ))}
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button
            startIcon={<UploadFileOutlined />}
            onClick={() => {
              // A CSV, because that is what gets pasted into the mail that
              // tells forty people their password.
              const lines = [
                "email,username,temporaryPassword,error",
                ...imported.results.map((row) =>
                  [
                    row.email,
                    row.username ?? "",
                    row.temporaryPassword ?? "",
                    row.error ?? "",
                  ]
                    .map((cell) => `"${String(cell).replace(/"/g, '""')}"`)
                    .join(","),
                ),
              ];
              const blob = new Blob(["﻿" + lines.join("\r\n")], {
                type: "text/csv;charset=utf-8",
              });
              const url = URL.createObjectURL(blob);
              const anchor = document.createElement("a");
              anchor.href = url;
              anchor.download = "muni-계정.csv";
              anchor.click();
              URL.revokeObjectURL(url);
            }}
          >
            결과 CSV 내려받기
          </Button>
          <Button variant="contained" onClick={close}>
            닫기
          </Button>
        </DialogActions>
      </Dialog>
    );
  }

  return (
    <Dialog open={open} onClose={close} fullWidth maxWidth="sm">
      <DialogTitle>계정 만들기</DialogTitle>
      <DialogContent>
        <Tabs
          value={tab}
          onChange={(_, next) => setTab(next)}
          sx={{ mb: 2.5 }}
        >
          <Tab value="one" label="한 명" />
          <Tab value="csv" label="CSV로 여러 명" />
        </Tabs>

        {tab === "one" && (
          <Stack gap={2}>
            <TextField
              label="이메일"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
              autoFocus
              fullWidth
            />
            <TextField
              label="이름"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              placeholder="비우면 아이디를 씁니다"
              fullWidth
            />
            <TextField
              label="아이디"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="비우면 이메일 앞부분을 씁니다"
              helperText="이미 쓰는 아이디면 뒤에 숫자가 붙습니다"
              fullWidth
            />
            <FormControl fullWidth>
              <InputLabel>역할</InputLabel>
              <Select
                label="역할"
                value={role}
                onChange={(e) => setRole(e.target.value)}
              >
                <MenuItem value="USER">USER</MenuItem>
                <MenuItem value="ADMIN">ADMIN</MenuItem>
              </Select>
            </FormControl>
            <FormControlLabel
              control={
                <Switch
                  checked={sendEmail}
                  onChange={(e) => setSendEmail(e.target.checked)}
                />
              }
              label="접속 정보를 메일로 보내기"
            />
            <Typography variant="body2" color="text.secondary">
              비밀번호는 muni가 만들어 다음 화면에 한 번 보여줍니다. 받은 사람은
              처음 로그인할 때 바꿔야 합니다.
            </Typography>
            {createOne.error && (
              <Alert severity="error">{errorMessage(createOne.error)}</Alert>
            )}
          </Stack>
        )}

        {tab === "csv" && (
          <Stack gap={2}>
            <Typography variant="body2" color="text.secondary">
              첫 줄에 열 이름을 넣어 주세요. <code>email</code>만 필수이고,{" "}
              <code>username</code>·<code>displayName</code>·<code>role</code>{" "}
              은 있으면 씁니다. 한글 열 이름(<code>이메일</code>,{" "}
              <code>이름</code>, <code>역할</code>)도 받습니다. 한 번에 500행까지.
            </Typography>
            <Box
              component="pre"
              sx={{
                m: 0,
                p: 1.5,
                borderRadius: 1,
                bgcolor: "action.hover",
                fontSize: 13,
                overflowX: "auto",
              }}
            >
              {"이름,email,역할\n김민수,minsu@example.com,USER\n이서연,seoyeon@example.com,ADMIN"}
            </Box>
            <Divider />
            <input
              ref={fileInput}
              type="file"
              accept=".csv,text/csv"
              hidden
              onChange={(e) => {
                const file = e.target.files?.[0];
                if (file) importCSV.mutate(file);
                e.target.value = "";
              }}
            />
            <Button
              variant="outlined"
              startIcon={<UploadFileOutlined />}
              onClick={() => fileInput.current?.click()}
              disabled={importCSV.isPending}
            >
              {importCSV.isPending ? "가져오는 중…" : "CSV 파일 고르기"}
            </Button>
            <Typography variant="body2" color="text.secondary">
              한 행이 잘못돼도 나머지는 그대로 만들어집니다. 실패한 행은 이유와
              함께 돌아옵니다.
            </Typography>
            {importCSV.error && (
              <Alert severity="error">{errorMessage(importCSV.error)}</Alert>
            )}
          </Stack>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={close}>취소</Button>
        {tab === "one" && (
          <Button
            variant="contained"
            startIcon={<PersonAddAlt1Outlined />}
            disabled={!email.trim() || createOne.isPending}
            onClick={() => createOne.mutate()}
          >
            {createOne.isPending ? "만드는 중…" : "만들기"}
          </Button>
        )}
      </DialogActions>
    </Dialog>
  );
}
