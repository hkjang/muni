import {
  Box,
  Button,
  Checkbox,
  FormControlLabel,
  IconButton,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from "@mui/material";
import { AddOutlined, DeleteOutline } from "@mui/icons-material";
import type { HandoffPeer, HandoffSettings as HandoffValue } from "../../types";

/** The formats muni sends. A peer is offered as a destination only for the
 * ones it receives, so these are the only boxes worth ticking here. */
export const sendableFormats: { value: string; label: string }[] = [
  { value: "markdown", label: "Markdown" },
  { value: "docx", label: "DOCX" },
];

type Props = {
  value: HandoffValue;
  onChange: (value: HandoffValue) => void;
};

/**
 * HandoffSettings is the 문서 넘기기 tab: which services documents may be sent
 * to and taken from. The list is the allow list for both directions — a
 * source not on it is refused before muni asks it for anything.
 */
export function HandoffSettings({ value, onChange }: Props) {
  const peers = value.peers ?? [];
  const update = (index: number, patch: Partial<HandoffPeer>) =>
    onChange({
      peers: peers.map((peer, i) =>
        i === index ? { ...peer, ...patch } : peer,
      ),
    });
  const remove = (index: number) =>
    onChange({ peers: peers.filter((_, i) => i !== index) });
  const add = () =>
    onChange({
      peers: [...peers, { origin: "", name: "", receives: ["markdown"] }],
    });
  const toggle = (index: number, format: string, on: boolean) => {
    const current = peers[index]?.receives ?? [];
    update(index, {
      receives: on
        ? [...current.filter((f) => f !== format), format]
        : current.filter((f) => f !== format),
    });
  };
  return (
    <Stack gap={2}>
      <Box mb={1}>
        <Typography variant="h3">문서 넘기기</Typography>
        <Typography color="text.secondary" mt={0.5}>
          문서를 내려받지 않고 다른 사내 서비스로 바로 보내고, 다른 서비스가
          보낸 문서를 받습니다. 여기 적은 서비스만 보낼 곳으로 나타나고, 여기
          적은 주소에서만 받습니다. 비어 있으면(기본값) 「다른 서비스로
          보내기」가 보이지 않고 아무 데서도 받지 않습니다.
        </Typography>
      </Box>
      <Table size="small" aria-label="문서를 주고받을 서비스">
        <TableHead>
          <TableRow>
            <TableCell sx={{ width: "34%" }}>주소 (오리진)</TableCell>
            <TableCell sx={{ width: "20%" }}>이름</TableCell>
            <TableCell>그 서비스가 받는 형식</TableCell>
            <TableCell padding="checkbox" />
          </TableRow>
        </TableHead>
        <TableBody>
          {peers.length === 0 && (
            <TableRow>
              <TableCell colSpan={4}>
                <Typography color="text.secondary" py={1}>
                  아직 없습니다. 「서비스 더하기」로 시작하세요.
                </Typography>
              </TableCell>
            </TableRow>
          )}
          {peers.map((peer, index) => (
            <TableRow key={index}>
              <TableCell>
                <TextField
                  size="small"
                  fullWidth
                  placeholder="https://ptium.intra"
                  value={peer.origin}
                  onChange={(e) => update(index, { origin: e.target.value })}
                  slotProps={{ htmlInput: { "aria-label": "주소" } }}
                />
              </TableCell>
              <TableCell>
                <TextField
                  size="small"
                  fullWidth
                  placeholder="ptium"
                  value={peer.name}
                  onChange={(e) => update(index, { name: e.target.value })}
                  slotProps={{ htmlInput: { "aria-label": "이름" } }}
                />
              </TableCell>
              <TableCell>
                <Stack direction="row" flexWrap="wrap">
                  {sendableFormats.map((format) => (
                    <FormControlLabel
                      key={format.value}
                      control={
                        <Checkbox
                          size="small"
                          checked={(peer.receives ?? []).includes(format.value)}
                          onChange={(e) =>
                            toggle(index, format.value, e.target.checked)
                          }
                        />
                      }
                      label={format.label}
                    />
                  ))}
                </Stack>
              </TableCell>
              <TableCell padding="checkbox">
                <IconButton
                  aria-label="서비스 지우기"
                  onClick={() => remove(index)}
                >
                  <DeleteOutline />
                </IconButton>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <Box>
        <Button startIcon={<AddOutlined />} onClick={add}>
          서비스 더하기
        </Button>
      </Box>
      <Typography variant="body2" color="text.secondary">
        주소는 스킴과 호스트까지만 적습니다(경로 없이). 상대 서비스에도 이 muni
        의 주소를 같은 방식으로 적어야 양쪽이 이어집니다. 받는 문서는 Markdown
        만, 25MB·30초까지이며 리다이렉트는 따라가지 않습니다.
      </Typography>
    </Stack>
  );
}
