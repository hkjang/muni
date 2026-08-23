import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Box,
  Button,
  Card,
  Checkbox,
  FormControlLabel,
  Grid,
  Stack,
  Typography,
} from "@mui/material";
import { useState } from "react";
import { api, errorMessage, jsonBody } from "../../lib/api";
type Policy = { role: string; permissions: string[]; updatedAt: string };
const permissions = [
  ["key:read:own", "내 키 조회"],
  ["key:rotate:own", "내 키 회전"],
  ["key:revoke:own", "내 과거 키 폐기"],
  ["key:read:any", "모든 사용자 키 조회"],
  ["key:rotate:any", "모든 사용자 키 회전"],
  ["key:revoke:any", "모든 사용자 키 폐기"],
  ["policy:manage", "키 정책 관리"],
] as const;
export function AdminKeyPoliciesPage() {
  const client = useQueryClient();
  const query = useQuery({
    queryKey: ["key-policies"],
    queryFn: () => api<Policy[]>("/api/v1/admin/key-policies"),
  });
  const [drafts, setDrafts] = useState<Record<string, string[]>>({});
  const save = useMutation({
    mutationFn: ({ role, values }: { role: string; values: string[] }) =>
      api(`/api/v1/admin/key-policies/${role}`, {
        method: "PUT",
        ...jsonBody({ permissions: values }),
      }),
    onSuccess: () => client.invalidateQueries({ queryKey: ["key-policies"] }),
  });
  const valuesFor = (policy: Policy) =>
    drafts[policy.role] ?? policy.permissions;
  const toggle = (policy: Policy, permission: string, checked: boolean) =>
    setDrafts((current) => ({
      ...current,
      [policy.role]: checked
        ? [...valuesFor(policy), permission]
        : valuesFor(policy).filter((value) => value !== permission),
    }));
  return (
    <Box sx={{ p: { xs: 2.5, sm: 4, lg: 5 }, maxWidth: 1100, mx: "auto" }}>
      <Typography variant="h1">키 권한 정책</Typography>
      <Typography color="text.secondary" mt={0.7} mb={3}>
        역할별 개인 키 관리 권한을 운영 중에도 변경할 수 있습니다.
      </Typography>
      {save.error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {errorMessage(save.error)}
        </Alert>
      )}
      <Grid container spacing={2}>
        {(query.data ?? []).map((policy) => (
          <Grid key={policy.role} size={{ xs: 12, md: 6 }}>
            <Card sx={{ p: 3 }}>
              <Typography variant="h3" mb={2}>
                {policy.role}
              </Typography>
              <Stack>
                {permissions.map(([permission, label]) => (
                  <FormControlLabel
                    key={permission}
                    control={
                      <Checkbox
                        checked={valuesFor(policy).includes(permission)}
                        disabled={
                          policy.role === "ADMIN" &&
                          permission === "policy:manage"
                        }
                        onChange={(_, checked) =>
                          toggle(policy, permission, checked)
                        }
                      />
                    }
                    label={
                      <Box>
                        <Typography>{label}</Typography>
                        <Typography variant="caption" color="text.secondary">
                          {permission}
                        </Typography>
                      </Box>
                    }
                  />
                ))}
              </Stack>
              <Button
                variant="contained"
                sx={{ mt: 2 }}
                disabled={!drafts[policy.role]}
                onClick={() =>
                  save.mutate({ role: policy.role, values: valuesFor(policy) })
                }
              >
                정책 저장
              </Button>
            </Card>
          </Grid>
        ))}
      </Grid>
    </Box>
  );
}
